package tour

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Extension is the file extension of a tour.
const Extension = ".tour"

// tourDirs are the places CodeTour looks for tours, relative to a workspace root.
var tourDirs = []string{".tours", filepath.Join(".vscode", "tours"), filepath.Join(".github", "tours")}

// Discover lists the tour files under a workspace root, in CodeTour's
// locations, plus those under each extra path (a folder of tours, or a
// workspace root in its own right). Paths come back absolute and sorted.
func Discover(root string, extraPaths ...string) ([]string, error) {
	seen := map[string]struct{}{}

	var out []string

	add := func(path string) {
		if _, dup := seen[path]; !dup {
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}

	for i, base := range append([]string{root}, extraPaths...) {
		abs, err := filepath.Abs(base)
		if err != nil {
			return nil, fmt.Errorf("resolving %q: %w", base, err)
		}

		// CodeTour also accepts a single tour named `.tour` at the root.
		if info, statErr := os.Stat(filepath.Join(abs, Extension)); statErr == nil && info.Mode().IsRegular() {
			add(filepath.Join(abs, Extension))
		}

		dirs := make([]string, 0, len(tourDirs)+1)
		for _, d := range tourDirs {
			dirs = append(dirs, filepath.Join(abs, d))
		}

		// An extra path may itself be a folder of tours.
		if i > 0 {
			dirs = append(dirs, abs)
		}

		for _, dir := range dirs {
			found, walkErr := walkTours(dir, i > 0 && dir == abs)
			if walkErr != nil {
				return nil, walkErr
			}

			for _, f := range found {
				add(f)
			}
		}
	}

	slices.Sort(out)

	return out, nil
}

// walkTours finds *.tour files under dir. A missing dir is not an error. When
// shallow is set only dir itself is read, so pointing an extra path at a big
// checkout doesn't crawl all of it.
func walkTours(dir string, shallow bool) ([]string, error) {
	var out []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == dir {
				return fs.SkipAll
			}

			return err
		}

		if d.IsDir() {
			if shallow && path != dir {
				return fs.SkipDir
			}

			return nil
		}

		if strings.HasSuffix(d.Name(), Extension) {
			out = append(out, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("looking for tours in %q: %w", dir, err)
	}

	return out, nil
}

// RootFor returns the directory a tour's CodeTour-style `file` paths are
// relative to: the folder containing `.tours/` (or `.vscode/`, `.github/`),
// which is CodeTour's workspace root. A tour outside those locations is rooted
// at its own directory.
func RootFor(tourPath string) string {
	dir := filepath.Dir(tourPath)

	for cur := dir; ; cur = filepath.Dir(cur) {
		for _, td := range tourDirs {
			if strings.HasSuffix(cur, string(filepath.Separator)+td) {
				return strings.TrimSuffix(cur, string(filepath.Separator)+td)
			}
		}

		if cur == filepath.Dir(cur) {
			return dir
		}
	}
}
