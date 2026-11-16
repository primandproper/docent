package anchor

import (
	"testing"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

var fixture = []string{
	"package items",   // 1
	"",                // 2
	"func Get() {",    // 3
	"\treturn",        // 4
	"}",               // 5
	"",                // 6
	"func Update() {", // 7
	"\tsave()",        // 8
	"}",               // 9
	"",                // 10
	"func Get2() {",   // 11
	"}",               // 12
}

func sel(startLine, startCol, endLine, endCol int) *tour.Selection {
	return &tour.Selection{
		Start: tour.Position{Line: startLine, Character: startCol},
		End:   tour.Position{Line: endLine, Character: endCol},
	}
}

func TestLocate(T *testing.T) {
	T.Parallel()

	T.Run("pattern at its line is ok", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Update`, Line: 7})
		test.Eq(t, StatusOK, loc.Status)
		test.Eq(t, Range{StartLine: 7, EndLine: 7}, loc.Range)
		test.Eq(t, 1, loc.Matches)
	})

	T.Run("pattern away from its line is moved, with the offset", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Update`, Line: 4})
		test.Eq(t, StatusMoved, loc.Status)
		test.Eq(t, 3, loc.Offset)
		test.Eq(t, diag.CodeAnchorMoved, loc.Problem)
		test.Eq(t, 7, loc.Range.StartLine)
	})

	T.Run("the match nearest to line wins", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Get`, Line: 10})
		test.Eq(t, StatusMoved, loc.Status)
		test.Eq(t, 11, loc.Range.StartLine)
		test.Eq(t, 2, loc.Matches)
	})

	T.Run("several matches and no line is ambiguous, first match shown", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Get`})
		test.Eq(t, StatusAmbiguous, loc.Status)
		test.Eq(t, diag.CodePatternAmbiguous, loc.Problem)
		test.Eq(t, 3, loc.Range.StartLine)
	})

	T.Run("one match and no line is ok", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `save\(\)`})
		test.Eq(t, StatusOK, loc.Status)
		test.Eq(t, 8, loc.Range.StartLine)
	})

	T.Run("no match is missing and falls back to line", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Delete`, Line: 7})
		test.Eq(t, StatusMissing, loc.Status)
		test.Eq(t, diag.CodePatternNoMatch, loc.Problem)
		test.Eq(t, 7, loc.Range.StartLine)
	})

	T.Run("an invalid pattern is missing", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `(?<=x)y`, Line: 3})
		test.Eq(t, StatusMissing, loc.Status)
		test.Eq(t, diag.CodePatternInvalid, loc.Problem)
		test.Eq(t, 3, loc.Range.StartLine)
	})

	T.Run("a selection moves with its pattern", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Update`, Line: 5, Selection: sel(5, 1, 7, 2)})
		test.Eq(t, StatusMoved, loc.Status)
		test.Eq(t, Range{StartLine: 7, StartCol: 1, EndLine: 9, EndCol: 2}, loc.Range)
	})

	T.Run("a selection without a line is measured from its own start", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Pattern: `^func Update`, Selection: sel(1, 1, 3, 2)})
		test.Eq(t, StatusOK, loc.Status)
		test.Eq(t, Range{StartLine: 7, StartCol: 1, EndLine: 9, EndCol: 2}, loc.Range)
	})

	T.Run("a selection without a pattern is absolute", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Line: 3, Selection: sel(3, 6, 5, 2)})
		test.Eq(t, StatusOK, loc.Status)
		test.Eq(t, Range{StartLine: 3, StartCol: 6, EndLine: 5, EndCol: 2}, loc.Range)
	})

	T.Run("a line alone is absolute", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Line: 8})
		test.Eq(t, StatusOK, loc.Status)
		test.Eq(t, Range{StartLine: 8, EndLine: 8}, loc.Range)
	})

	T.Run("a line past the end is missing", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{Line: 99})
		test.Eq(t, StatusMissing, loc.Status)
		test.Eq(t, diag.CodeLineOutOfRange, loc.Problem)
		test.Eq(t, Range{}, loc.Range)
	})

	T.Run("no anchor at all points at the whole file", func(t *testing.T) {
		t.Parallel()

		loc := Locate(fixture, Spec{})
		test.Eq(t, StatusOK, loc.Status)
		test.Eq(t, Range{}, loc.Range)
	})
}

func TestSplitLines(T *testing.T) {
	T.Parallel()

	T.Run("handles terminators", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, []string{"a", "b"}, SplitLines([]byte("a\r\nb\n")))
		test.Eq(t, []string{"a", ""}, SplitLines([]byte("a\n\n")))
		test.Len(t, 0, SplitLines(nil))
	})
}

func TestResolver_Resolve(T *testing.T) {
	T.Parallel()

	src := gitsrc.Memory{gitsrc.Key("/r", "", "a.go"): []byte("package a\n\nfunc A() {}\n")}

	T.Run("reads the file and locates the anchor", func(t *testing.T) {
		t.Parallel()

		res, err := NewResolver(src, nil, nil).Resolve(t.Context(), &Request{Root: "/r", File: "a.go", Spec: Spec{Pattern: `^func A`, Line: 3}})
		must.NoError(t, err)
		test.Eq(t, StatusOK, res.Location.Status)
		test.Eq(t, 3, res.LineCount)
		test.Eq(t, 3, res.Location.Range.StartLine)
	})

	T.Run("a missing file is an answer, not an error", func(t *testing.T) {
		t.Parallel()

		res, err := NewResolver(src, nil, nil).Resolve(t.Context(), &Request{Root: "/r", File: "b.go"})
		must.NoError(t, err)
		test.Eq(t, StatusMissing, res.Location.Status)
		test.Eq(t, diag.CodeFileMissing, res.Location.Problem)
		test.Nil(t, res.Content)
	})

	T.Run("an escaping path is never read", func(t *testing.T) {
		t.Parallel()

		res, err := NewResolver(src, nil, nil).Resolve(t.Context(), &Request{Root: "/r", File: "../etc/passwd"})
		must.NoError(t, err)
		test.Eq(t, diag.CodePathEscapesRoot, res.Location.Problem)
	})
}
