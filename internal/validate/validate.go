// Package validate checks tours end to end: the schema, then every step
// against the code it points at.
package validate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/visual"
	"github.com/primandproper/docent/internal/walk"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const o11yName = "validator"

type (
	// Validator checks tour files.
	Validator interface {
		// Tour validates one tour file and returns everything wrong with it.
		Tour(ctx context.Context, path string) ([]diag.Diagnostic, error)
	}

	validator struct {
		_      struct{}
		store  tour.Store
		walker walk.Walker
		logger logging.Logger
		tracer tracing.Tracer
	}
)

var _ Validator = (*validator)(nil)

// NewValidator returns a Validator.
func NewValidator(store tour.Store, walker walk.Walker, logger logging.Logger, tracerProvider tracing.Provider) Validator {
	return &validator{
		store:  store,
		walker: walker,
		logger: logging.NewNamedLogger(logger, o11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, o11yName),
	}
}

func (v *validator) Tour(ctx context.Context, path string) ([]diag.Diagnostic, error) {
	ctx, span := v.tracer.StartSpan(ctx)
	defer span.End()

	logger := v.logger.WithValue("tour", path)

	t, diags, err := v.store.Load(ctx, path)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "loading tour")
	}

	if t == nil {
		return diags, nil
	}

	diags = append(diags, mapDiagnostics(t, v.walker.Map(ctx, t))...)

	for i := range t.Steps {
		step, stepErr := v.walker.Step(ctx, t, i)
		if stepErr != nil {
			return nil, observability.PrepareAndLogError(stepErr, logger, span, "resolving step %d", i)
		}

		if d, found := stepDiagnostic(t, i, step); found {
			diags = append(diags, d)
		}

		for j := range step.Extras {
			if d, found := extraDiagnostic(t, i, j, step.Extras[j].Code); found {
				diags = append(diags, d)
			}
		}

		diags = append(diags, narrativeDiagnostics(t, i, step)...)
		diags = append(diags, embedDiagnostics(t, i, step)...)
	}

	logger.WithValue("diagnostics", len(diags)).Debug("validated tour")

	return diags, nil
}

// stepDiagnostic reports what's wrong with a resolved step, if anything.
func stepDiagnostic(t *tour.Tour, index int, step *walk.Step) (diag.Diagnostic, bool) {
	d := diag.Diagnostic{
		File:     t.Path,
		Step:     t.StepLabel(index),
		Severity: diag.SeverityError,
	}

	switch step.Kind {
	case walk.KindDirectory:
		if step.Problem == "" {
			return d, false
		}

		d.Code, d.Message = step.Problem, step.Detail
		d.Pointer = diag.Pointer("steps", index, "directory")
		d.Hint = "Paths are relative to the folder that contains the tours directory."

		if step.Problem == diag.CodeRepoUnknown || step.Problem == diag.CodeRepoMissing {
			d.Pointer, d.Hint = anchorPointerAndHint(index, &t.Steps[index], &anchor.Location{Problem: step.Problem})
		}
	case walk.KindFile:
		loc := step.Anchor.Location
		if loc.Problem == "" {
			return d, false
		}

		d.Code, d.Message = loc.Problem, loc.Detail
		d.Pointer, d.Hint = anchorPointerAndHint(index, &t.Steps[index], &loc)

		// Drift is a warning: the step still shows something, and saying how
		// far to trust it is the point. Code that is gone is an error.
		switch loc.Status {
		case anchor.StatusMoved, anchor.StatusChanged, anchor.StatusUnverified:
			d.Severity = diag.SeverityWarning
		case anchor.StatusOK, anchor.StatusAmbiguous, anchor.StatusMissing:
		}
	case walk.KindURI, walk.KindContent:
		return d, false
	}

	return d, true
}

// mapDiagnostics checks the system map and every `mapFocus` against it.
func mapDiagnostics(t *tour.Tour, m *walk.Map) []diag.Diagnostic {
	var out []diag.Diagnostic

	if m != nil && m.Problem != "" {
		out = append(out, diag.Diagnostic{
			File: t.Path, Pointer: diag.Pointer("map"), Severity: diag.SeverityError, Code: m.Problem, Message: m.Detail,
		})
	}

	for i := range t.Steps {
		for j, id := range t.Steps[i].MapFocus {
			d := diag.Diagnostic{
				File:     t.Path,
				Pointer:  diag.Pointer("steps", i, "mapFocus", j),
				Step:     t.StepLabel(i),
				Severity: diag.SeverityError,
				Code:     diag.CodeMapFocusUnknown,
			}

			switch {
			case m == nil:
				d.Message = fmt.Sprintf("mapFocus names %q, but the tour has no `map`", id)
			case m.Problem != "" || m.Defined.Has(id):
				// An unreadable map is reported once, above.
				continue
			default:
				d.Message = fmt.Sprintf("mapFocus names %q, which the map doesn't define", id)
				d.Hint = "Use a node id, or an edge named in the map with Mermaid's edge-id syntax: `api e1@--> queue`."
			}

			out = append(out, d)
		}
	}

	return out
}

// narrativeDiagnostics reports problems with a step's body file and links.
func narrativeDiagnostics(t *tour.Tour, index int, step *walk.Step) []diag.Diagnostic {
	var out []diag.Diagnostic

	if step.BodyProblem != "" {
		out = append(out, diag.Diagnostic{
			File:     t.Path,
			Pointer:  diag.Pointer("steps", index, "body"),
			Step:     t.StepLabel(index),
			Severity: diag.SeverityError,
			Code:     step.BodyProblem,
			Message:  step.BodyDetail,
		})
	}

	for i := range step.Links {
		card := &step.Links[i]
		if card.Problem == "" {
			continue
		}

		d := diag.Diagnostic{
			File:     t.Path,
			Pointer:  diag.Pointer("steps", index, "links", i, "url"),
			Step:     t.StepLabel(index),
			Severity: diag.SeverityError,
			Code:     diag.CodeLinkScheme,
			Message:  fmt.Sprintf("link %q: only http, https and docent URLs can be opened", card.Title),
		}

		if len(card.Missing) > 0 {
			// A value may be set on another machine, so this can't be an error.
			d.Severity = diag.SeverityWarning
			d.Code = diag.CodeLinkVarUnresolved
			d.Message = fmt.Sprintf("link %q: %s", card.Title, strings.ToLower(card.Problem[:1])+card.Problem[1:])
			d.Hint = "Declare it under the tour's `vars` with a default, or set it in docent.json `vars`."
		}

		out = append(out, d)
	}

	return out
}

var stepLink = regexp.MustCompile(`docent:step/([A-Za-z0-9._-]+)`)

// embedDiagnostics checks what a step embeds: its `visual`, the docent-*
// fences in its narrative, and every link to another step.
func embedDiagnostics(t *tour.Tour, index int, step *walk.Step) []diag.Diagnostic {
	var out []diag.Diagnostic

	s := &t.Steps[index]

	report := func(code diag.Code, field, where, message string) {
		tokens := []any{"steps", index, field}
		if where != "" {
			for _, token := range strings.Split(where, "/") {
				tokens = append(tokens, token)
			}
		}

		out = append(out, diag.Diagnostic{
			File: t.Path, Pointer: diag.Pointer(tokens...), Step: t.StepLabel(index),
			Severity: diag.SeverityError, Code: code, Message: message,
		})
	}

	knownStep := func(id string) bool {
		for i := range t.Steps {
			if t.Steps[i].ID == id {
				return true
			}
		}

		return false
	}

	checkVisual := func(value any, field, prefix string) {
		problems, err := visual.Validate(value)
		if err != nil {
			report(diag.CodeVisualInvalid, field, "", prefix+err.Error())
			return
		}

		for i := range problems {
			p := &problems[i]

			where, message := "", prefix+p.Message
			if p.Where != "" {
				message = prefix + p.Where + ": " + p.Message
			}

			// Only the `visual` field is JSON the pointer can reach into.
			if field == "visual" {
				where, message = p.Where, p.Message
			}

			report(diag.CodeVisualInvalid, field, where, message)
		}

		for _, id := range visual.StepRefs(value) {
			if !knownStep(id) {
				report(diag.CodeStepLinkUnknown, field, "", fmt.Sprintf("%sthe visual links to step %q, which this tour doesn't have", prefix, id))
			}
		}
	}

	if len(s.Visual) > 0 {
		var value any
		if err := json.Unmarshal(s.Visual, &value); err == nil {
			checkVisual(value, "visual", "")
		}
	}

	// The narrative is the body file when there is one, else the description.
	field, text := "description", s.Description
	if step.Body != "" {
		field, text = "body", step.Body
	}

	fences := visual.Fences(text)

	for i := range fences {
		f := &fences[i]
		prefix := fmt.Sprintf("the %s fence on line %d: ", f.Lang, f.Line)

		switch {
		case f.Lang == visual.LinksFence:
			for _, problem := range fencedLinks(f) {
				report(diag.CodeVisualInvalid, field, "", prefix+problem)
			}
		case strings.HasPrefix(f.Lang, visual.FencePrefix):
			value, err := visual.FromFence(f)
			if err != nil {
				report(diag.CodeVisualInvalid, field, "", prefix+err.Error())
				continue
			}

			checkVisual(value, field, prefix)
		}
	}

	for _, match := range stepLink.FindAllStringSubmatch(text, -1) {
		if !knownStep(match[1]) {
			report(diag.CodeStepLinkUnknown, field, "", fmt.Sprintf("links to step %q, which this tour doesn't have", match[1]))
		}
	}

	return out
}

// fencedLinks checks the array of links in a docent-links fence.
func fencedLinks(f *visual.Fence) []string {
	fenced, err := FencedLinks(f.Body)
	if err != nil {
		return []string{err.Error()}
	}

	var problems []string

	for i := range fenced {
		link := &fenced[i]

		switch {
		case link.Title == "" || link.URL == "":
			problems = append(problems, fmt.Sprintf("link %d needs a `title` and a `url`", i+1))
		case !slices.Contains(tour.LinkKinds, link.Kind):
			problems = append(problems, fmt.Sprintf("link %q has kind %q; kinds are %s", link.Title, link.Kind, strings.Join(tour.LinkKinds, ", ")))
		}
	}

	return problems
}

// FencedLinks decodes the body of a docent-links fence: a YAML (or JSON)
// array of links, the same shape as a step's `links`.
func FencedLinks(body string) ([]tour.Link, error) {
	value, err := visual.Decode(body)
	if err != nil {
		return nil, err
	}

	list, ok := value.([]any)
	if !ok {
		return nil, errors.New("a docent-links fence holds a list of links")
	}

	out := make([]tour.Link, 0, len(list))

	for i, item := range list {
		object, isObject := item.(map[string]any)
		if !isObject {
			return nil, fmt.Errorf("link %d is not a set of fields", i+1)
		}

		field := func(name string) string {
			s, _ := object[name].(string) //nolint:errcheck // a missing or non-string field reads as empty
			return s
		}

		out = append(out, tour.Link{Kind: field("kind"), Title: field("title"), URL: field("url"), Description: field("description")})
	}

	return out, nil
}

// extraDiagnostic reports what's wrong with one of a step's extra anchors,
// pointing inside `anchors` instead of at the step's own fields.
func extraDiagnostic(t *tour.Tour, index, extra int, code *walk.Step) (diag.Diagnostic, bool) {
	a := &t.Steps[index].Anchors[extra]
	pseudo := tour.Tour{Path: t.Path, Steps: []tour.Step{{ID: t.Steps[index].ID, Repo: a.Repo, File: a.File, Selection: a.Selection}}}

	d, found := stepDiagnostic(&pseudo, 0, code)
	if !found {
		return d, false
	}

	d.Step = t.StepLabel(index)
	d.Message = fmt.Sprintf("anchor %q: %s", a.Label, d.Message)

	// stepDiagnostic pointed at /steps/0/<field>; the field lives under the anchor.
	field := strings.TrimPrefix(d.Pointer, "/steps/0")
	if strings.HasPrefix(d.Pointer, "/repos/") {
		return d, true
	}

	d.Pointer = diag.Pointer("steps", index, "anchors", extra) + field

	return d, true
}

func anchorPointerAndHint(index int, s *tour.Step, loc *anchor.Location) (pointer, hint string) {
	switch loc.Problem {
	case diag.CodePatternInvalid:
		return diag.Pointer("steps", index, "pattern"), "Patterns use Go RE2 syntax, which has no lookaround or backreferences."
	case diag.CodePatternNoMatch:
		return diag.Pointer("steps", index, "pattern"), "The pattern is tested against each line of the file on its own."
	case diag.CodePatternAmbiguous:
		return diag.Pointer("steps", index, "pattern"), "Add `line` to pick the nearest match, or make the pattern more specific."
	case diag.CodeAnchorMoved:
		return diag.Pointer("steps", index, "line"), "Update `line` to where the pattern now matches."
	case diag.CodeLineOutOfRange:
		if s.Selection != nil {
			return diag.Pointer("steps", index, "selection"), ""
		}

		return diag.Pointer("steps", index, "line"), ""
	case diag.CodeContentChanged:
		return diag.Pointer("steps", index), "Check the step still describes the code, fix what it says, then run `docent stamp`."
	case diag.CodeCodeRemoved:
		return diag.Pointer("steps", index), "Run `docent stale` to see what replaced it, then re-anchor or drop the step."
	case diag.CodeStampUnreachable:
		return diag.Pointer("repos", s.Repo, "stamp"), "Fetch the repo, or run `docent stamp` once the tour has been checked against the current code."
	case diag.CodeRefMissing:
		return diag.Pointer("steps", index, "file"), "Fetch the repo to get the commit, or change the `ref`."
	case diag.CodeRepoUnknown:
		return diag.Pointer("steps", index, "repo"), "Add the repo to the tour's `repos`, or fix the name."
	case diag.CodeRepoMissing:
		return diag.Pointer("steps", index, "repo"), "Clone it beside the other repos, or map it under `repos` in docent.json."
	default:
		if s.Repo != "" {
			return diag.Pointer("steps", index, "file"), "With `repo` set, paths are relative to that repo's root."
		}

		return diag.Pointer("steps", index, "file"), "Paths are relative to the folder that contains the tours directory."
	}
}
