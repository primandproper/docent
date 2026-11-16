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
	}
	store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{tourPath: []byte(tourJSON)}}

	return NewValidator(store, walk.NewWalker(source, nil, nil), nil, nil)
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
