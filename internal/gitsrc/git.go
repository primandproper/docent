package gitsrc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const gitO11yName = "git"

var (
	// ErrNotARepo means the directory isn't inside a git repository.
	ErrNotARepo = errors.New("not a git repository")
	// ErrRevNotFound means a revision doesn't name a commit in the repository.
	ErrRevNotFound = errors.New("revision not found")

	hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)
	hexSHA     = regexp.MustCompile(`^[0-9a-f]{7,64}$`)
)

type (
	// Hunk is one changed region of a diff. A count of zero means nothing on
	// that side: a pure insertion or a pure deletion.
	Hunk struct {
		_        struct{}
		OldStart int
		OldCount int
		NewStart int
		NewCount int
	}

	// Commit identifies a commit for display.
	Commit struct {
		_       struct{}  `json:"-"`
		Time    time.Time `json:"time"`
		SHA     string    `json:"sha"`
		Subject string    `json:"subject"`
	}

	// Git answers questions about a repository. Every method takes the
	// directory to run in, which may be anywhere inside the repo.
	Git interface {
		FileSource

		// Locate reports the repo's top level and dir's path inside it, with
		// a trailing slash unless it is the top itself.
		Locate(ctx context.Context, dir string) (top, prefix string, err error)
		// Commit resolves a revision to a full commit SHA.
		Commit(ctx context.Context, dir, rev string) (string, error)
		// Describe returns a commit's subject and time.
		Describe(ctx context.Context, dir, rev string) (*Commit, error)
		// RefExists reports whether a fully qualified ref exists.
		RefExists(ctx context.Context, dir, ref string) bool
		// DefaultBranch names the branch origin's HEAD points at, as a
		// remote-tracking ref, or "" when there isn't one.
		DefaultBranch(ctx context.Context, dir string) string
		// Origin returns the origin remote's URL, or "".
		Origin(ctx context.Context, dir string) string
		// UserName returns the name git would put on a commit made in dir, or "".
		UserName(ctx context.Context, dir string) string
		// LastFetch reports when the repo was last fetched, if it ever was.
		LastFetch(ctx context.Context, dir string) (time.Time, bool)
		// CountBetween counts the commits in from..to.
		CountBetween(ctx context.Context, dir, from, to string) (int, error)
		// RenamedTo finds where a file (relative to dir) went between two
		// commits, or returns "" when it wasn't renamed.
		RenamedTo(ctx context.Context, dir, from, to, file string) (string, error)
		// Diff returns the hunks that turn oldFile at from into newFile at to.
		Diff(ctx context.Context, dir, from, to, oldFile, newFile string) ([]Hunk, error)
		// DiffText returns the same change as a unified diff, for people.
		DiffText(ctx context.Context, dir, from, to, oldFile, newFile string) (string, error)
		// LastTouch finds the most recent commit in from..to that changed how
		// often text occurs in file, or that deleted file when text is "".
		LastTouch(ctx context.Context, dir, from, to, file, text string) (*Commit, error)
		// Fetch and Clone are the only things Docent ever does to a repo, and
		// only when the reader has agreed.
		Fetch(ctx context.Context, dir string) error
		Clone(ctx context.Context, remote, dest string) error
		// Close stops the helper processes.
		Close()
	}

	batch struct {
		_      struct{}
		cmd    *exec.Cmd
		stdin  io.WriteCloser
		stdout *bufio.Reader
	}

	location struct {
		_      struct{}
		top    string
		prefix string
	}

	git struct {
		_         struct{}
		worktree  FileSource
		logger    logging.Logger
		tracer    tracing.Tracer
		batches   map[string]*batch
		locations map[string]location
		// memo holds answers that can never change, because they are about
		// commits named by full SHA: what was renamed between two of them,
		// how a file differs between them, what a short SHA expands to.
		memo   map[string]any
		mu     sync.Mutex
		memoMu sync.Mutex
	}
)

var _ Git = (*git)(nil)

// NewGit returns a Git that shells out to the git on the PATH. As a
// FileSource it reads the working tree for an empty ref and git objects
// otherwise, so a read at a ref never touches the checkout.
func NewGit(logger logging.Logger, tracerProvider tracing.Provider) Git {
	return &git{
		worktree:  NewWorkingTree(logger, tracerProvider),
		logger:    logging.NewNamedLogger(logger, gitO11yName),
		tracer:    tracing.NewNamedTracer(tracerProvider, gitO11yName),
		batches:   map[string]*batch{},
		locations: map[string]location{},
		memo:      map[string]any{},
	}
}

// remember returns the memoized answer for key, computing it once. Only use
// it for questions whose answer is fixed by the SHAs in the key.
func remember[T any](g *git, key string, compute func() (T, error)) (T, error) {
	g.memoMu.Lock()
	cached, ok := g.memo[key]
	g.memoMu.Unlock()

	if ok {
		return cached.(T), nil //nolint:errcheck,forcetypeassert // each key prefix is only ever stored with one type
	}

	value, err := compute()
	if err != nil {
		return value, err
	}

	g.memoMu.Lock()
	g.memo[key] = value
	g.memoMu.Unlock()

	return value, nil
}

// isFullSHA reports whether rev names a commit immutably.
func isFullSHA(rev string) bool {
	return len(rev) >= 40 && hexSHA.MatchString(rev)
}

// run executes git in dir and returns its trimmed stdout.
func (g *git) run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...) //nolint:gosec // arguments are passed as argv, never through a shell
	// Never prompt: a fetch that needs credentials should fail, not hang.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimRight(string(out), "\n"), nil
}

func (g *git) Locate(ctx context.Context, dir string) (top, prefix string, err error) {
	g.mu.Lock()
	cached, ok := g.locations[dir]
	g.mu.Unlock()

	if ok {
		return cached.top, cached.prefix, nil
	}

	out, err := g.run(ctx, dir, "rev-parse", "--show-toplevel", "--show-prefix")
	if err != nil {
		return "", "", fmt.Errorf("%q: %w", dir, ErrNotARepo)
	}

	top, prefix, _ = strings.Cut(out, "\n")

	g.mu.Lock()
	g.locations[dir] = location{top: top, prefix: prefix}
	g.mu.Unlock()

	return top, prefix, nil
}

func (g *git) Commit(ctx context.Context, dir, rev string) (string, error) {
	resolve := func() (string, error) {
		// --end-of-options keeps a ref that starts with a dash from being read as a flag.
		out, err := g.run(ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
		if err != nil || out == "" {
			return "", fmt.Errorf("%q: %w", rev, ErrRevNotFound)
		}

		return out, nil
	}

	// A commit id, even abbreviated, names the same commit for as long as it
	// names one at all. A branch or tag can move, so those are always asked.
	if !LooksLikeSHA(rev) {
		return resolve()
	}

	return remember(g, "commit\x00"+dir+"\x00"+rev, resolve)
}

func (g *git) Describe(ctx context.Context, dir, rev string) (*Commit, error) {
	out, err := g.run(ctx, dir, "log", "-1", "--format=%H%x00%ct%x00%s", "--end-of-options", rev)
	if err != nil {
		return nil, fmt.Errorf("%q: %w", rev, ErrRevNotFound)
	}

	return parseCommit(out)
}

func parseCommit(line string) (*Commit, error) {
	parts := strings.SplitN(line, "\x00", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("unexpected git log output %q", line)
	}

	seconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parsing commit time %q: %w", parts[1], err)
	}

	return &Commit{SHA: parts[0], Time: time.Unix(seconds, 0).UTC(), Subject: parts[2]}, nil
}

func (g *git) RefExists(ctx context.Context, dir, ref string) bool {
	_, err := g.run(ctx, dir, "show-ref", "--verify", "--quiet", ref)
	return err == nil
}

func (g *git) DefaultBranch(ctx context.Context, dir string) string {
	if out, err := g.run(ctx, dir, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); err == nil && out != "" {
		return out
	}

	for _, candidate := range []string{"refs/remotes/origin/main", "refs/remotes/origin/master", "refs/heads/main", "refs/heads/master"} {
		if g.RefExists(ctx, dir, candidate) {
			return candidate
		}
	}

	return ""
}

func (g *git) Origin(ctx context.Context, dir string) string {
	out, err := g.run(ctx, dir, "config", "--get", "remote.origin.url")
	if err != nil {
		return ""
	}

	return out
}

func (g *git) UserName(ctx context.Context, dir string) string {
	out, err := g.run(ctx, dir, "config", "--get", "user.name")
	if err != nil {
		return ""
	}

	return out
}

func (g *git) LastFetch(ctx context.Context, dir string) (time.Time, bool) {
	gitDir, err := g.run(ctx, dir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return time.Time{}, false
	}

	info, err := os.Stat(filepath.Join(gitDir, "FETCH_HEAD"))
	if err != nil {
		return time.Time{}, false
	}

	return info.ModTime(), true
}

func (g *git) CountBetween(ctx context.Context, dir, from, to string) (int, error) {
	out, err := g.run(ctx, dir, "rev-list", "--count", from+".."+to)
	if err != nil {
		return 0, err
	}

	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parsing commit count %q: %w", out, err)
	}

	return n, nil
}

func (g *git) RenamedTo(ctx context.Context, dir, from, to, file string) (string, error) {
	top, prefix, err := g.Locate(ctx, dir)
	if err != nil {
		return "", err
	}

	// Rename detection needs both sides in view, so the diff can't be limited
	// to the old path: every rename between the two commits is found at once
	// and kept, since every anchor in the repo will ask.
	renames := func() (map[string]string, error) {
		out, runErr := g.run(ctx, top, "diff", "-M", "--name-status", "--diff-filter=R", "-z", from, to)
		if runErr != nil {
			return nil, runErr
		}

		found := map[string]string{}
		fields := strings.Split(out, "\x00")

		for i := 0; i+2 < len(fields); i += 3 {
			found[fields[i+1]] = fields[i+2]
		}

		return found, nil
	}

	var all map[string]string
	if isFullSHA(from) && isFullSHA(to) {
		all, err = remember(g, "renames\x00"+top+"\x00"+from+"\x00"+to, renames)
	} else {
		all, err = renames()
	}

	if err != nil {
		return "", err
	}

	return strings.TrimPrefix(all[prefix+filepath.ToSlash(file)], prefix), nil
}

func (g *git) diffArgs(ctx context.Context, dir, from, to, oldFile, newFile string, extra ...string) ([]string, error) {
	_, prefix, err := g.Locate(ctx, dir)
	if err != nil {
		return nil, err
	}

	args := append([]string{"diff", "--no-color", "--no-ext-diff"}, extra...)

	return append(args, from+":"+prefix+filepath.ToSlash(oldFile), to+":"+prefix+filepath.ToSlash(newFile)), nil
}

func (g *git) Diff(ctx context.Context, dir, from, to, oldFile, newFile string) ([]Hunk, error) {
	if !isFullSHA(from) || !isFullSHA(to) {
		return g.diff(ctx, dir, from, to, oldFile, newFile)
	}

	return remember(g, strings.Join([]string{"diff", dir, from, to, oldFile, newFile}, "\x00"), func() ([]Hunk, error) {
		return g.diff(ctx, dir, from, to, oldFile, newFile)
	})
}

func (g *git) diff(ctx context.Context, dir, from, to, oldFile, newFile string) ([]Hunk, error) {
	args, err := g.diffArgs(ctx, dir, from, to, oldFile, newFile, "-U0")
	if err != nil {
		return nil, err
	}

	out, err := g.run(ctx, dir, args...)
	if err != nil {
		return nil, err
	}

	var hunks []Hunk

	for line := range strings.SplitSeq(out, "\n") {
		m := hunkHeader.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		hunks = append(hunks, Hunk{
			OldStart: atoi(m[1], 0),
			OldCount: atoi(m[2], 1),
			NewStart: atoi(m[3], 0),
			NewCount: atoi(m[4], 1),
		})
	}

	return hunks, nil
}

func (g *git) DiffText(ctx context.Context, dir, from, to, oldFile, newFile string) (string, error) {
	args, err := g.diffArgs(ctx, dir, from, to, oldFile, newFile)
	if err != nil {
		return "", err
	}

	return g.run(ctx, dir, args...)
}

func atoi(s string, fallback int) int {
	if s == "" {
		return fallback
	}

	n, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}

	return n
}

func (g *git) LastTouch(ctx context.Context, dir, from, to, file, text string) (*Commit, error) {
	args := []string{"log", "-1", "--format=%H%x00%ct%x00%s"}
	if text == "" {
		args = append(args, "--diff-filter=D")
	} else {
		args = append(args, "-S"+text)
	}

	out, err := g.run(ctx, dir, append(args, from+".."+to, "--", filepath.ToSlash(file))...)
	if err != nil {
		return nil, err
	}

	if out == "" {
		return nil, nil //nolint:nilnil // no such commit is an answer
	}

	return parseCommit(out)
}

func (g *git) Fetch(ctx context.Context, dir string) error {
	g.logger.WithValue("dir", dir).Info("running git fetch")

	_, err := g.run(ctx, dir, "fetch", "--quiet", "--prune", "origin")

	return err
}

func (g *git) Clone(ctx context.Context, remote, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(dest), err)
	}

	g.logger.WithValues(map[string]any{"remote": remote, "dest": dest}).Info("running git clone")

	_, err := g.run(ctx, filepath.Dir(dest), "clone", "--quiet", "--end-of-options", remote, dest)

	return err
}

// ReadFile implements FileSource.
func (g *git) ReadFile(ctx context.Context, root, ref, file string) ([]byte, error) {
	if ref == "" {
		return g.worktree.ReadFile(ctx, root, ref, file)
	}

	_, span := g.tracer.StartSpan(ctx)
	defer span.End()

	clean, err := CleanRelative(file)
	if err != nil {
		return nil, err
	}

	top, prefix, err := g.Locate(ctx, root)
	if err != nil {
		return nil, err
	}

	kind, data, err := g.object(ctx, top, ref+":"+path.Join(prefix, filepath.ToSlash(clean)))
	if err != nil {
		return nil, err
	}

	switch kind {
	case "blob":
		return data, nil
	case "tree":
		return nil, fmt.Errorf("%q: %w", file, ErrIsDirectory)
	default:
		return nil, fmt.Errorf("%q at %s: %w", file, ref, ErrNotFound)
	}
}

// ReadDir implements FileSource.
func (g *git) ReadDir(ctx context.Context, root, ref, dir string) ([]Entry, error) {
	if ref == "" {
		return g.worktree.ReadDir(ctx, root, ref, dir)
	}

	clean, err := CleanRelative(dir)
	if err != nil {
		return nil, err
	}

	_, prefix, err := g.Locate(ctx, root)
	if err != nil {
		return nil, err
	}

	spec := ref + ":" + strings.TrimSuffix(path.Join(prefix, filepath.ToSlash(clean)), ".")

	out, err := g.run(ctx, root, "ls-tree", "-z", "--end-of-options", strings.TrimSuffix(spec, "/"))
	if err != nil {
		if strings.Contains(err.Error(), "not a tree object") {
			return nil, fmt.Errorf("%q: %w", dir, ErrNotDirectory)
		}

		return nil, fmt.Errorf("%q at %s: %w", dir, ref, ErrNotFound)
	}

	var entries []Entry

	for record := range strings.SplitSeq(out, "\x00") {
		meta, name, ok := strings.Cut(record, "\t")
		if !ok {
			continue
		}

		entries = append(entries, Entry{Name: name, IsDir: strings.Contains(meta, " tree ")})
	}

	SortEntries(entries)

	return entries, nil
}

// object reads one object through the repo's long-lived `git cat-file
// --batch`, starting it on first use. One process per repo, instead of one
// per file, is what keeps step navigation fast.
func (g *git) object(ctx context.Context, top, spec string) (kind string, data []byte, err error) {
	if strings.ContainsAny(spec, "\n\x00") {
		return "", nil, fmt.Errorf("%q: %w", spec, ErrNotFound)
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	b, ok := g.batches[top]
	if !ok {
		if b, err = startBatch(ctx, top); err != nil {
			return "", nil, err
		}

		g.batches[top] = b
	}

	kind, data, err = b.read(spec)
	if err != nil && !errors.Is(err, ErrNotFound) {
		// The process is in an unknown state; start afresh next time.
		b.close()
		delete(g.batches, top)
	}

	return kind, data, err
}

func startBatch(ctx context.Context, top string) (*batch, error) {
	// The process outlives the request that happened to start it.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), "git", "-C", top, "cat-file", "--batch") //nolint:gosec // top is a repo path this process resolved

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening git cat-file stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening git cat-file stdout: %w", err)
	}

	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting git cat-file: %w", err)
	}

	return &batch{cmd: cmd, stdin: stdin, stdout: bufio.NewReaderSize(stdout, 64<<10)}, nil
}

func (b *batch) read(spec string) (kind string, data []byte, err error) {
	if _, err = io.WriteString(b.stdin, spec+"\n"); err != nil {
		return "", nil, fmt.Errorf("asking git cat-file: %w", err)
	}

	header, err := b.stdout.ReadString('\n')
	if err != nil {
		return "", nil, fmt.Errorf("reading git cat-file: %w", err)
	}

	fields := strings.Fields(header)
	if strings.HasSuffix(header, " missing\n") || len(fields) != 3 {
		// "<spec> missing", or "ambiguous": either way there's no such object.
		return "", nil, fmt.Errorf("%q: %w", spec, ErrNotFound)
	}

	size, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", nil, fmt.Errorf("parsing git cat-file header %q: %w", header, err)
	}

	data = make([]byte, size+1)
	if _, err = io.ReadFull(b.stdout, data); err != nil {
		return "", nil, fmt.Errorf("reading git cat-file: %w", err)
	}

	return fields[1], data[:size], nil
}

func (b *batch) close() {
	_ = b.stdin.Close() //nolint:errcheck // closing stdin is what stops the process
	_ = b.cmd.Wait()    //nolint:errcheck // its exit status carries nothing useful
}

func (g *git) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()

	for top, b := range g.batches {
		b.close()
		delete(g.batches, top)
	}
}

// LooksLikeSHA reports whether a ref is written as a commit id.
func LooksLikeSHA(ref string) bool {
	return hexSHA.MatchString(ref)
}
