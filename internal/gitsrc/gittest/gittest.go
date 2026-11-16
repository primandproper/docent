// Package gittest builds real git repositories for tests.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
)

// Repo is a git repository in a temporary directory.
type Repo struct {
	_   struct{}
	t   *testing.T
	Dir string
}

// New creates an empty repository on branch main.
func New(t *testing.T) *Repo {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	return At(t, dir)
}

// At makes dir, which must exist, a repository on branch main.
func At(t *testing.T, dir string) *Repo {
	t.Helper()

	r := &Repo{t: t, Dir: dir}
	r.Git("init", "-q", "-b", "main")

	return r
}

// Attach wraps an existing directory, repository or not, so its files can be
// written with the same helpers.
func Attach(t *testing.T, dir string) *Repo {
	t.Helper()

	return &Repo{t: t, Dir: dir}
}

// Git runs a git command in the repo and returns its trimmed output.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()

	cmd := exec.CommandContext(r.t.Context(), "git", append([]string{"-C", r.Dir}, args...)...)
	// Fixed identity and dates: nothing about the host leaks into a test.
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)

	out, err := cmd.CombinedOutput()
	must.NoError(r.t, err, must.Sprintf("git %v: %s", args, out))

	return strings.TrimSpace(string(out))
}

// Write writes files (slash-separated path to content) without committing.
func (r *Repo) Write(files map[string]string) {
	r.t.Helper()

	for name, content := range files {
		path := filepath.Join(r.Dir, filepath.FromSlash(name))
		must.NoError(r.t, os.MkdirAll(filepath.Dir(path), 0o750))
		must.NoError(r.t, os.WriteFile(path, []byte(content), 0o600))
	}
}

// Commit writes files, commits everything, and returns the new commit's SHA.
func (r *Repo) Commit(message string, files map[string]string) string {
	r.t.Helper()

	r.Write(files)
	r.Git("add", "-A")
	r.Git("commit", "-q", "--allow-empty", "-m", message)

	return r.Git("rev-parse", "HEAD")
}
