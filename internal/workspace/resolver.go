package workspace

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/primandproper/docent/internal/tour"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const o11yName = "repo_resolver"

// How a repo was found.
const (
	ByWorkspaceMapping How = "workspace-mapping"
	ByUserMapping      How = "user-mapping"
	ByConvention       How = "convention"
	ByRemote           How = "remote"
)

type (
	// How names the rule that located a repo.
	How string

	// Checkout is where a tour's repo lives on this machine.
	Checkout struct {
		_ struct{}
		// Root is the directory step paths are relative to: the checkout,
		// plus the repo's subdir when it has one. Empty when not found.
		Root string
		How  How
		// CloneCommand is what would fetch a missing repo, as argv.
		CloneCommand []string
		Found        bool
	}

	// RepoResolver maps the repos a tour names to local checkouts.
	RepoResolver interface {
		Resolve(ctx context.Context, repo *tour.Repo) (*Checkout, error)
	}

	resolver struct {
		_      struct{}
		config *Config
		logger logging.Logger
		tracer tracing.Tracer
		cache  map[string]*Checkout
		// origins caches each scanned directory's origin URL, normalized.
		origins map[string]string
		mu      sync.Mutex
	}
)

var _ RepoResolver = (*resolver)(nil)

// NewResolver returns a RepoResolver for a loaded configuration. Results are
// cached for the life of the resolver, which is one session.
func NewResolver(config *Config, logger logging.Logger, tracerProvider tracing.Provider) RepoResolver {
	return &resolver{
		config:  config,
		logger:  logging.NewNamedLogger(logger, o11yName),
		tracer:  tracing.NewNamedTracer(tracerProvider, o11yName),
		cache:   map[string]*Checkout{},
		origins: map[string]string{},
	}
}

// Resolve finds a repo's checkout. First match wins:
//
//  1. a mapping in the workspace file
//  2. a mapping in the user config
//  3. a directory named after the remote's repo name, under the opened folder
//     or any repo root
//  4. a git repo one level under those folders whose origin matches the remote
//
// A repo that isn't found is an answer (Found false, with the clone command),
// not an error.
func (r *resolver) Resolve(ctx context.Context, repo *tour.Repo) (*Checkout, error) {
	ctx, span := r.tracer.StartSpan(ctx)
	defer span.End()

	r.mu.Lock()
	defer r.mu.Unlock()

	key := repo.Name + "\x00" + repo.Remote + "\x00" + repo.Subdir
	if cached, ok := r.cache[key]; ok {
		return cached, nil
	}

	checkout := r.find(ctx, repo)

	if checkout.Found && repo.Subdir != "" {
		sub := filepath.Join(checkout.Root, filepath.FromSlash(repo.Subdir))
		if rel, err := filepath.Rel(checkout.Root, sub); err != nil || !filepath.IsLocal(rel) {
			return nil, fmt.Errorf("repo %q: subdir %q leaves the repo", repo.Name, repo.Subdir)
		}

		checkout.Root = sub
	}

	r.cache[key] = checkout
	r.logger.WithValues(map[string]any{"repo": repo.Name, "found": checkout.Found, "how": string(checkout.How), "root": checkout.Root}).Debug("resolved repo")

	return checkout, nil
}

func (r *resolver) find(ctx context.Context, repo *tour.Repo) *Checkout {
	if dir, ok := r.config.Workspace.Repos[repo.Name]; ok && isDir(dir) {
		return &Checkout{Found: true, Root: dir, How: ByWorkspaceMapping}
	}

	if dir, ok := r.config.User.Repos[repo.Name]; ok && isDir(dir) {
		return &Checkout{Found: true, Root: dir, How: ByUserMapping}
	}

	roots := r.config.RepoRoots()
	name := RepoName(repo.Remote)

	if name != "" {
		for _, root := range roots {
			if dir := filepath.Join(root, name); isDir(dir) {
				return &Checkout{Found: true, Root: dir, How: ByConvention}
			}
		}
	}

	if want := NormalizeRemote(repo.Remote); want != "" {
		for _, root := range roots {
			if dir, ok := r.scan(ctx, root, want); ok {
				return &Checkout{Found: true, Root: dir, How: ByRemote}
			}
		}
	}

	if name == "" {
		name = repo.Name
	}

	return &Checkout{CloneCommand: []string{"git", "clone", repo.Remote, filepath.Join(r.config.CloneRoot(), name)}}
}

// scan looks one level under root for a git repo whose origin is want.
func (r *resolver) scan(ctx context.Context, root, want string) (string, bool) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", false
	}

	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if !isDir(dir) {
			continue
		}

		origin, known := r.origins[dir]
		if !known {
			origin = originOf(ctx, dir)
			r.origins[dir] = origin
		}

		if origin == want {
			return dir, true
		}
	}

	return "", false
}

// originOf returns a directory's normalized origin URL, or "" when it isn't a
// git repo or has no origin.
func originOf(ctx context.Context, dir string) string {
	// Only a directory that is itself a checkout counts: without this, git
	// would walk up and answer for whatever repo contains dir.
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return ""
	}

	//nolint:gosec // dir is a folder this process just listed, passed as one argument
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		// No origin, or no git: either way this directory isn't a match.
		return ""
	}

	return NormalizeRemote(strings.TrimSpace(string(out)))
}

func isDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// NormalizeRemote reduces a git URL to "host/path" so SSH and HTTPS forms of
// the same repo compare equal: scheme, user, port, a trailing ".git" and a
// trailing slash are dropped, and the host is lowercased.
func NormalizeRemote(remote string) string {
	s := strings.TrimSpace(remote)
	if s == "" {
		return ""
	}

	var host, repoPath string

	switch {
	case strings.Contains(s, "://"):
		_, rest, _ := strings.Cut(s, "://")
		host, repoPath, _ = strings.Cut(rest, "/")
	case strings.Contains(s, ":") && !strings.HasPrefix(s, "/"):
		// scp-like: git@github.com:acme/api.git
		host, repoPath, _ = strings.Cut(s, ":")
	default:
		return strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	}

	if _, afterUser, ok := strings.Cut(host, "@"); ok {
		host = afterUser
	}

	host, _, _ = strings.Cut(host, ":")
	repoPath = strings.TrimSuffix(strings.TrimSuffix(strings.Trim(repoPath, "/"), ".git"), "/")

	return strings.ToLower(host) + "/" + repoPath
}

// RepoName is the last path element of a remote: "api" for acme/api.git.
func RepoName(remote string) string {
	normalized := NormalizeRemote(remote)
	if normalized == "" {
		return ""
	}

	return path.Base(normalized)
}
