package gitsrc_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/gitsrc/gittest"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func newGit(t *testing.T) gitsrc.Git {
	t.Helper()

	g := gitsrc.NewGit(nil, nil)
	t.Cleanup(g.Close)

	return g
}

func TestGit_ReadFile(T *testing.T) {
	T.Parallel()

	T.Run("reads at a commit without touching the checkout", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		first := repo.Commit("one", map[string]string{"pkg/a.go": "v1\n", "b with space.txt": "b\n"})
		second := repo.Commit("two", map[string]string{"pkg/a.go": "v2\n"})
		repo.Write(map[string]string{"pkg/a.go": "uncommitted\n"})

		g := newGit(t)

		for ref, want := range map[string]string{first: "v1\n", second: "v2\n", "main": "v2\n", "": "uncommitted\n"} {
			data, err := g.ReadFile(t.Context(), repo.Dir, ref, "pkg/a.go")
			must.NoError(t, err)
			test.Eq(t, want, string(data), test.Sprint(ref))
		}

		data, err := g.ReadFile(t.Context(), repo.Dir, first, "b with space.txt")
		must.NoError(t, err)
		test.Eq(t, "b\n", string(data))
	})

	T.Run("reads relative to a subdirectory root", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		sha := repo.Commit("one", map[string]string{"services/items/main.go": "package main\n"})

		data, err := newGit(t).ReadFile(t.Context(), filepath.Join(repo.Dir, "services", "items"), sha, "main.go")
		must.NoError(t, err)
		test.Eq(t, "package main\n", string(data))
	})

	T.Run("missing files, directories and escapes are told apart, and the batch survives them", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		sha := repo.Commit("one", map[string]string{"pkg/a.go": "a\n"})
		g := newGit(t)

		_, err := g.ReadFile(t.Context(), repo.Dir, sha, "pkg/gone.go")
		test.ErrorIs(t, err, gitsrc.ErrNotFound)

		_, err = g.ReadFile(t.Context(), repo.Dir, sha, "a b/gone missing")
		test.ErrorIs(t, err, gitsrc.ErrNotFound)

		_, err = g.ReadFile(t.Context(), repo.Dir, sha, "pkg")
		test.ErrorIs(t, err, gitsrc.ErrIsDirectory)

		_, err = g.ReadFile(t.Context(), repo.Dir, sha, "../outside")
		test.ErrorIs(t, err, gitsrc.ErrEscapesRoot)

		_, err = g.ReadFile(t.Context(), repo.Dir, "0000000000000000000000000000000000000000", "pkg/a.go")
		test.ErrorIs(t, err, gitsrc.ErrNotFound)

		data, err := g.ReadFile(t.Context(), repo.Dir, sha, "pkg/a.go")
		must.NoError(t, err)
		test.Eq(t, "a\n", string(data))
	})

	T.Run("a folder that isn't a repo says so", func(t *testing.T) {
		t.Parallel()

		_, err := newGit(t).ReadFile(t.Context(), t.TempDir(), "main", "a.go")
		test.ErrorIs(t, err, gitsrc.ErrNotARepo)
	})
}

func TestGit_ReadDir(T *testing.T) {
	T.Parallel()

	T.Run("lists a tree at a commit", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		sha := repo.Commit("one", map[string]string{"pkg/a.go": "", "pkg/sub/b.go": "", "top.go": ""})
		must.NoError(t, os.Remove(filepath.Join(repo.Dir, "top.go")))

		g := newGit(t)

		entries, err := g.ReadDir(t.Context(), repo.Dir, sha, "")
		must.NoError(t, err)
		test.Eq(t, []gitsrc.Entry{{Name: "pkg", IsDir: true}, {Name: "top.go"}}, entries)

		entries, err = g.ReadDir(t.Context(), repo.Dir, sha, "pkg")
		must.NoError(t, err)
		test.Eq(t, []gitsrc.Entry{{Name: "sub", IsDir: true}, {Name: "a.go"}}, entries)

		_, err = g.ReadDir(t.Context(), repo.Dir, sha, "nope")
		test.ErrorIs(t, err, gitsrc.ErrNotFound)

		_, err = g.ReadDir(t.Context(), repo.Dir, sha, "top.go")
		test.ErrorIs(t, err, gitsrc.ErrNotDirectory)
	})
}

func TestGit_History(T *testing.T) {
	T.Parallel()

	T.Run("resolves, describes and counts commits", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		first := repo.Commit("add a", map[string]string{"a.go": "a\n"})
		second := repo.Commit("change a", map[string]string{"a.go": "b\n"})
		g := newGit(t)

		sha, err := g.Commit(t.Context(), repo.Dir, "main")
		must.NoError(t, err)
		test.Eq(t, second, sha)

		sha, err = g.Commit(t.Context(), repo.Dir, first[:8])
		must.NoError(t, err)
		test.Eq(t, first, sha)

		_, err = g.Commit(t.Context(), repo.Dir, "nope")
		test.ErrorIs(t, err, gitsrc.ErrRevNotFound)

		_, err = g.Commit(t.Context(), repo.Dir, "--help")
		test.ErrorIs(t, err, gitsrc.ErrRevNotFound)

		commit, err := g.Describe(t.Context(), repo.Dir, first)
		must.NoError(t, err)
		test.Eq(t, "add a", commit.Subject)
		test.Eq(t, first, commit.SHA)

		n, err := g.CountBetween(t.Context(), repo.Dir, first, second)
		must.NoError(t, err)
		test.Eq(t, 1, n)

		test.True(t, g.RefExists(t.Context(), repo.Dir, "refs/heads/main"))
		test.False(t, g.RefExists(t.Context(), repo.Dir, "refs/heads/nope"))
		test.Eq(t, "refs/heads/main", g.DefaultBranch(t.Context(), repo.Dir))
		test.Eq(t, "", g.Origin(t.Context(), repo.Dir))

		_, fetched := g.LastFetch(t.Context(), repo.Dir)
		test.False(t, fetched)
	})

	T.Run("follows a rename and maps the diff", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		body := "package a\n\nfunc One() {}\n\nfunc Two() {}\n\nfunc Three() {}\n"
		first := repo.Commit("add", map[string]string{"old/a.go": body})
		repo.Git("mv", "old/a.go", "moved.go")
		second := repo.Commit("move and edit", map[string]string{"moved.go": "// header\n" + body + "\nfunc Four() {}\n"})
		g := newGit(t)

		to, err := g.RenamedTo(t.Context(), repo.Dir, first, second, "old/a.go")
		must.NoError(t, err)
		test.Eq(t, "moved.go", to)

		to, err = g.RenamedTo(t.Context(), repo.Dir, first, second, "other.go")
		must.NoError(t, err)
		test.Eq(t, "", to)

		hunks, err := g.Diff(t.Context(), repo.Dir, first, second, "old/a.go", "moved.go")
		must.NoError(t, err)
		test.Eq(t, []gitsrc.Hunk{
			{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: 1},
			{OldStart: 7, OldCount: 0, NewStart: 9, NewCount: 2},
		}, hunks)

		text, err := g.DiffText(t.Context(), repo.Dir, first, second, "old/a.go", "moved.go")
		must.NoError(t, err)
		test.StrContains(t, text, "+// header")
	})

	T.Run("finds the commit that removed some code, or a whole file", func(t *testing.T) {
		t.Parallel()

		repo := gittest.New(t)
		first := repo.Commit("add", map[string]string{"a.go": "package a\n\nfunc pollWorker() {}\n", "b.go": "b\n"})
		repo.Commit("unrelated", map[string]string{"c.go": "c\n"})
		removal := repo.Commit("replace polling worker with event stream", map[string]string{"a.go": "package a\n\nfunc stream() {}\n"})
		must.NoError(t, os.Remove(filepath.Join(repo.Dir, "b.go")))
		deletion := repo.Commit("drop b", nil)
		g := newGit(t)

		commit, err := g.LastTouch(t.Context(), repo.Dir, first, deletion, "a.go", "func pollWorker() {}")
		must.NoError(t, err)
		must.NotNil(t, commit)
		test.Eq(t, removal, commit.SHA)
		test.Eq(t, "replace polling worker with event stream", commit.Subject)

		commit, err = g.LastTouch(t.Context(), repo.Dir, first, deletion, "b.go", "")
		must.NoError(t, err)
		must.NotNil(t, commit)
		test.Eq(t, deletion, commit.SHA)

		commit, err = g.LastTouch(t.Context(), repo.Dir, first, deletion, "a.go", "never there")
		must.NoError(t, err)
		test.Nil(t, commit)
	})

	T.Run("clones and fetches", func(t *testing.T) {
		t.Parallel()

		upstream := gittest.New(t)
		upstream.Commit("one", map[string]string{"a.go": "v1\n"})
		g := newGit(t)

		dest := filepath.Join(t.TempDir(), "nested", "clone")
		must.NoError(t, g.Clone(t.Context(), upstream.Dir, dest))
		test.Eq(t, "refs/remotes/origin/main", g.DefaultBranch(t.Context(), dest))
		test.Eq(t, upstream.Dir, g.Origin(t.Context(), dest))

		second := upstream.Commit("two", map[string]string{"a.go": "v2\n"})
		must.NoError(t, g.Fetch(t.Context(), dest))

		_, fetched := g.LastFetch(t.Context(), dest)
		test.True(t, fetched)

		sha, err := g.Commit(t.Context(), dest, "refs/remotes/origin/main")
		must.NoError(t, err)
		test.Eq(t, second, sha)

		// The checkout itself was never moved.
		data, err := os.ReadFile(filepath.Join(dest, "a.go"))
		must.NoError(t, err)
		test.Eq(t, "v1\n", string(data))
	})
}
