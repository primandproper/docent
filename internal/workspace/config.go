// Package workspace loads Docent's configuration and maps the repos a tour
// names to checkouts on this machine.
package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/tailscale/hujson"
)

// FileName is the workspace file, looked for in the opened folder and then
// each of its ancestors.
const FileName = "docent.json"

type (
	// File is the shape shared by the workspace file and the user config.
	// Both are JSON with comments and trailing commas allowed.
	File struct {
		_ struct{} `json:"-"`
		// Repos maps a tour's logical repo name to a checkout.
		Repos map[string]string `json:"repos"`
		// Vars are values for link templates.
		Vars    map[string]string `json:"vars"`
		Editors Editors           `json:"editors"`
		// RepoRoots are folders that hold checkouts side by side.
		RepoRoots []string `json:"repoRoots"`
		// TourPaths are extra places to look for tours.
		TourPaths []string `json:"tourPaths"`
	}

	// Editors is the open-in-editor configuration.
	Editors struct {
		_ struct{} `json:"-"`
		// Custom defines editors by argv template: {path}, {line} and {col}
		// are substituted, and the result is run without a shell.
		Custom map[string]CustomEditor `json:"custom"`
		// Default is the editor `o` uses when no rule matches.
		Default string `json:"default"`
		// Rules pick an editor per file. The first match wins.
		Rules []EditorRule `json:"rules"`
	}

	// EditorRule maps files to an editor. A rule with both Match and Repo
	// needs both to hold; a rule with neither matches nothing.
	EditorRule struct {
		_ struct{} `json:"-"`
		// Match is a glob over the file's path inside its repo, with `**`
		// and `{a,b}` alternatives: "**/*.{java,kt}".
		Match string `json:"match"`
		// Repo is a tour's logical repo name.
		Repo string `json:"repo"`
		// Use is the id of the editor to open matching files in.
		Use string `json:"use"`
	}

	// CustomEditor is an editor launched by an argv template.
	CustomEditor struct {
		_    struct{} `json:"-"`
		Argv []string `json:"argv"`
	}

	// Config is the effective configuration for one opened folder: the
	// workspace file and the user config, each with its paths made absolute.
	Config struct {
		_ struct{}
		// Root is the opened folder.
		Root string
		// WorkspacePath and UserPath say where each file was found, or are
		// empty when it doesn't exist.
		WorkspacePath string
		UserPath      string
		Workspace     File
		User          File
	}
)

// UserConfigPath is ~/.config/docent/config.json.
func UserConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding the home directory: %w", err)
	}

	return filepath.Join(home, ".config", "docent", "config.json"), nil
}

// Load reads the configuration for an opened folder. userPath is the user
// config file; neither file has to exist.
func Load(root, userPath string) (*Config, error) {
	cfg := &Config{Root: root}

	for dir := root; ; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, FileName)

		found, err := readFile(candidate, &cfg.Workspace)
		if err != nil {
			return nil, err
		}

		if found {
			cfg.WorkspacePath = candidate
			break
		}

		if dir == filepath.Dir(dir) {
			break
		}
	}

	if userPath != "" {
		found, err := readFile(userPath, &cfg.User)
		if err != nil {
			return nil, err
		}

		if found {
			cfg.UserPath = userPath
		}
	}

	return cfg, nil
}

// readFile decodes a config file, resolving the paths in it against the
// file's own directory. A missing file is not an error.
func readFile(path string, into *File) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}

		return false, fmt.Errorf("reading %s: %w", path, err)
	}

	standard, err := hujson.Standardize(data)
	if err != nil {
		return false, fmt.Errorf("parsing %s: %w", path, err)
	}

	if err = json.Unmarshal(standard, into); err != nil {
		return false, fmt.Errorf("parsing %s: %w", path, err)
	}

	base := filepath.Dir(path)

	for name, p := range into.Repos {
		into.Repos[name] = absolute(base, p)
	}

	for i, p := range into.RepoRoots {
		into.RepoRoots[i] = absolute(base, p)
	}

	for i, p := range into.TourPaths {
		into.TourPaths[i] = absolute(base, p)
	}

	return true, nil
}

// absolute expands a leading ~ and resolves a relative path against base.
func absolute(base, path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}

	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}

	return filepath.Clean(path)
}

// RepoRoots lists the folders to look for checkouts in, in order: the opened
// folder, the workspace file's roots, the user's, and last the opened folder's
// parent. The parent is what makes the plain case need no config: clone the
// tours repo beside the repos it describes, open it, and they are found.
func (c *Config) RepoRoots() []string {
	roots := []string{c.Root}
	roots = append(roots, c.Workspace.RepoRoots...)
	roots = append(roots, c.User.RepoRoots...)

	if parent := filepath.Dir(c.Root); parent != c.Root {
		roots = append(roots, parent)
	}

	return roots
}

// CloneRoot is where a missing repo would be cloned: the first configured
// repo root. With none configured it is the opened folder, unless that is
// itself a checkout, in which case its siblings live in its parent.
func (c *Config) CloneRoot() string {
	if configured := append(append([]string{}, c.Workspace.RepoRoots...), c.User.RepoRoots...); len(configured) > 0 {
		return configured[0]
	}

	if _, err := os.Stat(filepath.Join(c.Root, ".git")); err == nil {
		return filepath.Dir(c.Root)
	}

	return c.Root
}

// TourPaths lists the extra places to look for tours.
func (c *Config) TourPaths() []string {
	return append(append([]string{}, c.Workspace.TourPaths...), c.User.TourPaths...)
}

// Var looks a link variable up in the workspace file, then the user config.
func (c *Config) Var(name string) (value, source string, ok bool) {
	if v, found := c.Workspace.Vars[name]; found {
		return v, "workspace", true
	}

	if v, found := c.User.Vars[name]; found {
		return v, "user", true
	}

	return "", "", false
}

// DefaultEditor is the configured default editor: the workspace's if it names
// one, else the user's.
func (c *Config) DefaultEditor() string {
	if c.Workspace.Editors.Default != "" {
		return c.Workspace.Editors.Default
	}

	return c.User.Editors.Default
}

// CustomEditors merges both files' custom editors. Only the user config is
// consulted: an argv template in a workspace file would be a command that
// arrives with a cloned repo, and Docent never runs anything a repo brought.
func (c *Config) CustomEditors() map[string]CustomEditor {
	return c.User.Editors.Custom
}

// EditorFor returns the editor the rules choose for a file, or "". Workspace
// rules are consulted before the user's, being the more specific. A rule only
// names an editor; what that editor runs is still the user's to define.
func (c *Config) EditorFor(repo, relPath string) string {
	for _, rules := range [][]EditorRule{c.Workspace.Editors.Rules, c.User.Editors.Rules} {
		for i := range rules {
			if rules[i].matches(repo, relPath) {
				return rules[i].Use
			}
		}
	}

	return ""
}

func (r *EditorRule) matches(repo, relPath string) bool {
	if r.Use == "" || (r.Match == "" && r.Repo == "") {
		return false
	}

	if r.Repo != "" && r.Repo != repo {
		return false
	}

	if r.Match == "" {
		return true
	}

	// A malformed glob matches nothing; it doesn't break opening files.
	ok, err := doublestar.Match(r.Match, filepath.ToSlash(relPath))

	return err == nil && ok
}
