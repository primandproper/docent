package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/library"

	"github.com/primandproper/primitives-go/v2/observability"
)

var (
	// ErrUnknownFolder is returned for a folder that isn't open.
	ErrUnknownFolder = errors.New("not a folder that is open")
	// ErrNoLibrary is returned when there is nowhere to save the list of folders.
	ErrNoLibrary = errors.New("folders can't be saved here")
)

// folder is one open folder of tours. Each has services of its own, so its
// docent.json, its variables and the repos beside it are its own too.
type folder struct {
	_ struct{}
	// core is nil for a folder whose services couldn't be assembled; problem
	// then says why.
	core *core.Core
	// launcher opens this folder's files in an editor, by its own rules. Nil
	// means the service's.
	launcher editor.Launcher
	root     string
	problem  string
	// known are the tours last discovered here. A tour two folders can both
	// see belongs to the one listed first.
	known []string
	saved bool
}

// setLibrary gives the service the saved list of folders.
func (s *Service) setLibrary(store library.Store) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.library = store
}

// svc returns the services of the folder a tour belongs to. A tour no open
// folder has is answered by the services that belong to none.
func (s *Service) svc(path string) *core.Core {
	s.mu.Lock()
	defer s.mu.Unlock()

	if f := s.holder(path); f != nil {
		return f.core
	}

	return s.bare
}

// launcherFor returns what opens a tour's files in an editor.
func (s *Service) launcherFor(path string) editor.Launcher {
	s.mu.Lock()
	defer s.mu.Unlock()

	if f := s.holder(path); f != nil && f.launcher != nil {
		return f.launcher
	}

	return s.launcher
}

// holder finds the folder a tour was discovered in. The caller holds s.mu.
func (s *Service) holder(path string) *folder {
	for _, f := range s.folders {
		if f.core != nil && slices.Contains(f.known, path) {
			return f
		}
	}

	return nil
}

// known confirms path is a tour some open folder discovered, looking again
// once in case it was created since the picker last listed.
func (s *Service) known(ctx context.Context, path string) error {
	s.mu.Lock()
	f := s.holder(path)
	s.mu.Unlock()

	if f != nil {
		return nil
	}

	if _, err := s.rediscover(ctx, false); err != nil {
		return fmt.Errorf("discovering tours: %w", err)
	}

	s.mu.Lock()
	f = s.holder(path)
	s.mu.Unlock()

	if f == nil {
		return fmt.Errorf("%q: %w", path, ErrUnknownTour)
	}

	return nil
}

// rediscover lists every open folder's tours again and returns the folders as
// they then stand. With reset, what each folder's services remember about the
// code and about each other's tours is dropped first.
func (s *Service) rediscover(ctx context.Context, reset bool) ([]folder, error) {
	s.mu.Lock()
	folders := slices.Clone(s.folders)
	s.mu.Unlock()

	seen := map[string]struct{}{}
	found := make([][]string, len(folders))

	for i, f := range folders {
		if f.core == nil {
			continue
		}

		paths, err := f.core.Store.Discover(ctx, f.root, f.core.Config.TourPaths()...)
		if err != nil {
			return nil, fmt.Errorf("looking in %s: %w", f.root, err)
		}

		for _, path := range paths {
			if _, dup := seen[path]; !dup {
				seen[path] = struct{}{}
				found[i] = append(found[i], path)
			}
		}
	}

	s.mu.Lock()

	out := make([]folder, 0, len(folders))

	for i, f := range folders {
		f.known = found[i]
		out = append(out, folder{core: f.core, root: f.root, known: found[i]})
	}

	s.mu.Unlock()

	// Only once every folder's tours are known can a validator be asked
	// about links between them.
	if reset {
		for _, f := range folders {
			if f.core != nil {
				f.core.Walker.Reset()
				f.core.Validator.Reset()
			}
		}
	}

	return slices.DeleteFunc(out, func(f folder) bool { return f.core == nil }), nil
}

// elsewhere lists the tours of every open folder but one, for that folder's
// validator to check links against.
func (s *Service) elsewhere(root string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) {
		s.mu.Lock()
		defer s.mu.Unlock()

		var out []string

		for _, f := range s.folders {
			if f.root != root {
				out = append(out, f.known...)
			}
		}

		return out, nil
	}
}

// Folders lists the folders that are open, in the order the picker shows them.
func (s *Service) Folders() []Folder {
	s.mu.Lock()
	folders := slices.Clone(s.folders)
	s.mu.Unlock()

	out := make([]Folder, 0, len(folders))

	for _, f := range folders {
		view := Folder{Root: f.root, Name: filepath.Base(f.root), Saved: f.saved, Problem: f.problem}

		if info, err := os.Stat(f.root); err != nil || !info.IsDir() {
			view.Problem = "This folder isn't there any more."
		}

		out = append(out, view)
	}

	return out
}

// AddFolder asks the reader for a folder, opens it and saves it in the
// library. It returns the folders as they now are, which is unchanged if they
// cancelled. It takes no path: the only folders the frontend can add are ones
// the reader picked in the system's own dialog.
func (s *Service) AddFolder(ctx context.Context) ([]Folder, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	s.mu.Lock()
	host := s.host
	s.mu.Unlock()

	if host == nil || host.ChooseFolder == nil {
		return s.Folders(), observability.PrepareAndLogError(ErrNoHost, s.logger, span, "choosing a folder")
	}

	chosen, err := host.ChooseFolder()
	if err != nil {
		return s.Folders(), observability.PrepareAndLogError(err, s.logger, span, "choosing a folder")
	}

	if chosen == "" {
		return s.Folders(), nil
	}

	if err = s.openFolder(ctx, chosen, true); err != nil {
		return s.Folders(), observability.PrepareAndLogError(err, s.logger, span, "opening the chosen folder")
	}

	return s.Folders(), nil
}

// KeepFolder saves a folder that is already open, one that came from the
// command line or a link, in the library. Only an open folder can be kept, so
// this adds nothing the reader isn't already looking at.
func (s *Service) KeepFolder(ctx context.Context, root string) ([]Folder, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue("folder", root)

	if !s.isOpen(root) {
		return s.Folders(), observability.PrepareAndLogError(fmt.Errorf("%q: %w", root, ErrUnknownFolder), logger, span, "keeping a folder")
	}

	if err := s.openFolder(ctx, root, true); err != nil {
		return s.Folders(), observability.PrepareAndLogError(err, logger, span, "keeping a folder")
	}

	return s.Folders(), nil
}

// RemoveFolder closes a folder and, if it was saved, takes it out of the
// library. Nothing on disk but the library file is touched.
func (s *Service) RemoveFolder(ctx context.Context, root string) ([]Folder, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue("folder", root)

	s.mu.Lock()
	index := slices.IndexFunc(s.folders, func(f *folder) bool { return f.root == root })
	store := s.library

	var removed *folder
	if index >= 0 {
		removed = s.folders[index]
	}
	s.mu.Unlock()

	if removed == nil {
		return s.Folders(), observability.PrepareAndLogError(fmt.Errorf("%q: %w", root, ErrUnknownFolder), logger, span, "removing a folder")
	}

	// The library goes first: a folder that couldn't be forgotten stays open.
	if removed.saved && store != nil {
		if err := store.Remove(ctx, root); err != nil {
			return s.Folders(), observability.PrepareAndLogError(err, logger, span, "removing a folder")
		}
	}

	s.mu.Lock()
	s.folders = slices.DeleteFunc(s.folders, func(f *folder) bool { return f == removed })

	for _, path := range removed.known {
		delete(s.open, path)
		delete(s.progress, path)
	}

	bare := s.bare
	s.mu.Unlock()

	if removed.core != nil && removed.core != bare {
		removed.core.Close()
	}

	s.opened()
	logger.Info("removed folder")

	return s.Folders(), nil
}

func (s *Service) isOpen(root string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return slices.ContainsFunc(s.folders, func(f *folder) bool { return f.root == root })
}

// openFolder opens a folder beside the ones already open, and with save keeps
// it in the library. Opening one that is already open only saves it.
func (s *Service) openFolder(ctx context.Context, root string, save bool) error {
	root = filepath.Clean(root)

	s.mu.Lock()
	host, store := s.host, s.library
	s.mu.Unlock()

	if save {
		if store == nil {
			return ErrNoLibrary
		}

		if err := store.Add(ctx, root); err != nil {
			return fmt.Errorf("saving %s: %w", root, err)
		}
	}

	if !s.isOpen(root) {
		if host == nil || host.Rebuild == nil {
			return ErrNoHost
		}

		services, err := host.Rebuild(root)
		if err != nil {
			return fmt.Errorf("opening %s: %w", root, err)
		}

		added := &folder{root: root, core: services}
		if host.Launcher != nil {
			added.launcher = host.Launcher(services)
		}

		s.mu.Lock()
		services.Walker.SetWorkingTree(s.workingTree)

		// Someone else may have opened it while its services were being built.
		raced := slices.ContainsFunc(s.folders, func(f *folder) bool { return f.root == root })
		if !raced {
			s.folders = append(s.folders, added)
		}
		s.mu.Unlock()

		if raced {
			services.Close()
		}

		s.logger.WithValue("root", root).Info("opened folder")
	}

	if save {
		s.mu.Lock()
		for _, f := range s.folders {
			if f.root == root {
				f.saved = true
			}
		}
		s.mu.Unlock()
	}

	s.opened()

	return nil
}

// restore opens the folders saved in the library. One that can't be opened
// is still listed, with what went wrong, so the reader can remove it.
func (s *Service) restore(ctx context.Context) error {
	s.mu.Lock()
	host, store := s.host, s.library
	s.mu.Unlock()

	if store == nil {
		return nil
	}

	saved, err := store.Folders(ctx)
	if err != nil {
		return err
	}

	for _, root := range saved {
		if s.isOpen(root) {
			s.mu.Lock()
			for _, f := range s.folders {
				if f.root == root {
					f.saved = true
				}
			}
			s.mu.Unlock()

			continue
		}

		restored := &folder{root: root, saved: true}

		switch {
		case host == nil || host.Rebuild == nil:
			restored.problem = ErrNoHost.Error()
		default:
			services, buildErr := host.Rebuild(root)
			if buildErr != nil {
				s.logger.WithValue("root", root).Error("opening a saved folder", buildErr)
				restored.problem = buildErr.Error()

				break
			}

			restored.core = services
			if host.Launcher != nil {
				restored.launcher = host.Launcher(services)
			}
		}

		s.mu.Lock()
		if restored.core != nil {
			restored.core.Walker.SetWorkingTree(s.workingTree)
		}

		s.folders = append(s.folders, restored)
		s.mu.Unlock()
	}

	s.opened()

	return nil
}

// opened tells the desktop shell which folders are open.
func (s *Service) opened() {
	s.mu.Lock()
	host := s.host
	cores := make([]*core.Core, 0, len(s.folders))

	for _, f := range s.folders {
		if f.core != nil {
			cores = append(cores, f.core)
		}
	}
	s.mu.Unlock()

	if host != nil && host.Opened != nil {
		host.Opened(cores)
	}
}

// close stops every folder's services.
func (s *Service) close() {
	s.mu.Lock()
	folders, bare := slices.Clone(s.folders), s.bare
	s.mu.Unlock()

	for _, f := range folders {
		if f.core != nil && f.core != bare {
			f.core.Close()
		}
	}

	if bare != nil {
		bare.Close()
	}
}
