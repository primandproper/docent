package tour

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/diag"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const minimalTour = `{"title": "t", "steps": [{"description": "d"}]}`

func write(t *testing.T, path, content string) string {
	t.Helper()

	must.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	must.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// realTempDir is t.TempDir with symlinks resolved: on macOS /var is a link to
// /private/var, and discovery reports the paths it was asked about.
func realTempDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	return dir
}

func TestDiscover(T *testing.T) {
	T.Parallel()

	T.Run("finds tours in every CodeTour location", func(t *testing.T) {
		t.Parallel()

		root := realTempDir(t)
		a := write(t, filepath.Join(root, ".tours", "a.tour"), minimalTour)
		nested := write(t, filepath.Join(root, ".tours", "sub", "n.tour"), minimalTour)
		b := write(t, filepath.Join(root, ".vscode", "tours", "b.tour"), minimalTour)
		c := write(t, filepath.Join(root, ".github", "tours", "c.tour"), minimalTour)
		single := write(t, filepath.Join(root, ".tour"), minimalTour)
		write(t, filepath.Join(root, ".tours", "notes.md"), "not a tour")
		write(t, filepath.Join(root, "src", "stray.tour"), minimalTour)

		found, err := Discover(root)
		must.NoError(t, err)
		test.Eq(t, []string{c, single, a, nested, b}, found)
	})

	T.Run("an empty workspace has no tours", func(t *testing.T) {
		t.Parallel()

		found, err := Discover(t.TempDir())
		must.NoError(t, err)
		test.Len(t, 0, found)
	})

	T.Run("extra paths may be a workspace or a bare folder of tours", func(t *testing.T) {
		t.Parallel()

		root := realTempDir(t)
		workspace := realTempDir(t)
		folder := realTempDir(t)

		inRoot := write(t, filepath.Join(root, ".tours", "a.tour"), minimalTour)
		inWorkspace := write(t, filepath.Join(workspace, ".tours", "w.tour"), minimalTour)
		inFolder := write(t, filepath.Join(folder, "f.tour"), minimalTour)
		write(t, filepath.Join(folder, "deep", "ignored.tour"), minimalTour)

		found, err := Discover(root, workspace, folder, workspace)
		must.NoError(t, err)
		test.SliceContainsAll(t, found, []string{inRoot, inWorkspace, inFolder})
		test.Len(t, 3, found)
	})
}

func TestRootFor(T *testing.T) {
	T.Parallel()

	T.Run("is the folder holding the tours directory", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, "/w/repo", RootFor("/w/repo/.tours/a.tour"))
		test.Eq(t, "/w/repo", RootFor("/w/repo/.tours/sub/dir/a.tour"))
		test.Eq(t, "/w/repo", RootFor("/w/repo/.vscode/tours/a.tour"))
		test.Eq(t, "/w/repo", RootFor("/w/repo/.github/tours/a.tour"))
		test.Eq(t, "/w/repo", RootFor("/w/repo/.tour"))
		test.Eq(t, "/w/loose", RootFor("/w/loose/a.tour"))
	})
}

func TestFileStore(T *testing.T) {
	T.Parallel()

	T.Run("loads a tour and roots it", func(t *testing.T) {
		t.Parallel()

		root := realTempDir(t)
		path := write(t, filepath.Join(root, ".tours", "a.tour"), minimalTour)

		store := NewStore(nil, nil)

		found, err := store.Discover(t.Context(), root)
		must.NoError(t, err)
		must.Eq(t, []string{path}, found)

		loaded, diags, err := store.Load(t.Context(), path)
		must.NoError(t, err)
		test.Len(t, 0, diags)
		test.Eq(t, root, loaded.Root)
		test.Eq(t, path, loaded.Path)
	})

	T.Run("an unreadable tour is a diagnostic", func(t *testing.T) {
		t.Parallel()

		loaded, diags, err := NewStore(nil, nil).Load(t.Context(), filepath.Join(t.TempDir(), "nope.tour"))
		must.NoError(t, err)
		test.Nil(t, loaded)
		test.Eq(t, []diag.Code{diag.CodeReadFailed}, codes(diags))
	})
}

func TestMemoryStore(T *testing.T) {
	T.Parallel()

	T.Run("serves tours from memory", func(t *testing.T) {
		t.Parallel()

		store := &MemoryStore{Root: "/r", Files: map[string][]byte{"/r/.tours/a.tour": []byte(minimalTour)}}

		found, err := store.Discover(t.Context(), "/r")
		must.NoError(t, err)
		test.Eq(t, []string{"/r/.tours/a.tour"}, found)

		loaded, diags, err := store.Load(t.Context(), "/r/.tours/a.tour")
		must.NoError(t, err)
		test.Len(t, 0, diags)
		test.Eq(t, "/r", loaded.Root)

		_, diags, err = store.Load(t.Context(), "/r/.tours/b.tour")
		must.NoError(t, err)
		test.Eq(t, []diag.Code{diag.CodeReadFailed}, codes(diags))
	})
}
