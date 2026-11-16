package validate

import (
	"testing"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const tourPath = "/r/.tours/t.tour"

func newValidator(tourJSON string) Validator {
	source := gitsrc.Memory{
		gitsrc.Key("/r", "", "a.go"):        []byte("package a\n\nfunc A() {}\n\nfunc B() {}\n"),
		gitsrc.Key("/r", "", "docs/x.md"):   []byte("x\n"),
		gitsrc.Key("/r", "", ".tours/keep"): []byte(""),

		gitsrc.Key("/r/.tours", "", "t/map.mmd"):   []byte("flowchart LR\n  api e1@--> queue\n  queue --> worker\n"),
		gitsrc.Key("/r/.tours", "", "t/notmap.md"): []byte("sequenceDiagram\n  a->>b: hi\n"),
	}
	store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{tourPath: []byte(tourJSON)}}

	return NewValidator(store, walk.NewWalker(&walk.Dependencies{Source: source}), nil, nil)
}

func TestValidator_Map(T *testing.T) {
	T.Parallel()

	T.Run("mapFocus may name nodes and named edges, inline or from a file", func(t *testing.T) {
		t.Parallel()

		for _, m := range []string{`{ "file": "t/map.mmd" }`, `{ "mermaid": "flowchart LR\n  api e1@--> queue --> worker" }`} {
			diags, err := newValidator(`{"docent": 1, "title": "t", "map": `+m+`, "steps": [
				{ "id": "a", "description": "d", "mapFocus": ["api", "e1", "worker"] }
			]}`).Tour(t.Context(), tourPath)
			must.NoError(t, err)
			test.Len(t, 0, diags, test.Sprintf("%s: %+v", m, diags))
		}
	})

	T.Run("an id the map doesn't define is an error, where it is", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "map": { "file": "t/map.mmd" }, "steps": [
			{ "id": "a", "description": "d", "mapFocus": ["api", "cache"] }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)
		test.Eq(t, diag.CodeMapFocusUnknown, diags[0].Code)
		test.Eq(t, "/steps/0/mapFocus/1", diags[0].Pointer)
		test.Eq(t, "a", diags[0].Step)
	})

	T.Run("a map that is missing or not a flowchart is reported once", func(t *testing.T) {
		t.Parallel()

		for _, file := range []string{"t/gone.mmd", "t/notmap.md", "../../etc/passwd"} {
			diags, err := newValidator(`{"docent": 1, "title": "t", "map": { "file": "`+file+`" }, "steps": [
				{ "id": "a", "description": "d", "mapFocus": ["api"] }
			]}`).Tour(t.Context(), tourPath)
			must.NoError(t, err)
			must.Len(t, 1, diags, must.Sprint(file))
			test.Eq(t, "/map", diags[0].Pointer)
		}
	})

	T.Run("mapFocus without a map is an error", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d", "mapFocus": ["api"] }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)
		test.StrContains(t, diags[0].Message, "the tour has no `map`")
	})
}

func TestValidator_Tour(T *testing.T) {
	T.Parallel()

	T.Run("a tour whose anchors all resolve is clean", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"title": "t", "steps": [
			{ "description": "d", "file": "a.go", "line": 3, "pattern": "^func A" },
			{ "description": "d", "file": "a.go", "line": 5 },
			{ "description": "d", "file": "a.go", "selection": { "start": { "line": 3, "character": 1 }, "end": { "line": 5, "character": 4 } } },
			{ "description": "d", "directory": "docs" },
			{ "description": "d", "uri": "https://example.com" },
			{ "description": "d" }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 0, diags, test.Sprintf("%+v", diags))
	})

	T.Run("reports each kind of broken anchor where it is", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "gone", "description": "d", "file": "gone.go" },
			{ "id": "escape", "description": "d", "file": "../../etc/passwd" },
			{ "id": "far", "description": "d", "file": "a.go", "line": 99 },
			{ "id": "regex", "description": "d", "file": "a.go", "pattern": "(?<=x)y" },
			{ "id": "nomatch", "description": "d", "file": "a.go", "pattern": "^func C" },
			{ "id": "twice", "description": "d", "file": "a.go", "pattern": "^func " },
			{ "id": "moved", "description": "d", "file": "a.go", "pattern": "^func B", "line": 3 },
			{ "id": "nodir", "description": "d", "directory": "nope" },
			{ "id": "sel", "description": "d", "file": "a.go", "selection": { "start": { "line": 40, "character": 1 }, "end": { "line": 41, "character": 1 } } }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)

		type finding struct {
			step     string
			code     diag.Code
			pointer  string
			severity diag.Severity
		}

		got := make([]finding, len(diags))
		for i, d := range diags {
			got[i] = finding{d.Step, d.Code, d.Pointer, d.Severity}

			test.Eq(t, tourPath, d.File)
			test.NotEq(t, "", d.Message)
		}

		test.Eq(t, []finding{
			{"gone", diag.CodeFileMissing, "/steps/0/file", diag.SeverityError},
			{"escape", diag.CodePathEscapesRoot, "/steps/1/file", diag.SeverityError},
			{"far", diag.CodeLineOutOfRange, "/steps/2/line", diag.SeverityError},
			{"regex", diag.CodePatternInvalid, "/steps/3/pattern", diag.SeverityError},
			{"nomatch", diag.CodePatternNoMatch, "/steps/4/pattern", diag.SeverityError},
			{"twice", diag.CodePatternAmbiguous, "/steps/5/pattern", diag.SeverityError},
			{"moved", diag.CodeAnchorMoved, "/steps/6/line", diag.SeverityWarning},
			{"nodir", diag.CodeFileMissing, "/steps/7/directory", diag.SeverityError},
			{"sel", diag.CodeLineOutOfRange, "/steps/8/selection", diag.SeverityError},
		}, got)
	})

	T.Run("parse diagnostics come first and steps are still checked", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"title": "t", "tags": ["x"], "steps": [
			{ "description": "d", "file": "gone.go" }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 2, diags)
		test.Eq(t, diag.CodeExtensionIgnored, diags[0].Code)
		test.Eq(t, diag.CodeFileMissing, diags[1].Code)
		test.Eq(t, "#1", diags[1].Step)
	})

	T.Run("an unparseable tour reports only that", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{nope`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)
		test.Eq(t, diag.CodeJSONSyntax, diags[0].Code)
	})
}
