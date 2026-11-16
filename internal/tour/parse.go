package tour

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/orderedjson"
)

var (
	tourExtensionFields = []string{"repos", "vars", "map", "tags", "generatedBy", "reviewedBy"}
	stepExtensionFields = []string{"id", "repo", "body", "anchors", "mapFocus", "links", "visual", "layout", "ref", "contentHash"}
)

// Parse turns the bytes of a tour file into a Tour plus whatever is wrong with
// it. The Tour is nil only when the bytes aren't a JSON object at all; a
// document with schema errors still comes back, decoded as far as it can be,
// so the app can show a broken tour instead of nothing.
func Parse(path string, data []byte) (*Tour, []diag.Diagnostic, error) {
	doc, err := orderedjson.Parse(data)
	if err != nil {
		// A syntax error is a finding about the tour, not a failure to check it.
		//nolint:nilerr // reported as a diagnostic
		return nil, []diag.Diagnostic{{
			File:     path,
			Severity: diag.SeverityError,
			Code:     diag.CodeJSONSyntax,
			Message:  err.Error(),
			Hint:     "A tour is strict JSON: no comments and no trailing commas.",
		}}, nil
	}

	diags, err := validateSchema(path, doc)
	if err != nil {
		return nil, nil, err
	}

	if doc.Kind != orderedjson.KindObject {
		return nil, diags, nil
	}

	t := &Tour{
		Raw:         doc,
		Path:        path,
		Title:       str(doc, "title"),
		Description: str(doc, "description"),
		Ref:         str(doc, "ref"),
		NextTour:    str(doc, "nextTour"),
		IsPrimary:   boolean(doc, "isPrimary"),
	}

	if integer(doc, "docent") == FormatVersion {
		t.Format = FormatVersion
		decodeTourExtensions(t, doc)
	} else {
		diags = append(diags, ignoredExtensions(path, doc, "", tourExtensionFields)...)
	}

	seen := map[string]int{}

	for i, raw := range elems(doc, "steps") {
		if raw.Kind != orderedjson.KindObject {
			t.Steps = append(t.Steps, Step{})
			continue
		}

		step := Step{
			Title:       str(raw, "title"),
			Description: str(raw, "description"),
			File:        str(raw, "file"),
			Directory:   str(raw, "directory"),
			URI:         str(raw, "uri"),
			Pattern:     str(raw, "pattern"),
			Line:        integer(raw, "line"),
			Selection:   selection(raw.Get("selection")),
		}

		if t.IsDocent() {
			decodeStepExtensions(&step, raw)

			if first, dup := seen[step.ID]; dup && step.ID != "" {
				diags = append(diags, diag.Diagnostic{
					File:     path,
					Pointer:  diag.Pointer("steps", i, "id"),
					Step:     step.ID,
					Severity: diag.SeverityError,
					Code:     diag.CodeStepIDDuplicate,
					Message:  fmt.Sprintf("step id %q is already used by step #%d", step.ID, first+1),
					Hint:     "Step ids must be unique within a tour: deep links and live reload rely on them.",
				})
			} else {
				seen[step.ID] = i
			}
		} else {
			diags = append(diags, ignoredExtensions(path, raw, stepNumber(i), stepExtensionFields, "steps", i)...)
		}

		t.Steps = append(t.Steps, step)
	}

	return t, diags, nil
}

func ignoredExtensions(path string, obj *orderedjson.Value, step string, fields []string, prefix ...any) []diag.Diagnostic {
	var out []diag.Diagnostic

	for _, field := range fields {
		if obj.Get(field) == nil {
			continue
		}

		out = append(out, diag.Diagnostic{
			File:     path,
			Pointer:  diag.Pointer(append(prefix, field)...),
			Step:     step,
			Severity: diag.SeverityWarning,
			Code:     diag.CodeExtensionIgnored,
			Message:  fmt.Sprintf("%q is a Docent extension field, and this tour doesn't set `docent`, so it is ignored", field),
			Hint:     `Add "docent": 1 at the top of the tour to use extension fields.`,
		})
	}

	return out
}

func decodeTourExtensions(t *Tour, doc *orderedjson.Value) {
	for _, m := range members(doc, "repos") {
		t.Repos = append(t.Repos, Repo{
			Name:   m.Key,
			Remote: str(m.Value, "remote"),
			Ref:    str(m.Value, "ref"),
			Stamp:  str(m.Value, "stamp"),
			Subdir: str(m.Value, "subdir"),
		})
	}

	for _, m := range members(doc, "vars") {
		t.Vars = append(t.Vars, Var{
			Name:        m.Key,
			Default:     str(m.Value, "default"),
			Description: str(m.Value, "description"),
			Choices:     strs(m.Value, "choices"),
		})
	}

	if m := doc.Get("map"); m != nil && m.Kind == orderedjson.KindObject {
		t.Map = &Map{Mermaid: str(m, "mermaid"), File: str(m, "file")}
	}

	t.Tags = strs(doc, "tags")

	if g := doc.Get("generatedBy"); g != nil && g.Kind == orderedjson.KindObject {
		t.GeneratedBy = &GeneratedBy{Agent: str(g, "agent"), Model: str(g, "model"), At: str(g, "at")}
	}

	for _, r := range elems(doc, "reviewedBy") {
		t.ReviewedBy = append(t.ReviewedBy, Review{Name: str(r, "name"), At: str(r, "at"), Ref: str(r, "ref")})
	}
}

func decodeStepExtensions(step *Step, raw *orderedjson.Value) {
	step.ID = str(raw, "id")
	step.Repo = str(raw, "repo")
	step.Body = str(raw, "body")
	step.Layout = str(raw, "layout")
	step.Ref = str(raw, "ref")
	step.ContentHash = str(raw, "contentHash")
	step.MapFocus = strs(raw, "mapFocus")

	for _, a := range elems(raw, "anchors") {
		step.Anchors = append(step.Anchors, Anchor{
			Label:       str(a, "label"),
			Repo:        str(a, "repo"),
			File:        str(a, "file"),
			Pattern:     str(a, "pattern"),
			ContentHash: str(a, "contentHash"),
			Line:        integer(a, "line"),
			Selection:   selection(a.Get("selection")),
		})
	}

	for _, l := range elems(raw, "links") {
		step.Links = append(step.Links, Link{
			Kind:        str(l, "kind"),
			Title:       str(l, "title"),
			URL:         str(l, "url"),
			Description: str(l, "description"),
		})
	}

	if v := raw.Get("visual"); v != nil && v.Kind == orderedjson.KindObject {
		if encoded, err := json.Marshal(v.Interface()); err == nil {
			step.Visual = encoded
		}
	}
}

// The helpers below read a field only when it has the expected type. A wrong
// type is the schema's to report; decoding just leaves the zero value.

func str(obj *orderedjson.Value, key string) string {
	if v := obj.Get(key); v != nil && v.Kind == orderedjson.KindString {
		return v.Str
	}

	return ""
}

func boolean(obj *orderedjson.Value, key string) bool {
	if v := obj.Get(key); v != nil && v.Kind == orderedjson.KindBool {
		return v.Bool
	}

	return false
}

func integer(obj *orderedjson.Value, key string) int {
	v := obj.Get(key)
	if v == nil || v.Kind != orderedjson.KindNumber {
		return 0
	}

	n, err := strconv.Atoi(v.Num.String())
	if err != nil {
		return 0
	}

	return n
}

func elems(obj *orderedjson.Value, key string) []*orderedjson.Value {
	if v := obj.Get(key); v != nil && v.Kind == orderedjson.KindArray {
		return v.Elems
	}

	return nil
}

func members(obj *orderedjson.Value, key string) []orderedjson.Member {
	if v := obj.Get(key); v != nil && v.Kind == orderedjson.KindObject {
		return v.Members
	}

	return nil
}

func strs(obj *orderedjson.Value, key string) []string {
	var out []string

	for _, e := range elems(obj, key) {
		if e.Kind == orderedjson.KindString {
			out = append(out, e.Str)
		}
	}

	return out
}

func selection(v *orderedjson.Value) *Selection {
	if v == nil || v.Kind != orderedjson.KindObject {
		return nil
	}

	start, end := v.Get("start"), v.Get("end")
	if start == nil || end == nil {
		return nil
	}

	return &Selection{
		Start: Position{Line: integer(start, "line"), Character: integer(start, "character")},
		End:   Position{Line: integer(end, "line"), Character: integer(end, "character")},
	}
}
