package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/gitsrc/gittest"
	"github.com/primandproper/docent/internal/orderedjson"
	"github.com/primandproper/docent/internal/walk"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const reviewedCode = "package worker\n\nfunc poll() {\n}\n\nfunc handle() {\n}\n"

// reviewWorkspace is a real workspace: a tours folder beside a `worker`
// repository, with a draft tour of two steps that both resolve.
func reviewWorkspace(t *testing.T) (s *Service, repo *gittest.Repo, tourPath string) {
	t.Helper()

	top, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	must.NoError(t, os.MkdirAll(filepath.Join(top, "worker"), 0o750))
	repo = gittest.At(t, filepath.Join(top, "worker"))
	repo.Commit("add worker", map[string]string{"worker.go": reviewedCode})

	root := filepath.Join(top, "tours")
	tourPath = filepath.Join(root, ".tours", "worker.tour")
	must.NoError(t, os.MkdirAll(filepath.Dir(tourPath), 0o750))
	// The name offered is the one git has for the repo the tour is in, set
	// here so the test never reads the machine's own git config.
	gittest.At(t, root).Git("config", "user.name", "Ada Reader")
	must.NoError(t, os.WriteFile(tourPath, []byte(`{
  "docent": 1, "title": "The worker",
  "repos": {"worker": {"remote": "git@github.com:acme/worker.git", "ref": "main"}},
  "generatedBy": {"agent": "test", "at": "2026-11-16T10:00:00Z"},
  "steps": [
    {"id": "poll", "description": "d", "repo": "worker", "file": "worker.go", "pattern": "^func poll", "line": 3},
    {"id": "handle", "description": "d", "repo": "worker", "file": "worker.go", "pattern": "^func handle", "line": 6}
  ]
}`), 0o600))

	services, err := core.New(&core.Options{Root: root, UserConfigPath: filepath.Join(top, "config.json")})
	must.NoError(t, err)
	t.Cleanup(services.Close)

	return NewService(root, services, &fakeLauncher{}, nil, nil), repo, tourPath
}

func reviewsOf(t *testing.T, path string) []any {
	t.Helper()

	data, err := os.ReadFile(path)
	must.NoError(t, err)

	var doc map[string]any
	must.NoError(t, json.Unmarshal(data, &doc))

	reviews, _ := doc["reviewedBy"].([]any)

	return reviews
}

func TestService_Review(T *testing.T) {
	T.Parallel()

	T.Run("is offered only once every step has been read", func(t *testing.T) {
		t.Parallel()

		s, repo, tourPath := reviewWorkspace(t)

		view, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		test.True(t, view.Review.Available)
		test.False(t, view.Review.Ready)
		test.Eq(t, "Ada Reader", view.Review.Name)
		test.Eq(t, []bool{false, false}, view.Review.Read)
		test.Eq(t, "Read every step first.", view.Review.Blocker)

		test.ErrorIs(t, s.Review(t.Context(), tourPath, "Ada Reader"), ErrNotRead)

		state, err := s.MarkRead(t.Context(), tourPath, 1)
		must.NoError(t, err)
		test.False(t, state.Ready)
		test.Eq(t, "Read step 1 first.", state.Blocker)

		test.ErrorIs(t, s.Review(t.Context(), tourPath, "Ada Reader"), ErrNotRead)
		test.Len(t, 0, reviewsOf(t, tourPath))

		state, err = s.MarkRead(t.Context(), tourPath, 0)
		must.NoError(t, err)
		test.True(t, state.Ready)
		test.Eq(t, "", state.Blocker)

		must.NoError(t, s.Review(t.Context(), tourPath, "  Ada Reader "))

		reviews := reviewsOf(t, tourPath)
		must.Len(t, 1, reviews)

		review, _ := reviews[0].(map[string]any)
		test.Eq[any](t, "Ada Reader", review["name"])
		test.Eq[any](t, repo.Git("rev-parse", "HEAD"), review["ref"])

		// The review changed the file, so the next one starts from nothing.
		view, err = s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 1, view.Tour.ReviewedBy)
		test.Eq(t, []bool{false, false}, view.Review.Read)
		test.NotEq(t, "", view.Tour.Repos[0].Stamp)
	})

	T.Run("reopening an unchanged tour keeps what was read", func(t *testing.T) {
		t.Parallel()

		s, _, tourPath := reviewWorkspace(t)

		_, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		_, err = s.MarkRead(t.Context(), tourPath, 0)
		must.NoError(t, err)

		view, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Eq(t, []bool{true, false}, view.Review.Read)
	})

	T.Run("an edit to the tour starts the read again", func(t *testing.T) {
		t.Parallel()

		s, _, tourPath := reviewWorkspace(t)

		_, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)

		for i := range 2 {
			_, err = s.MarkRead(t.Context(), tourPath, i)
			must.NoError(t, err)
		}

		data, err := os.ReadFile(tourPath)
		must.NoError(t, err)

		raw, err := orderedjson.Parse(data)
		must.NoError(t, err)
		raw.Set("title", orderedjson.String("The worker, rewritten"))

		edited, err := raw.Format()
		must.NoError(t, err)
		must.NoError(t, os.WriteFile(tourPath, edited, 0o600))

		// Read through, but not this text: the app hasn't even reopened it.
		test.ErrorIs(t, s.Review(t.Context(), tourPath, "Ada Reader"), ErrNotRead)

		view, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Eq(t, []bool{false, false}, view.Review.Read)
		test.Len(t, 0, reviewsOf(t, tourPath))
	})

	T.Run("the working tree isn't what a review vouches for", func(t *testing.T) {
		t.Parallel()

		s, _, tourPath := reviewWorkspace(t)

		_, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		_, err = s.MarkRead(t.Context(), tourPath, 0)
		must.NoError(t, err)

		s.SetWorkingTree(true)

		view, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Eq(t, []bool{false, false}, view.Review.Read)
		test.StrContains(t, view.Review.Blocker, "Working tree")

		for i := range 2 {
			_, err = s.MarkRead(t.Context(), tourPath, i)
			must.NoError(t, err)
		}

		test.ErrorIs(t, s.Review(t.Context(), tourPath, "Ada Reader"), ErrNotRead)
		test.Len(t, 0, reviewsOf(t, tourPath))
	})

	T.Run("a drifted anchor blocks it however much was read", func(t *testing.T) {
		t.Parallel()

		s, repo, tourPath := reviewWorkspace(t)
		repo.Commit("drop handle", map[string]string{"worker.go": "package worker\n\nfunc poll() {\n}\n"})

		view, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Eq(t, "1 anchor doesn't resolve cleanly. Fix it first.", view.Review.Blocker)

		for i := range 2 {
			var state *ReviewState

			state, err = s.MarkRead(t.Context(), tourPath, i)
			must.NoError(t, err)
			test.False(t, state.Ready)
		}

		test.Error(t, s.Review(t.Context(), tourPath, "Ada Reader"))
		test.Len(t, 0, reviewsOf(t, tourPath))
	})

	T.Run("refuses a name it can't sign with", func(t *testing.T) {
		t.Parallel()

		s, _, tourPath := reviewWorkspace(t)

		_, err := s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)

		for i := range 2 {
			_, err = s.MarkRead(t.Context(), tourPath, i)
			must.NoError(t, err)
		}

		for _, name := range []string{"", "   ", "Ada\nReader", string(make([]byte, maxNameLength+1))} {
			test.ErrorIs(t, s.Review(t.Context(), tourPath, name), ErrBadName)
		}

		test.Len(t, 0, reviewsOf(t, tourPath))
	})

	T.Run("refuses tours and steps it doesn't have open", func(t *testing.T) {
		t.Parallel()

		s, _, tourPath := reviewWorkspace(t)

		_, err := s.MarkRead(t.Context(), tourPath, 0)
		test.ErrorIs(t, err, ErrUnknownTour)
		test.ErrorIs(t, s.Review(t.Context(), tourPath, "Ada Reader"), ErrUnknownTour)

		_, err = s.OpenTour(t.Context(), tourPath)
		must.NoError(t, err)

		for _, index := range []int{-1, 2} {
			_, err = s.MarkRead(t.Context(), tourPath, index)
			test.ErrorIs(t, err, walk.ErrNoSuchStep)
		}
	})

	T.Run("a plain CodeTour file has no review to give", func(t *testing.T) {
		t.Parallel()

		s := newService()

		view, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)
		test.False(t, view.Review.Available)
		test.False(t, view.Review.Ready)

		for i := range view.Tour.Steps {
			_, err = s.MarkRead(t.Context(), mainTour, i)
			must.NoError(t, err)
		}

		test.Error(t, s.Review(t.Context(), mainTour, "Ada Reader"))
	})
}
