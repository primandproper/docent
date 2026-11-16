package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/tour"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

func tempDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	return dir
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()

	dir := filepath.Join(parts...)
	must.NoError(t, os.MkdirAll(dir, 0o750))

	return dir
}

func write(t *testing.T, path, content string) {
	t.Helper()

	must.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	must.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// gitRepo makes dir a git repo whose origin is remote.
func gitRepo(t *testing.T, dir, remote string) {
	t.Helper()

	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", remote}} {
		cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		must.NoError(t, err, must.Sprintf("git %v: %s", args, out))
	}
}

func TestLoad(T *testing.T) {
	T.Parallel()

	T.Run("reads the workspace file from an ancestor and the user config", func(t *testing.T) {
		t.Parallel()

		top := tempDir(t)
		opened := mkdir(t, top, "a", "b")
		write(t, filepath.Join(top, FileName), `{
			// comments and trailing commas are fine
			"repoRoots": ["work"],
			"repos": { "api": "./api-service" },
			"tourPaths": ["team-tours"],
			"vars": { "grafana": "https://grafana.example.com" },
			"editors": { "default": "zed" },
		}`)

		userPath := filepath.Join(tempDir(t), "config.json")
		write(t, userPath, `{
			"repos": { "worker": "/abs/worker" },
			"vars": { "grafana": "https://user.example.com", "env": "prod" },
			"editors": { "default": "code", "custom": { "helix": { "argv": ["hx", "{path}:{line}"] } } }
		}`)

		cfg, err := Load(opened, userPath)
		must.NoError(t, err)

		test.Eq(t, filepath.Join(top, FileName), cfg.WorkspacePath)
		test.Eq(t, userPath, cfg.UserPath)
		test.Eq(t, filepath.Join(top, "api-service"), cfg.Workspace.Repos["api"])
		test.Eq(t, "/abs/worker", cfg.User.Repos["worker"])
		test.Eq(t, []string{opened, filepath.Join(top, "work"), filepath.Dir(opened)}, cfg.RepoRoots())
		test.Eq(t, filepath.Join(top, "work"), cfg.CloneRoot())
		test.Eq(t, []string{filepath.Join(top, "team-tours")}, cfg.TourPaths())
		test.Eq(t, "zed", cfg.DefaultEditor())
		test.Eq(t, []string{"hx", "{path}:{line}"}, cfg.CustomEditors()["helix"].Argv)

		value, source, ok := cfg.Var("grafana")
		test.True(t, ok)
		test.Eq(t, "https://grafana.example.com", value)
		test.Eq(t, "workspace", source)

		value, source, ok = cfg.Var("env")
		test.True(t, ok)
		test.Eq(t, "prod", value)
		test.Eq(t, "user", source)

		_, _, ok = cfg.Var("nope")
		test.False(t, ok)
	})

	T.Run("works with no files at all", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)

		cfg, err := Load(opened, filepath.Join(opened, "missing.json"))
		must.NoError(t, err)
		test.Eq(t, "", cfg.WorkspacePath)
		test.Eq(t, "", cfg.UserPath)
		test.Eq(t, []string{opened, filepath.Dir(opened)}, cfg.RepoRoots())
		test.Eq(t, opened, cfg.CloneRoot())

		// An opened folder that is itself a checkout clones beside itself.
		mkdir(t, opened, ".git")
		test.Eq(t, filepath.Dir(opened), cfg.CloneRoot())
	})

	T.Run("a malformed file is an error that names it", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		write(t, filepath.Join(opened, FileName), `{ "repos": [ }`)

		_, err := Load(opened, "")
		test.ErrorContains(t, err, FileName)
	})

	T.Run("custom editors in a workspace file are ignored", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		write(t, filepath.Join(opened, FileName), `{ "editors": { "custom": { "evil": { "argv": ["rm", "-rf", "/"] } } } }`)

		cfg, err := Load(opened, "")
		must.NoError(t, err)
		test.MapEmpty(t, cfg.CustomEditors())
	})
}

func TestResolver_Resolve(T *testing.T) {
	T.Parallel()

	repo := &tour.Repo{Name: "api", Remote: "git@github.com:acme/api.git"}

	T.Run("a workspace mapping wins", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		mapped := mkdir(t, opened, "elsewhere")
		mkdir(t, opened, "api")

		cfg := &Config{Root: opened, Workspace: File{Repos: map[string]string{"api": mapped}}, User: File{Repos: map[string]string{"api": opened}}}

		got, err := NewResolver(cfg, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.Eq(t, &Checkout{Found: true, Root: mapped, How: ByWorkspaceMapping}, got)
	})

	T.Run("then a user mapping", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		mapped := mkdir(t, opened, "elsewhere")
		mkdir(t, opened, "api")

		cfg := &Config{Root: opened, User: File{Repos: map[string]string{"api": mapped}}}

		got, err := NewResolver(cfg, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.Eq(t, ByUserMapping, got.How)
		test.Eq(t, mapped, got.Root)
	})

	T.Run("then a sibling folder named after the remote", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		elsewhere := tempDir(t)
		dir := mkdir(t, elsewhere, "api")

		cfg := &Config{Root: opened, Workspace: File{RepoRoots: []string{elsewhere}}}

		got, err := NewResolver(cfg, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.Eq(t, &Checkout{Found: true, Root: dir, How: ByConvention}, got)
	})

	T.Run("then a checkout whose origin matches, whatever its folder is called", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		other := mkdir(t, opened, "something-else")
		gitRepo(t, other, "https://github.com/acme/web.git")
		dir := mkdir(t, opened, "api-service")
		gitRepo(t, dir, "https://GitHub.com/acme/api")
		mkdir(t, opened, "not-a-repo")

		got, err := NewResolver(&Config{Root: opened}, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.Eq(t, &Checkout{Found: true, Root: dir, How: ByRemote}, got)
	})

	T.Run("then a sibling of the opened folder", func(t *testing.T) {
		t.Parallel()

		top := tempDir(t)
		opened := mkdir(t, top, "team-tours")
		dir := mkdir(t, top, "api")

		got, err := NewResolver(&Config{Root: opened}, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.Eq(t, &Checkout{Found: true, Root: dir, How: ByConvention}, got)
	})

	T.Run("a missing repo comes back with its clone command", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		roots := tempDir(t)

		cfg := &Config{Root: opened, User: File{RepoRoots: []string{roots}}}

		got, err := NewResolver(cfg, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.False(t, got.Found)
		test.Eq(t, []string{"git", "clone", "git@github.com:acme/api.git", filepath.Join(roots, "api")}, got.CloneCommand)
	})

	T.Run("a mapping to a folder that isn't there falls through", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		dir := mkdir(t, opened, "api")

		cfg := &Config{Root: opened, Workspace: File{Repos: map[string]string{"api": filepath.Join(opened, "gone")}}}

		got, err := NewResolver(cfg, nil, nil).Resolve(t.Context(), repo)
		must.NoError(t, err)
		test.Eq(t, ByConvention, got.How)
		test.Eq(t, dir, got.Root)
	})

	T.Run("subdir narrows the root, and may not leave the repo", func(t *testing.T) {
		t.Parallel()

		opened := tempDir(t)
		dir := mkdir(t, opened, "api")
		r := NewResolver(&Config{Root: opened}, nil, nil)

		got, err := r.Resolve(t.Context(), &tour.Repo{Name: "api", Remote: repo.Remote, Subdir: "services/items"})
		must.NoError(t, err)
		test.Eq(t, filepath.Join(dir, "services", "items"), got.Root)

		_, err = r.Resolve(t.Context(), &tour.Repo{Name: "api", Remote: repo.Remote, Subdir: "../other"})
		test.ErrorContains(t, err, "leaves the repo")
	})
}

func TestNormalizeRemote(T *testing.T) {
	T.Parallel()

	T.Run("ssh and https forms of one repo are equal", func(t *testing.T) {
		t.Parallel()

		for _, remote := range []string{
			"git@github.com:acme/api.git",
			"git@github.com:acme/api",
			"https://github.com/acme/api.git",
			"https://github.com/acme/api/",
			"https://user@GitHub.com/acme/api",
			"ssh://git@github.com:22/acme/api.git",
			"  git@github.com:acme/api.git\n",
		} {
			test.Eq(t, "github.com/acme/api", NormalizeRemote(remote), test.Sprint(remote))
		}

		test.Eq(t, "", NormalizeRemote(""))
		test.Eq(t, "/srv/git/api", NormalizeRemote("/srv/git/api.git"))
	})
}

func TestRepoName(T *testing.T) {
	T.Parallel()

	T.Run("is the last element of the remote", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, "api", RepoName("git@github.com:acme/api.git"))
		test.Eq(t, "api", RepoName("https://gitlab.com/group/sub/api"))
		test.Eq(t, "", RepoName(""))
	})
}
