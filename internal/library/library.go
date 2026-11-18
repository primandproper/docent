// Package library keeps the reader's list of tour folders: the ones they
// added in the app, so it opens with them next time. The list lives in a file
// of its own beside the user config, because the app writes this one and
// never writes the config.
package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const (
	o11yName = "library"

	// FileName is the library file's name, in the user config's folder.
	FileName = "library.json"
)

// ErrNotAFolder is returned for a path that can't be kept: one that is
// relative, or isn't a directory.
var ErrNotAFolder = errors.New("not a folder")

type (
	// Store is the saved list of folders.
	Store interface {
		// Folders lists the saved folders, in the order they were added.
		Folders(ctx context.Context) ([]string, error)
		// Add saves a folder. Adding one that is already there changes nothing.
		Add(ctx context.Context, folder string) error
		// Remove forgets a folder. Removing one that isn't there changes nothing.
		Remove(ctx context.Context, folder string) error
	}

	// file is the library file's JSON shape.
	file struct {
		_       struct{} `json:"-"`
		Folders []string `json:"folders"`
	}

	fileStore struct {
		_      struct{}
		logger logging.Logger
		tracer tracing.Tracer
		path   string
		mu     sync.Mutex
	}
)

var _ Store = (*fileStore)(nil)

// Path is ~/.config/docent/library.json.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}

	return filepath.Join(home, ".config", "docent", FileName), nil
}

// NewStore returns the library kept in the file at path, which doesn't have
// to exist yet.
func NewStore(path string, logger logging.Logger, tracerProvider tracing.Provider) Store {
	return &fileStore{
		path:   path,
		logger: logging.NewNamedLogger(logger, o11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, o11yName),
	}
}

func (s *fileStore) Folders(ctx context.Context) ([]string, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.read()
}

func (s *fileStore) Add(ctx context.Context, folder string) error {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	if !filepath.IsAbs(folder) {
		return fmt.Errorf("%q: %w", folder, ErrNotAFolder)
	}

	folder = filepath.Clean(folder)

	if info, err := os.Stat(folder); err != nil || !info.IsDir() {
		return fmt.Errorf("%q: %w", folder, ErrNotAFolder)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	folders, err := s.read()
	if err != nil {
		return err
	}

	if slices.Contains(folders, folder) {
		return nil
	}

	if err = s.write(append(folders, folder)); err != nil {
		return err
	}

	s.logger.WithValue("folder", folder).Info("added a folder to the library")

	return nil
}

func (s *fileStore) Remove(ctx context.Context, folder string) error {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	folder = filepath.Clean(folder)

	s.mu.Lock()
	defer s.mu.Unlock()

	folders, err := s.read()
	if err != nil {
		return err
	}

	if !slices.Contains(folders, folder) {
		return nil
	}

	if err = s.write(slices.DeleteFunc(folders, func(f string) bool { return f == folder })); err != nil {
		return err
	}

	s.logger.WithValue("folder", folder).Info("removed a folder from the library")

	return nil
}

// read loads the list. A missing file is an empty library. A file that can't
// be understood is an error, so that it is never overwritten with less than
// it held.
func (s *fileStore) read() ([]string, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("reading the library: %w", err)
	}

	var decoded file
	if err = json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("reading the library at %s: %w", s.path, err)
	}

	// The file can be edited by hand: keep only what could be a folder, once.
	var folders []string

	for _, folder := range decoded.Folders {
		if !filepath.IsAbs(folder) {
			continue
		}

		if folder = filepath.Clean(folder); !slices.Contains(folders, folder) {
			folders = append(folders, folder)
		}
	}

	return folders, nil
}

// write replaces the file in one step, so a crash can't leave half a list.
func (s *fileStore) write(folders []string) error {
	if folders == nil {
		folders = []string{}
	}

	encoded, err := json.MarshalIndent(file{Folders: folders}, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding the library: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, FileName+".*")
	if err != nil {
		return fmt.Errorf("saving the library: %w", err)
	}

	defer func() { _ = os.Remove(temp.Name()) }() //nolint:errcheck // gone already once renamed

	if _, err = temp.Write(append(encoded, '\n')); err != nil {
		_ = temp.Close() //nolint:errcheck // the write error is the one to report

		return fmt.Errorf("saving the library: %w", err)
	}

	if err = temp.Close(); err != nil {
		return fmt.Errorf("saving the library: %w", err)
	}

	if err = os.Rename(temp.Name(), s.path); err != nil {
		return fmt.Errorf("saving the library: %w", err)
	}

	return nil
}
