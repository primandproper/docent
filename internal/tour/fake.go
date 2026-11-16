package tour

import (
	"context"
	"fmt"
	"slices"

	"github.com/primandproper/docent/internal/diag"
)

// MemoryStore is an in-memory Store for tests: tour file path to its bytes.
// Every tour is rooted at Root.
type MemoryStore struct {
	_     struct{}
	Files map[string][]byte
	Root  string
}

var _ Store = (*MemoryStore)(nil)

// Discover implements Store, returning every path in sorted order.
func (m *MemoryStore) Discover(context.Context, string, ...string) ([]string, error) {
	paths := make([]string, 0, len(m.Files))
	for p := range m.Files {
		paths = append(paths, p)
	}

	slices.Sort(paths)

	return paths, nil
}

// Load implements Store.
func (m *MemoryStore) Load(_ context.Context, path string) (*Tour, []diag.Diagnostic, error) {
	data, ok := m.Files[path]
	if !ok {
		return nil, []diag.Diagnostic{{
			File:     path,
			Severity: diag.SeverityError,
			Code:     diag.CodeReadFailed,
			Message:  fmt.Sprintf("reading the tour file: %q does not exist", path),
		}}, nil
	}

	t, diags, err := Parse(path, data)
	if err != nil {
		return nil, nil, err
	}

	if t != nil {
		t.Root = m.Root
	}

	return t, diags, nil
}
