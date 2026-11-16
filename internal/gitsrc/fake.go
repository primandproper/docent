package gitsrc

import (
	"context"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Memory is an in-memory FileSource for tests. Keys are
// "<root>@<ref>:<slash/separated/path>"; use Key to build them.
type Memory map[string][]byte

var _ FileSource = Memory(nil)

// Key builds the Memory key for a file.
func Key(root, ref, file string) string {
	return root + "@" + ref + ":" + file
}

// ReadFile implements FileSource.
func (m Memory) ReadFile(_ context.Context, root, ref, file string) ([]byte, error) {
	clean, err := CleanRelative(file)
	if err != nil {
		return nil, err
	}

	slashed := filepath.ToSlash(clean)
	if data, ok := m[Key(root, ref, slashed)]; ok {
		return data, nil
	}

	if len(m.children(root, ref, slashed)) > 0 {
		return nil, fmt.Errorf("%q: %w", file, ErrIsDirectory)
	}

	return nil, fmt.Errorf("%q: %w", file, ErrNotFound)
}

// ReadDir implements FileSource.
func (m Memory) ReadDir(_ context.Context, root, ref, dir string) ([]Entry, error) {
	clean, err := CleanRelative(dir)
	if err != nil {
		return nil, err
	}

	slashed := filepath.ToSlash(clean)
	if _, ok := m[Key(root, ref, slashed)]; ok {
		return nil, fmt.Errorf("%q: %w", dir, ErrNotDirectory)
	}

	entries := m.children(root, ref, slashed)
	if len(entries) == 0 {
		return nil, fmt.Errorf("%q: %w", dir, ErrNotFound)
	}

	return entries, nil
}

func (m Memory) children(root, ref, dir string) []Entry {
	prefix := Key(root, ref, "")
	if dir != "." {
		prefix += dir + "/"
	}

	seen := map[string]bool{}

	var entries []Entry

	for key := range m {
		rest, ok := strings.CutPrefix(key, prefix)
		if !ok || rest == "" {
			continue
		}

		name, _, isDir := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}

		seen[name] = true
		entries = append(entries, Entry{Name: path.Base(name), IsDir: isDir})
	}

	SortEntries(entries)

	return entries
}
