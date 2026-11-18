package validate

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/tour"
)

type (
	// neighbor is what a link needs to know about a tour: where it is, what
	// it is called, and which steps it has.
	neighbor struct {
		_     struct{}
		steps map[string]struct{}
		path  string
		title string
		// readable is false for a tour that couldn't be parsed, whose title
		// and steps are therefore unknown.
		readable bool
	}

	// tourRef is one docent:tour link, and where in the document it is.
	tourRef struct {
		_       struct{}
		pointer string
		step    string
		path    string
		id      string
	}
)

// tourLink finds docent:tour/<path>#<id> wherever it is written: in a Markdown
// link, a link card or a docent-links fence.
var tourLink = regexp.MustCompile(`docent:tour/([^\s#)"'<>\]]+)(?:#([A-Za-z0-9._-]+))?`)

// stepTourRefs lists a step's links into other tours: those in its narrative
// (the body file when there is one, else the description) and its link cards.
func stepTourRefs(t *tour.Tour, index int, body string) []tourRef {
	s := &t.Steps[index]

	field, text := "description", s.Description
	if body != "" {
		field, text = "body", body
	}

	out := refsIn(text, diag.Pointer("steps", index, field), t.StepLabel(index))

	for j := range s.Links {
		out = append(out, refsIn(s.Links[j].URL, diag.Pointer("steps", index, "links", j, "url"), t.StepLabel(index))...)
	}

	return out
}

func refsIn(text, pointer, step string) []tourRef {
	var out []tourRef

	for _, match := range tourLink.FindAllStringSubmatch(text, -1) {
		// A path built from a {{variable}} can't be checked here.
		if strings.Contains(match[1], "{{") {
			continue
		}

		out = append(out, tourRef{pointer: pointer, step: step, path: match[1], id: match[2]})
	}

	return out
}

// neighbors indexes the tours a link from t may lead to, t among them. It is
// nil when there is no way to list them. The index is kept until Reset.
func (v *validator) neighbors(ctx context.Context, t *tour.Tour) ([]neighbor, error) {
	if v.discover == nil {
		return nil, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.indexed {
		paths, err := v.discover(ctx)
		if err != nil {
			return nil, err
		}

		others := make([]neighbor, 0, len(paths))

		for _, path := range paths {
			other, _, loadErr := v.store.Load(ctx, path)
			if loadErr != nil {
				return nil, fmt.Errorf("loading %s: %w", path, loadErr)
			}

			others = append(others, describe(path, other))
		}

		v.others, v.indexed = others, true
	}

	// The tour being checked is described as it is now, and is always there
	// to link to even when it was named outright and not discovered.
	out := make([]neighbor, 0, len(v.others)+1)
	out = append(out, describe(t.Path, t))

	for i := range v.others {
		if v.others[i].path != t.Path {
			out = append(out, v.others[i])
		}
	}

	return out, nil
}

func describe(path string, t *tour.Tour) neighbor {
	n := neighbor{path: path, steps: map[string]struct{}{}}
	if t == nil {
		return n
	}

	n.title, n.readable = t.Title, true

	for i := range t.Steps {
		if t.Steps[i].ID != "" {
			n.steps[t.Steps[i].ID] = struct{}{}
		}
	}

	return n
}

// named reports whether a link's path names a tour: a tour is named by the end
// of its path, so links survive wherever the tours folder lives.
func named(tourPath, link string) bool {
	tourPath, link = filepath.ToSlash(tourPath), filepath.ToSlash(link)

	return tourPath == link || strings.HasSuffix(tourPath, "/"+strings.TrimPrefix(link, "/"))
}

// tourLinkDiagnostics checks a tour's links into other tours, and its `nextTour`.
func tourLinkDiagnostics(t *tour.Tour, refs []tourRef, others []neighbor) []diag.Diagnostic {
	var out []diag.Diagnostic

	for i := range refs {
		ref := &refs[i]

		var matches []*neighbor

		for j := range others {
			if named(others[j].path, ref.path) {
				matches = append(matches, &others[j])
			}
		}

		d := diag.Diagnostic{File: t.Path, Pointer: ref.pointer, Step: ref.step, Severity: diag.SeverityWarning}

		switch {
		case len(matches) == 0:
			// The tour may live in a folder that isn't open here, so this
			// can't be an error.
			d.Code = diag.CodeTourLinkUnknown
			d.Message = fmt.Sprintf("links to the tour %q, which isn't among the tours here", ref.path)
			d.Hint = "A tour is named by the end of its path, like `cache-invalidation.tour`."
		case len(matches) > 1:
			paths := make([]string, 0, len(matches))
			for _, m := range matches {
				paths = append(paths, m.path)
			}

			d.Code = diag.CodeTourLinkAmbiguous
			d.Message = fmt.Sprintf("links to the tour %q, which could be any of %s", ref.path, strings.Join(paths, ", "))
			d.Hint = "Write more of the path, like `billing/overview.tour`."
		case ref.id != "" && matches[0].readable:
			if _, known := matches[0].steps[ref.id]; known {
				continue
			}

			d.Severity = diag.SeverityError
			d.Code = diag.CodeTourLinkStepUnknown
			d.Message = fmt.Sprintf("links to step %q of %s, which that tour doesn't have", ref.id, ref.path)
		default:
			continue
		}

		out = append(out, d)
	}

	if t.NextTour != "" {
		found := false

		for i := range others {
			if others[i].readable && others[i].title == t.NextTour && others[i].path != t.Path {
				found = true
				break
			}
		}

		if !found {
			out = append(out, diag.Diagnostic{
				File:     t.Path,
				Pointer:  diag.Pointer("nextTour"),
				Severity: diag.SeverityWarning,
				Code:     diag.CodeNextTourUnknown,
				Message:  fmt.Sprintf("nextTour names %q, which is the title of no other tour here", t.NextTour),
				Hint:     "`nextTour` is the other tour's `title`, exactly.",
			})
		}
	}

	return out
}
