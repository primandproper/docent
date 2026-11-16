// Package validate checks tours end to end: the schema, then every step
// against the code it points at.
package validate

import (
	"context"
	"fmt"
	"strings"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/tour"
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

	for i := range t.Steps {
		step, stepErr := v.walker.Step(ctx, t, i)
		if stepErr != nil {
			return nil, observability.PrepareAndLogError(stepErr, logger, span, "resolving step %d", i)
		}

		if d, found := stepDiagnostic(t, i, step); found {
			diags = append(diags, d)
		}

		diags = append(diags, narrativeDiagnostics(t, i, step)...)
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

		if loc.Status == anchor.StatusMoved {
			d.Severity = diag.SeverityWarning
		}
	case walk.KindURI, walk.KindContent:
		return d, false
	}

	return d, true
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
			Message:  fmt.Sprintf("link %q: only http and https URLs can be opened", card.Title),
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
