package walk

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/gitsrc/gittest"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// oneRepo resolves every repo to the same checkout.
type oneRepo struct{ root string }

func (o oneRepo) Resolve(context.Context, *tour.Repo) (*workspace.Checkout, error) {
	return &workspace.Checkout{Found: true, Root: o.root, How: workspace.ByConvention}, nil
}

func gitWalker(t *testing.T, root string) Walker {
	t.Helper()

	git := gitsrc.NewGit(nil, nil)
	t.Cleanup(git.Close)

	return NewWalker(&Dependencies{Source: git, Git: git, Repos: oneRepo{root: root}})
}

func repoTour(repo *tour.Repo, steps ...tour.Step) *tour.Tour {
	repo.Name = "api"

	for i := range steps {
		steps[i].Repo = "api"
	}

	return &tour.Tour{Path: "/t/.tours/t.tour", Root: "/t", Format: 1, Repos: []tour.Repo{*repo}, Steps: steps}
}

func TestWalker_Refs(T *testing.T) {
	T.Parallel()

	T.Run("a branch is tracked at its remote tip, not the checkout", func(t *testing.T) {
		t.Parallel()

		upstream := gittest.New(t)
		upstream.Commit("one", map[string]string{"a.go": "v1\n"})

		clone := filepath.Join(t.TempDir(), "clone")
		git := gitsrc.NewGit(nil, nil)
		t.Cleanup(git.Close)
		must.NoError(t, git.Clone(t.Context(), upstream.Dir, clone))

		second := upstream.Commit("two", map[string]string{"a.go": "v2\n"})
		must.NoError(t, git.Fetch(t.Context(), clone))
		gittest.Attach(t, clone).Write(map[string]string{"a.go": "local edit\n"})

		w := gitWalker(t, clone)
		tr := repoTour(&tour.Repo{Ref: "main"}, tour.Step{File: "a.go"})

		step, err := w.Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, "v2\n", string(step.Anchor.Content))
		test.Eq(t, &Ref{Requested: "main", Kind: RefTracking, Name: "origin/main", Commit: second}, step.Ref)

		// The reader's toggle shows what is on disk instead.
		w.SetWorkingTree(true)

		step, err = w.Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, "local edit\n", string(step.Anchor.Content))
		test.Eq(t, RefWorkingTree, step.Ref.Kind)
	})

	T.Run("a commit or tag is pinned, and a step can pin its own", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		first := repo.Commit("one", map[string]string{"a.go": "v1\n"})
		repo.Git("tag", "v1.0.0")
		second := repo.Commit("two", map[string]string{"a.go": "v2\n"})

		w := gitWalker(t, repo.Dir)
		tr := repoTour(&tour.Repo{Ref: "v1.0.0"}, tour.Step{File: "a.go"}, tour.Step{File: "a.go", Ref: second[:10]})

		step, err := w.Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, "v1\n", string(step.Anchor.Content))
		test.Eq(t, RefPinned, step.Ref.Kind)
		test.Eq(t, first, step.Ref.Commit)

		step, err = w.Step(t.Context(), tr, 1)
		must.NoError(t, err)
		test.Eq(t, "v2\n", string(step.Anchor.Content))
		test.Eq(t, second, step.Ref.Commit)
	})

	T.Run("a branch that has gone falls back to the default branch and says so", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		repo.Commit("one", map[string]string{"a.go": "v1\n"})

		step, err := gitWalker(t, repo.Dir).Step(t.Context(), repoTour(&tour.Repo{Ref: "feature/merged"}, tour.Step{File: "a.go"}), 0)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusOK, step.Anchor.Location.Status)
		test.Eq(t, diag.CodeRefFallback, step.Ref.Problem)
		test.Eq(t, "main", step.Ref.Name)
		test.StrContains(t, step.Ref.Detail, `"feature/merged" no longer exists`)
	})

	T.Run("a pinned commit that isn't here is missing", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		repo.Commit("one", map[string]string{"a.go": "v1\n"})

		step, err := gitWalker(t, repo.Dir).Step(t.Context(), repoTour(&tour.Repo{Ref: "deadbeef1234"}, tour.Step{File: "a.go"}), 0)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusMissing, step.Anchor.Location.Status)
		test.Eq(t, diag.CodeRefMissing, step.Anchor.Location.Problem)
		test.Nil(t, step.Anchor.Content)
	})

	T.Run("a folder that isn't a repo is read from disk", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		gittest.Attach(t, dir).Write(map[string]string{"a.go": "plain\n"})

		step, err := gitWalker(t, dir).Step(t.Context(), repoTour(&tour.Repo{Ref: "main"}, tour.Step{File: "a.go"}), 0)
		must.NoError(t, err)
		test.Eq(t, "plain\n", string(step.Anchor.Content))
		test.Eq(t, RefWorkingTree, step.Ref.Kind)
	})

	T.Run("a directory step is listed at the ref", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		repo.Commit("one", map[string]string{"docs/README.md": "# Docs\n", "docs/a.md": ""})
		repo.Write(map[string]string{"docs/uncommitted.md": ""})

		step, err := gitWalker(t, repo.Dir).Step(t.Context(), repoTour(&tour.Repo{Ref: "main"}, tour.Step{Directory: "docs"}), 0)
		must.NoError(t, err)
		test.Eq(t, []gitsrc.Entry{{Name: "README.md"}, {Name: "a.md"}}, step.Entries)
		test.Eq(t, "# Docs\n", step.Readme)
	})
}

func TestWalker_Stamps(T *testing.T) {
	T.Parallel()

	T.Run("an anchor is followed from the stamp to the tip", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		stamp := repo.Commit("one", map[string]string{"a.go": "package a\n\nfunc A() {}\n"})
		repo.Commit("two", map[string]string{"a.go": "package a\n\n// new\n// lines\n\nfunc A() {}\n"})

		w := gitWalker(t, repo.Dir)
		tr := repoTour(&tour.Repo{Ref: "main", Stamp: stamp[:7]}, tour.Step{File: "a.go", Line: 3})

		step, err := w.Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusMoved, step.Anchor.Location.Status)
		test.Eq(t, 6, step.Anchor.Location.Range.StartLine)

		state, err := w.Repo(t.Context(), tr, "api")
		must.NoError(t, err)
		test.True(t, state.Found)
		test.Eq(t, stamp, state.Stamp)
		test.Eq(t, 1, state.Behind)
		test.False(t, state.StampedAt.IsZero())
		test.Eq(t, "main", state.Ref.Name)
	})

	T.Run("a stamp that isn't here leaves the anchor unverified", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		repo.Commit("one", map[string]string{"a.go": "package a\n\nfunc A() {}\n"})

		w := gitWalker(t, repo.Dir)
		tr := repoTour(&tour.Repo{Ref: "main", Stamp: "0123abc"},
			tour.Step{File: "a.go", Pattern: "^func A", Line: 3},
			tour.Step{File: "a.go", Pattern: "^func Gone"},
		)

		step, err := w.Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusUnverified, step.Anchor.Location.Status)
		test.Eq(t, diag.CodeStampUnreachable, step.Anchor.Location.Problem)
		test.Eq(t, 3, step.Anchor.Location.Range.StartLine)

		step, err = w.Step(t.Context(), tr, 1)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusMissing, step.Anchor.Location.Status)

		state, err := w.Repo(t.Context(), tr, "api")
		must.NoError(t, err)
		test.True(t, state.StampUnreachable)
		test.Eq(t, "", state.Stamp)
	})

	T.Run("a recorded hash is checked when the stamp is the tip", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		stamp := repo.Commit("one", map[string]string{"a.go": "package a\n\nfunc A() {}\n"})

		w := gitWalker(t, repo.Dir)
		good := anchor.Hash([]string{"package a", "", "func A() {}"}, anchor.Range{StartLine: 3, EndLine: 3})
		tr := repoTour(&tour.Repo{Ref: "main", Stamp: stamp},
			tour.Step{File: "a.go", Line: 3, ContentHash: good},
			tour.Step{File: "a.go", Line: 3, ContentHash: anchor.HashPrefix + "00"},
		)

		step, err := w.Step(t.Context(), tr, 0)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusOK, step.Anchor.Location.Status)

		step, err = w.Step(t.Context(), tr, 1)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusChanged, step.Anchor.Location.Status)
	})

	T.Run("an unknown repo is an error for Repo", func(t *testing.T) {
		t.Parallel()

		_, err := gitWalker(t, t.TempDir()).Repo(t.Context(), repoTour(&tour.Repo{}), "nope")
		test.ErrorIs(t, err, ErrNoSuchRepo)
	})
}

func TestWalker_Home(T *testing.T) {
	T.Parallel()

	T.Run("names the repo the tour file lives in, by its origin", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		repo.Git("remote", "add", "origin", "https://github.com/acme/api.git")

		w := gitWalker(t, repo.Dir)
		tr := &tour.Tour{Root: repo.Dir, Repos: []tour.Repo{
			{Name: "worker", Remote: "git@github.com:acme/worker.git"},
			{Name: "api", Remote: "git@github.com:acme/api.git"},
		}}

		test.Eq(t, "api", w.Home(t.Context(), tr))

		tr.Repos = tr.Repos[:1]
		test.Eq(t, "", w.Home(t.Context(), tr))

		tr.Root = t.TempDir()
		test.Eq(t, "", w.Home(t.Context(), tr))
	})
}
