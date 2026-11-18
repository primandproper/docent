package library

import (
	"context"
	"path/filepath"
	"slices"
	"sync"
)

// Memory is an in-memory Store for tests. It doesn't check that a folder exists.
type Memory struct {
	_     struct{}
	Saved []string
	mu    sync.Mutex
}

var _ Store = (*Memory)(nil)

// Folders implements Store.
func (m *Memory) Folders(context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return slices.Clone(m.Saved), nil
}

// Add implements Store.
func (m *Memory) Add(_ context.Context, folder string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if folder = filepath.Clean(folder); !slices.Contains(m.Saved, folder) {
		m.Saved = append(m.Saved, folder)
	}

	return nil
}

// Remove implements Store.
func (m *Memory) Remove(_ context.Context, folder string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	folder = filepath.Clean(folder)
	m.Saved = slices.DeleteFunc(m.Saved, func(f string) bool { return f == folder })

	return nil
}
