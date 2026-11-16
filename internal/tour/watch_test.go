package tour

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

func awaitChange(t *testing.T, changes <-chan struct{}) {
	t.Helper()

	select {
	case <-changes:
	case <-time.After(5 * time.Second):
		t.Fatal("no change was reported")
	}
}

func TestWatch(T *testing.T) {
	T.Parallel()

	T.Run("reports edits to tours and the files beside them", func(t *testing.T) {
		t.Parallel()

		root := realTempDir(t)
		tourPath := write(t, filepath.Join(root, ".tours", "a.tour"), minimalTour)
		body := write(t, filepath.Join(root, ".tours", "a", "01.md"), "one")

		changes := make(chan struct{}, 16)
		must.NoError(t, Watch(t.Context(), nil, func() { changes <- struct{}{} }, root))

		must.NoError(t, os.WriteFile(tourPath, []byte(minimalTour+" "), 0o600))
		awaitChange(t, changes)

		must.NoError(t, os.WriteFile(body, []byte("two"), 0o600))
		awaitChange(t, changes)
	})

	T.Run("notices a tours folder created after the watch began", func(t *testing.T) {
		t.Parallel()

		root := realTempDir(t)

		changes := make(chan struct{}, 16)
		must.NoError(t, Watch(t.Context(), nil, func() { changes <- struct{}{} }, root))

		write(t, filepath.Join(root, ".tours", "new.tour"), minimalTour)
		awaitChange(t, changes)

		// The new folder is now watched in its own right.
		for len(changes) > 0 {
			<-changes
		}

		time.Sleep(2 * watchDebounce)
		write(t, filepath.Join(root, ".tours", "second.tour"), minimalTour)
		awaitChange(t, changes)
	})

	T.Run("ignores unrelated files in the workspace root", func(t *testing.T) {
		t.Parallel()

		root := realTempDir(t)
		write(t, filepath.Join(root, ".tours", "a.tour"), minimalTour)

		changes := make(chan struct{}, 16)
		must.NoError(t, Watch(t.Context(), nil, func() { changes <- struct{}{} }, root))

		write(t, filepath.Join(root, "main.go"), "package main")

		select {
		case <-changes:
			t.Fatal("a change outside the tours was reported")
		case <-time.After(4 * watchDebounce):
		}
	})
}
