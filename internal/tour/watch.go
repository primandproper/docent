package tour

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"time"

	"github.com/primandproper/primitives-go/v2/observability/logging"

	"github.com/fsnotify/fsnotify"
)

// watchDebounce groups the burst of events one save produces into one reload.
const watchDebounce = 75 * time.Millisecond

// tourDirParents are the names directly under a workspace root that can lead
// to tours. Changes to anything else at that level are none of our business.
var tourDirParents = []string{".tours", ".vscode", ".github", Extension}

// Watch calls onChange, debounced, whenever a tour or anything beside one
// (a body file, a map) changes under root or the extra paths, until ctx is
// done. It returns once the watch is established; watching carries on in the
// background.
func Watch(ctx context.Context, logger logging.Logger, onChange func(), root string, extraPaths ...string) error {
	logger = logging.NewNamedLogger(logger, "tour_watcher")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("starting file watcher: %w", err)
	}

	bases := map[string]bool{}

	for i, base := range append([]string{root}, extraPaths...) {
		abs, absErr := filepath.Abs(base)
		if absErr != nil {
			continue
		}

		// The base itself is watched so a tours folder created later is noticed.
		if addErr := watcher.Add(abs); addErr != nil {
			logger.WithValue("dir", abs).WithError(addErr).Debug("not watching folder")
			continue
		}

		bases[abs] = i > 0

		for _, td := range tourDirs {
			addTree(watcher, filepath.Join(abs, td))
		}
	}

	go func() {
		defer func() {
			if closeErr := watcher.Close(); closeErr != nil {
				logger.Error("closing file watcher", closeErr)
			}
		}()

		var (
			timer   *time.Timer
			pending <-chan time.Time
		)

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if !relevant(event, bases) {
					continue
				}

				if event.Has(fsnotify.Create) {
					addTree(watcher, event.Name)
				}

				if timer == nil {
					timer = time.NewTimer(watchDebounce)
				} else {
					timer.Reset(watchDebounce)
				}

				pending = timer.C
			case <-pending:
				pending = nil

				onChange()
			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return
				}

				logger.Error("watching tours", watchErr)
			}
		}
	}()

	return nil
}

// relevant filters out changes directly inside a workspace root that can't
// affect tours. bases maps each watched base to whether it is an extra path,
// which may hold tours directly.
func relevant(event fsnotify.Event, bases map[string]bool) bool {
	if event.Op == fsnotify.Chmod {
		return false
	}

	holdsTours, isBase := bases[filepath.Dir(event.Name)]
	if !isBase {
		return true
	}

	name := filepath.Base(event.Name)

	return slices.Contains(tourDirParents, name) || (holdsTours && filepath.Ext(name) == Extension)
}

// addTree watches dir and every directory under it. fsnotify isn't recursive,
// and a missing dir is fine: it is picked up when it appears.
func addTree(watcher *fsnotify.Watcher, dir string) {
	//nolint:errcheck // best effort: an unwatchable folder just doesn't live-reload
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}

			return nil
		}

		if d.IsDir() {
			_ = watcher.Add(path)
		}

		return nil
	})
}
