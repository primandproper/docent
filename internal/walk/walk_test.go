package walk

import (
	"testing"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func fixture() (*tour.Tour, gitsrc.FileSource) {
	source := gitsrc.Memory{
		gitsrc.Key("/r", "", "pkg/a.go"):       []byte("package a\n\nfunc A() {}\n"),
		gitsrc.Key("/r", "", "docs/README.md"): []byte("# Docs\n"),
		gitsrc.Key("/r", "", "docs/guide.md"):  []byte("guide\n"),
		gitsrc.Key("/r", "", "bare/x.txt"):     []byte("x\n"),
	}

	t := &tour.Tour{
		Path: "/r/.tours/t.tour",
		Root: "/r",
		Steps: []tour.Step{
			{File: "pkg/a.go", Line: 3, Pattern: "^func A"},
			{Directory: "docs"},
			{URI: "https://example.com"},
			{Description: "just words"},
			{File: "pkg/gone.go"},
			{Directory: "nope"},
			{Directory: "../outside"},
			{Directory: "bare"},
			{Directory: "pkg/a.go"},
		},
	}

	return t, source
}

func TestWalker_Step(T *testing.T) {
	T.Parallel()

	T.Run("resolves a file step", func(t *testing.T) {
		t.Parallel()

		tr, source := fixture()

		step, err := NewWalker(source, nil, nil).Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, KindFile, step.Kind)
		test.Eq(t, "pkg/a.go", step.Path)
		test.Eq(t, anchor.StatusOK, step.Anchor.Location.Status)
		test.Eq(t, 3, step.Anchor.Location.Range.StartLine)
	})

	T.Run("lists a directory step with its README", func(t *testing.T) {
		t.Parallel()

		tr, source := fixture()

		step, err := NewWalker(source, nil, nil).Step(t.Context(), tr, 1)
		must.NoError(t, err)
		test.Eq(t, KindDirectory, step.Kind)
		test.Eq(t, []gitsrc.Entry{{Name: "README.md"}, {Name: "guide.md"}}, step.Entries)
		test.Eq(t, "# Docs\n", step.Readme)
		test.Eq(t, "", string(step.Problem))
	})

	T.Run("a directory without a README still lists", func(t *testing.T) {
		t.Parallel()

		tr, source := fixture()

		step, err := NewWalker(source, nil, nil).Step(t.Context(), tr, 7)
		must.NoError(t, err)
		test.Len(t, 1, step.Entries)
		test.Eq(t, "", step.Readme)
	})

	T.Run("uri and content steps need no code", func(t *testing.T) {
		t.Parallel()

		tr, source := fixture()
		w := NewWalker(source, nil, nil)

		step, err := w.Step(t.Context(), tr, 2)
		must.NoError(t, err)
		test.Eq(t, KindURI, step.Kind)

		step, err = w.Step(t.Context(), tr, 3)
		must.NoError(t, err)
		test.Eq(t, KindContent, step.Kind)
		test.Nil(t, step.Anchor)
	})

	T.Run("missing and escaping targets are answers, not errors", func(t *testing.T) {
		t.Parallel()

		tr, source := fixture()
		w := NewWalker(source, nil, nil)

		step, err := w.Step(t.Context(), tr, 4)
		must.NoError(t, err)
		test.Eq(t, diag.CodeFileMissing, step.Anchor.Location.Problem)

		step, err = w.Step(t.Context(), tr, 5)
		must.NoError(t, err)
		test.Eq(t, diag.CodeFileMissing, step.Problem)

		step, err = w.Step(t.Context(), tr, 6)
		must.NoError(t, err)
		test.Eq(t, diag.CodePathEscapesRoot, step.Problem)

		step, err = w.Step(t.Context(), tr, 8)
		must.NoError(t, err)
		test.Eq(t, diag.CodeFileMissing, step.Problem)
	})

	T.Run("rejects an index outside the tour", func(t *testing.T) {
		t.Parallel()

		tr, source := fixture()
		w := NewWalker(source, nil, nil)

		_, err := w.Step(t.Context(), tr, len(tr.Steps))
		test.ErrorIs(t, err, ErrNoSuchStep)

		_, err = w.Step(t.Context(), tr, -1)
		test.ErrorIs(t, err, ErrNoSuchStep)
	})
}

func TestKindOf(T *testing.T) {
	T.Parallel()

	T.Run("a file wins over a directory, which wins over a uri", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, KindFile, KindOf(&tour.Step{File: "a", Directory: "b", URI: "c"}))
		test.Eq(t, KindDirectory, KindOf(&tour.Step{Directory: "b", URI: "c"}))
		test.Eq(t, KindURI, KindOf(&tour.Step{URI: "c"}))
		test.Eq(t, KindContent, KindOf(&tour.Step{}))
	})
}
