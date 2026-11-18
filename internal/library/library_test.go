package library

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func TestStore(T *testing.T) {
	T.Parallel()

	T.Run("a library that was never saved is empty", func(t *testing.T) {
		t.Parallel()

		folders, err := NewStore(filepath.Join(t.TempDir(), "nope", FileName), nil, nil).Folders(t.Context())
		must.NoError(t, err)
		test.Len(t, 0, folders)
	})

	T.Run("folders are kept in the order they were added, once each", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "config", FileName)
		a, b := t.TempDir(), t.TempDir()

		store := NewStore(path, nil, nil)
		must.NoError(t, store.Add(t.Context(), a))
		must.NoError(t, store.Add(t.Context(), b))
		must.NoError(t, store.Add(t.Context(), a+string(filepath.Separator)))

		// Another store on the same file sees what was saved.
		folders, err := NewStore(path, nil, nil).Folders(t.Context())
		must.NoError(t, err)
		test.Eq(t, []string{a, b}, folders)

		info, err := os.Stat(path)
		must.NoError(t, err)
		test.Eq(t, os.FileMode(0o600), info.Mode().Perm())
	})

	T.Run("a folder can be removed, and removing a stranger changes nothing", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), FileName)
		a, b := t.TempDir(), t.TempDir()

		store := NewStore(path, nil, nil)
		must.NoError(t, store.Add(t.Context(), a))
		must.NoError(t, store.Add(t.Context(), b))
		must.NoError(t, store.Remove(t.Context(), a))
		must.NoError(t, store.Remove(t.Context(), "/never/added"))

		folders, err := store.Folders(t.Context())
		must.NoError(t, err)
		test.Eq(t, []string{b}, folders)
	})

	T.Run("only a folder that exists can be added", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		regular := filepath.Join(dir, "file")
		must.NoError(t, os.WriteFile(regular, nil, 0o600))

		store := NewStore(filepath.Join(dir, FileName), nil, nil)
		for _, bad := range []string{"relative/path", filepath.Join(dir, "missing"), regular} {
			test.ErrorIs(t, store.Add(t.Context(), bad), ErrNotAFolder, test.Sprint(bad))
		}

		_, err := os.Stat(filepath.Join(dir, FileName))
		test.ErrorIs(t, err, os.ErrNotExist)
	})

	T.Run("a file that can't be understood is an error, and is left alone", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), FileName)
		must.NoError(t, os.WriteFile(path, []byte("{nope"), 0o600))

		store := NewStore(path, nil, nil)
		_, err := store.Folders(t.Context())
		test.Error(t, err)
		test.Error(t, store.Add(t.Context(), t.TempDir()))

		kept, err := os.ReadFile(path)
		must.NoError(t, err)
		test.Eq(t, "{nope", string(kept))
	})

	T.Run("a hand-edited file keeps only absolute paths, once", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), FileName)
		must.NoError(t, os.WriteFile(path, []byte(`{"folders": ["/a", "b", "/a/", "/c/../d"]}`), 0o600))

		folders, err := NewStore(path, nil, nil).Folders(t.Context())
		must.NoError(t, err)
		test.Eq(t, []string{"/a", "/d"}, folders)
	})
}
