package stamp

import (
	"errors"
	"testing"

	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func parse(t *testing.T, text string) *tour.Tour {
	t.Helper()

	parsed, _, err := tour.Parse("/r/.tours/a.tour", []byte(text))
	must.NoError(t, err)
	must.NotNil(t, parsed)

	// The store sets Root when it loads a tour from disk.
	parsed.Root = "/r"

	return parsed
}

func TestApply(T *testing.T) {
	T.Parallel()

	source := gitsrc.Memory{gitsrc.Key("/r", "", "a.go"): []byte("package a\n\nfunc A() {}\n")}
	walker := walk.NewWalker(&walk.Dependencies{Source: source})

	T.Run("refuses a plain CodeTour file", func(t *testing.T) {
		t.Parallel()

		plain := parse(t, `{"title": "Plain", "steps": [{"description": "d", "file": "a.go", "line": 3}]}`)

		_, err := Apply(t.Context(), walker, plain, &Options{})
		test.ErrorIs(t, err, ErrPlainTour)
	})

	T.Run("refuses while an anchor has drifted, and names it", func(t *testing.T) {
		t.Parallel()

		drifted := parse(t, `{"docent": 1, "title": "Drifted", "steps": [
			{"id": "ok", "description": "d", "file": "a.go", "line": 3},
			{"id": "gone", "description": "d", "file": "gone.go", "line": 1}
		]}`)

		_, err := Apply(t.Context(), walker, drifted, &Options{})

		var drift *DriftError
		must.True(t, errors.As(err, &drift))
		must.Len(t, 1, drift.Anchors)
		test.StrContains(t, drift.Anchors[0], "gone")
		test.Nil(t, drifted.Raw.Get("reviewedBy"))
	})

	T.Run("records a review on a tour whose anchors resolve", func(t *testing.T) {
		t.Parallel()

		clean := parse(t, `{"docent": 1, "title": "Clean", "steps": [{"id": "a", "description": "d", "file": "a.go", "line": 3}]}`)

		result, err := Apply(t.Context(), walker, clean, &Options{Hashes: true, Review: &tour.Review{Name: "Ada", At: "2026-11-16T10:00:00Z"}})
		must.NoError(t, err)
		test.Len(t, 0, result.Moves)

		reviews := clean.Raw.Get("reviewedBy")
		must.NotNil(t, reviews)
		must.Len(t, 1, reviews.Elems)
		test.NotNil(t, clean.Raw.Get("steps").Elems[0].Get("contentHash"))
	})
}
