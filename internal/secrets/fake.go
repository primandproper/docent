package secrets

import (
	"context"
	"fmt"
	"sync"
)

// Memory is an in-memory Store for tests.
type Memory struct {
	_      struct{}
	Stored map[string]string
	// Err, when set, is what every call fails with: a store that can't be reached.
	Err error
	mu  sync.Mutex
}

var _ Store = (*Memory)(nil)

// Get implements Store.
func (m *Memory) Get(_ context.Context, name string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Err != nil {
		return "", m.Err
	}

	secret, ok := m.Stored[name]
	if !ok {
		return "", fmt.Errorf("%q: %w", name, ErrNotFound)
	}

	return secret, nil
}

// Set implements Store.
func (m *Memory) Set(_ context.Context, name, secret string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Err != nil {
		return m.Err
	}

	if m.Stored == nil {
		m.Stored = map[string]string{}
	}

	m.Stored[name] = secret

	return nil
}

// Delete implements Store.
func (m *Memory) Delete(_ context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.Err != nil {
		return m.Err
	}

	if _, ok := m.Stored[name]; !ok {
		return fmt.Errorf("%q: %w", name, ErrNotFound)
	}

	delete(m.Stored, name)

	return nil
}
