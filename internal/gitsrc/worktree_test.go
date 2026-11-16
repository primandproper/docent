package gitsrc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	must.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	must.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestWorkingTree_ReadFile(T *testing.T) {
	T.Parallel()

	T.Run("reads a file under the root", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "pkg", "a.go"), "package a\n")

		data, err := NewWorkingTree(nil, nil).ReadFile(t.Context(), root, "", "pkg/a.go")
		must.NoError(t, err)
		test.Eq(t, "package a\n", string(data))
	})

	T.Run("accepts backslash separators", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "pkg", "a.go"), "package a\n")

		_, err := NewWorkingTree(nil, nil).ReadFile(t.Context(), root, "", `pkg\a.go`)
		test.NoError(t, err)
	})

	T.Run("reports a missing file", func(t *testing.T) {
		t.Parallel()

		_, err := NewWorkingTree(nil, nil).ReadFile(t.Context(), t.TempDir(), "", "nope.go")
		test.ErrorIs(t, err, ErrNotFound)
	})

	T.Run("refuses a directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "pkg", "a.go"), "package a\n")

		_, err := NewWorkingTree(nil, nil).ReadFile(t.Context(), root, "", "pkg")
		test.ErrorIs(t, err, ErrIsDirectory)
	})

	T.Run("refuses a path that climbs out of the root", func(t *testing.T) {
		t.Parallel()

		parent := t.TempDir()
		root := filepath.Join(parent, "repo")
		writeFile(t, filepath.Join(root, "a.go"), "package a\n")
		writeFile(t, filepath.Join(parent, "secret"), "hunter2\n")

		src := NewWorkingTree(nil, nil)

		_, err := src.ReadFile(t.Context(), root, "", "../secret")
		test.ErrorIs(t, err, ErrEscapesRoot)

		_, err = src.ReadFile(t.Context(), root, "", "sub/../../secret")
		test.ErrorIs(t, err, ErrEscapesRoot)

		_, err = src.ReadFile(t.Context(), root, "", filepath.Join(parent, "secret"))
		test.ErrorIs(t, err, ErrEscapesRoot)
	})

	T.Run("refuses a symlink that leaves the root", func(t *testing.T) {
		t.Parallel()

		parent := t.TempDir()
		root := filepath.Join(parent, "repo")
		writeFile(t, filepath.Join(root, "a.go"), "package a\n")
		writeFile(t, filepath.Join(parent, "secret"), "hunter2\n")
		must.NoError(t, os.Symlink(filepath.Join(parent, "secret"), filepath.Join(root, "link")))

		_, err := NewWorkingTree(nil, nil).ReadFile(t.Context(), root, "", "link")
		test.ErrorIs(t, err, ErrEscapesRoot)
	})

	T.Run("follows a symlink that stays inside the root", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "a.go"), "package a\n")
		must.NoError(t, os.Symlink(filepath.Join(root, "a.go"), filepath.Join(root, "link.go")))

		data, err := NewWorkingTree(nil, nil).ReadFile(t.Context(), root, "", "link.go")
		must.NoError(t, err)
		test.Eq(t, "package a\n", string(data))
	})
}

func TestWorkingTree_ReadDir(T *testing.T) {
	T.Parallel()

	T.Run("lists directories first and hides .git", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "b.go"), "")
		writeFile(t, filepath.Join(root, "a.go"), "")
		writeFile(t, filepath.Join(root, "zdir", "x.go"), "")
		writeFile(t, filepath.Join(root, ".git", "HEAD"), "")

		entries, err := NewWorkingTree(nil, nil).ReadDir(t.Context(), root, "", "")
		must.NoError(t, err)
		test.Eq(t, []Entry{{Name: "zdir", IsDir: true}, {Name: "a.go"}, {Name: "b.go"}}, entries)
	})

	T.Run("refuses a file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "a.go"), "")

		_, err := NewWorkingTree(nil, nil).ReadDir(t.Context(), root, "", "a.go")
		test.ErrorIs(t, err, ErrNotDirectory)
	})
}

func TestMemory(T *testing.T) {
	T.Parallel()

	T.Run("behaves like a file source", func(t *testing.T) {
		t.Parallel()

		src := Memory{
			Key("/r", "", "pkg/a.go"):     []byte("a"),
			Key("/r", "", "pkg/sub/b.go"): []byte("b"),
			Key("/r", "", "top.go"):       []byte("t"),
		}

		data, err := src.ReadFile(t.Context(), "/r", "", "pkg/a.go")
		must.NoError(t, err)
		test.Eq(t, "a", string(data))

		_, err = src.ReadFile(t.Context(), "/r", "", "pkg")
		test.ErrorIs(t, err, ErrIsDirectory)

		_, err = src.ReadFile(t.Context(), "/r", "", "../x")
		test.ErrorIs(t, err, ErrEscapesRoot)

		_, err = src.ReadFile(t.Context(), "/r", "main", "pkg/a.go")
		test.ErrorIs(t, err, ErrNotFound)

		entries, err := src.ReadDir(t.Context(), "/r", "", "pkg")
		must.NoError(t, err)
		test.Eq(t, []Entry{{Name: "sub", IsDir: true}, {Name: "a.go"}}, entries)

		entries, err = src.ReadDir(t.Context(), "/r", "", "")
		must.NoError(t, err)
		test.Eq(t, []Entry{{Name: "pkg", IsDir: true}, {Name: "top.go"}}, entries)
	})
}
