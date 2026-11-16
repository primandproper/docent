package app

import (
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const (
	mainTour  = "/r/.tours/main.tour"
	draftTour = "/r/.tours/draft.tour"
	junkTour  = "/r/.tours/junk.tour"
)

func newService() *Service {
	source := gitsrc.Memory{
		gitsrc.Key("/r", "", "a.go"):       []byte("package a\n\nfunc A() {}\n"),
		gitsrc.Key("/r", "", "bin/blob"):   {0x00, 0x01, 0x02},
		gitsrc.Key("/r", "", "docs/x.md"):  []byte("x\n"),
		gitsrc.Key("/r", "", "big/big.go"): []byte(strings.Repeat("x", maxFileSize+1)),
	}

	store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{
		mainTour: []byte(`{"title": "Main", "isPrimary": true, "description": "The big one", "steps": [
			{ "description": "d", "file": "a.go", "line": 3, "pattern": "^func A" },
			{ "description": "d", "file": "a.go", "line": 1, "pattern": "^func A" },
			{ "description": "d", "file": "gone.go" },
			{ "description": "d", "directory": "docs" },
			{ "description": "d", "file": "bin/blob" },
			{ "description": "d", "file": "big/big.go" },
			{ "description": "d" }
		]}`),
		draftTour: []byte(`{"docent": 1, "title": "Draft", "tags": ["cache"],
			"generatedBy": { "agent": "claude-code", "at": "2026-11-16T10:00:00Z" },
			"steps": [{ "id": "one", "description": "d" }]}`),
		junkTour: []byte(`{nope`),
	}}

	walker := walk.NewWalker(source, nil, nil)

	return NewService("/r", store, walker, validate.NewValidator(store, walker, nil, nil), nil, nil)
}

func TestService_ListTours(T *testing.T) {
	T.Parallel()

	T.Run("summarizes every tour, primary first", func(t *testing.T) {
		t.Parallel()

		tours, err := newService().ListTours(t.Context())
		must.NoError(t, err)
		must.Len(t, 3, tours)

		test.Eq(t, TourSummary{
			Path: mainTour, DisplayPath: ".tours/main.tour", Title: "Main", Description: "The big one",
			Tags: []string{}, Steps: 7, Errors: 1, Warnings: 1, IsPrimary: true,
		}, tours[0])

		test.Eq(t, "Draft", tours[1].Title)
		test.True(t, tours[1].Unreviewed)
		test.Eq(t, []string{"cache"}, tours[1].Tags)
		test.Eq(t, 0, tours[1].Errors)

		test.True(t, tours[2].Unreadable)
		test.Eq(t, "junk.tour", tours[2].Title)
		test.Eq(t, 1, tours[2].Errors)
	})
}

func TestService_OpenTour(T *testing.T) {
	T.Parallel()

	T.Run("returns the tour with a status per step", func(t *testing.T) {
		t.Parallel()

		view, err := newService().OpenTour(t.Context(), mainTour)
		must.NoError(t, err)
		test.Eq(t, "Main", view.Tour.Title)
		test.Len(t, 0, view.Diagnostics)
		test.NotNil(t, view.Diagnostics)

		statuses := make([]anchor.Status, len(view.Steps))
		for i, s := range view.Steps {
			statuses[i] = s.Status
		}

		test.Eq(t, []anchor.Status{
			anchor.StatusOK, anchor.StatusMoved, anchor.StatusMissing, anchor.StatusOK,
			anchor.StatusOK, anchor.StatusOK, anchor.StatusOK,
		}, statuses)
		test.Eq(t, walk.KindDirectory, view.Steps[3].Kind)
		test.Eq(t, walk.KindContent, view.Steps[6].Kind)
	})

	T.Run("refuses a path the workspace didn't discover", func(t *testing.T) {
		t.Parallel()

		_, err := newService().OpenTour(t.Context(), "/etc/passwd")
		test.ErrorIs(t, err, ErrUnknownTour)
	})

	T.Run("an unparseable tour is an error that says why", func(t *testing.T) {
		t.Parallel()

		_, err := newService().OpenTour(t.Context(), junkTour)
		test.ErrorIs(t, err, ErrTourUnreadable)
	})
}

func TestService_Step(T *testing.T) {
	T.Parallel()

	T.Run("returns the file and where the anchor landed", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 0)
		must.NoError(t, err)
		test.Eq(t, walk.KindFile, step.Kind)
		test.Eq(t, "a.go", step.Path)
		test.Eq(t, "package a\n\nfunc A() {}\n", step.Content)
		test.Eq(t, anchor.Range{StartLine: 3, EndLine: 3}, step.Location.Range)
		test.Eq(t, 0, step.Index)
		test.NotNil(t, step.Entries)
	})

	T.Run("a missing file carries its reason", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 2)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusMissing, step.Location.Status)
		test.Eq(t, diag.CodeFileMissing, step.Location.Problem)
		test.Eq(t, "", step.Content)
	})

	T.Run("a directory step lists its entries", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 3)
		must.NoError(t, err)
		test.Eq(t, []gitsrc.Entry{{Name: "x.md"}}, step.Entries)
		test.Eq(t, anchor.StatusOK, step.Location.Status)
	})

	T.Run("binary and oversized files are withheld, with a reason", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 4)
		must.NoError(t, err)
		test.Eq(t, "", step.Content)
		test.Eq(t, "This is a binary file.", step.Unavailable)

		step, err = s.Step(t.Context(), mainTour, 5)
		must.NoError(t, err)
		test.Eq(t, "", step.Content)
		test.StrContains(t, step.Unavailable, "too large")
	})

	T.Run("refuses a tour that isn't open", func(t *testing.T) {
		t.Parallel()

		_, err := newService().Step(t.Context(), mainTour, 0)
		test.ErrorIs(t, err, ErrUnknownTour)
	})

	T.Run("refuses an index outside the tour", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		_, err = s.Step(t.Context(), mainTour, 99)
		test.ErrorIs(t, err, walk.ErrNoSuchStep)

		_, err = s.Step(t.Context(), mainTour, -1)
		test.ErrorIs(t, err, walk.ErrNoSuchStep)
	})
}

func TestService_Workspace(T *testing.T) {
	T.Parallel()

	T.Run("names the opened folder", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, Workspace{Root: "/r", Name: "r"}, newService().Workspace())
	})
}
