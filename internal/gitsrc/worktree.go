package gitsrc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const worktreeO11yName = "worktree_file_source"

type workingTree struct {
	_      struct{}
	logger logging.Logger
	tracer tracing.Tracer
}

var _ FileSource = (*workingTree)(nil)

// NewWorkingTree returns a FileSource that reads files as they are on disk,
// uncommitted changes included. It ignores ref.
func NewWorkingTree(logger logging.Logger, tracerProvider tracing.Provider) FileSource {
	return &workingTree{
		logger: logging.NewNamedLogger(logger, worktreeO11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, worktreeO11yName),
	}
}

func (w *workingTree) ReadFile(ctx context.Context, root, _, path string) ([]byte, error) {
	_, span := w.tracer.StartSpan(ctx)
	defer span.End()

	abs, info, err := Confine(root, path)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		return nil, fmt.Errorf("%q: %w", path, ErrIsDirectory)
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, w.logger.WithValue("path", abs), span, "reading file")
	}

	return data, nil
}

func (w *workingTree) ReadDir(ctx context.Context, root, _, path string) ([]Entry, error) {
	_, span := w.tracer.StartSpan(ctx)
	defer span.End()

	abs, info, err := Confine(root, path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("%q: %w", path, ErrNotDirectory)
	}

	dirEntries, err := os.ReadDir(abs)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, w.logger.WithValue("path", abs), span, "reading directory")
	}

	entries := make([]Entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		if de.Name() == ".git" {
			continue
		}

		entries = append(entries, Entry{Name: de.Name(), IsDir: de.IsDir()})
	}

	SortEntries(entries)

	return entries, nil
}

// SortEntries orders directories first, then by name.
func SortEntries(entries []Entry) {
	slices.SortFunc(entries, func(a, b Entry) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}

			return 1
		}

		return strings.Compare(a.Name, b.Name)
	})
}

// Confine resolves a root-relative path to an absolute one and proves it stays
// inside root, both as written and after following symlinks. It returns
// ErrEscapesRoot or ErrNotFound without touching anything outside root.
func Confine(root, rel string) (string, fs.FileInfo, error) {
	clean, err := CleanRelative(rel)
	if err != nil {
		return "", nil, err
	}

	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil, fmt.Errorf("root %q: %w", root, ErrNotFound)
		}

		return "", nil, fmt.Errorf("resolving root %q: %w", root, err)
	}

	resolved, err := filepath.EvalSymlinks(filepath.Join(realRoot, clean))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil, fmt.Errorf("%q: %w", rel, ErrNotFound)
		}

		return "", nil, fmt.Errorf("resolving %q: %w", rel, err)
	}

	if resolved != realRoot && !strings.HasPrefix(resolved, realRoot+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("%q: %w", rel, ErrEscapesRoot)
	}

	info, err := os.Stat(resolved)
	if err != nil {
		return "", nil, fmt.Errorf("inspecting %q: %w", rel, err)
	}

	return resolved, info, nil
}

// CleanRelative normalizes a path written in a tour and rejects one that is
// absolute or climbs out of its root. The result uses the OS separator.
func CleanRelative(rel string) (string, error) {
	// Tours recorded on Windows write backslashes.
	slashed := strings.ReplaceAll(rel, `\`, "/")
	if slashed == "" {
		slashed = "."
	}

	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(slashed) {
		return "", fmt.Errorf("%q is absolute: %w", rel, ErrEscapesRoot)
	}

	clean := filepath.Clean(filepath.FromSlash(slashed))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q: %w", rel, ErrEscapesRoot)
	}

	return clean, nil
}
