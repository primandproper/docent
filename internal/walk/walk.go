// Package walk resolves the steps of a tour against the code they point at.
// The app and `docent validate` both go through it, so what validates is what
// the reader sees.
package walk

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/links"
	"github.com/primandproper/docent/internal/mermaid"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const o11yName = "walker"

// The kinds of step, by what they point at.
const (
	KindFile      Kind = "file"
	KindDirectory Kind = "directory"
	KindURI       Kind = "uri"
	KindContent   Kind = "content"
)

type (
	// Kind is what a step points at.
	Kind string

	// Step is one step resolved against the code.
	Step struct {
		_ struct{}
		// Anchor is set for file steps.
		Anchor *anchor.Resolution
		Kind   Kind
		// Root is the directory Path is relative to.
		Root string
		// Path is the step's file or directory.
		Path string
		// Readme is the directory's README, for directory steps that have one.
		Readme string
		// Problem and Detail say why a directory couldn't be listed.
		Problem diag.Code
		Detail  string
		// Ref is what the step's code is being read at.
		Ref *Ref
		// Repo is the logical repo the step lives in, or "" for the tour's own root.
		Repo string
		// Body is the step's Markdown body file, when it has one.
		Body string
		// BodyProblem and BodyDetail say why a body file couldn't be read.
		BodyProblem diag.Code
		BodyDetail  string
		// Entries are the directory's children, for directory steps.
		Entries []gitsrc.Entry
		// CloneCommand would fetch the step's repo, when it isn't checked out.
		CloneCommand []string
		// Links are the step's link cards, expanded.
		Links []links.Card
		// Extras are the step's additional anchors, shown as tabs.
		Extras []Extra
	}

	// Extra is one of a step's additional anchors, resolved.
	Extra struct {
		_ struct{}
		// Code holds the anchor's Root, Repo, Path, Ref, Anchor and
		// CloneCommand, exactly as a file step's would be.
		Code  *Step
		Label string
	}

	// Dependencies are what a Walker resolves steps with.
	Dependencies struct {
		_      struct{}
		Source gitsrc.FileSource
		// Git answers history questions: refs, stamps and drift. Nil means
		// everything is read from the working tree.
		Git gitsrc.Git
		// Repos locates the repos a tour names. Nil means none are checked out.
		Repos workspace.RepoResolver
		// Vars fills in link templates. Nil means only tour defaults apply.
		Vars           *links.Vars
		Logger         logging.Logger
		TracerProvider tracing.Provider
	}

	// Walker resolves steps.
	Walker interface {
		Step(ctx context.Context, t *tour.Tour, index int) (*Step, error)
		// Repo reports where one of a tour's repos stands: found or not, what
		// is being read, and how far that is from the tour's stamp.
		Repo(ctx context.Context, t *tour.Tour, name string) (*RepoState, error)
		// Map loads the tour's system map and lists what it defines. It
		// returns nil for a tour without one.
		Map(ctx context.Context, t *tour.Tour) *Map
		// SetWorkingTree switches between reading at each ref and reading
		// files as they are on disk, uncommitted changes included.
		SetWorkingTree(on bool)
		// Reset forgets what refs resolved to, so a fetch or a new commit is
		// seen. Call it when the reader reloads.
		Reset()
	}

	// Map is a tour's system map.
	Map struct {
		_ struct{}
		// Defined is what the flowchart declares.
		Defined *mermaid.Map
		// Source is the Mermaid flowchart, or "" when it couldn't be loaded.
		Source string
		// Problem and Detail say why not.
		Problem diag.Code
		Detail  string
	}

	// RepoState is where one of a tour's repos stands on this machine.
	RepoState struct {
		_   struct{} `json:"-"`
		Ref *Ref     `json:"ref"`
		// StampedAt and LastFetch are zero when unknown.
		StampedAt time.Time `json:"stampedAt"`
		LastFetch time.Time `json:"lastFetch"`
		Name      string    `json:"name"`
		Remote    string    `json:"remote"`
		// Root is the checkout, or "" when it isn't on this machine.
		Root string `json:"root"`
		// Stamp is the commit the tour was written against, in full, or ""
		// when the tour has none or it isn't in the local repository.
		Stamp string `json:"stamp"`
		// CloneCommand would fetch a missing repo.
		CloneCommand []string `json:"cloneCommand"`
		// Behind is how many commits the stamp is behind what is being read.
		Behind int  `json:"behind"`
		Found  bool `json:"found"`
		// StampUnreachable is set when the tour has a stamp that isn't here.
		StampUnreachable bool `json:"stampUnreachable"`
	}

	walker struct {
		_           struct{}
		resolver    anchor.Resolver
		source      gitsrc.FileSource
		repos       workspace.RepoResolver
		git         gitsrc.Git
		logger      logging.Logger
		tracer      tracing.Tracer
		vars        *links.Vars
		refs        map[string]*Ref
		mu          sync.Mutex
		workingTree bool
	}
)

var (
	// ErrNoSuchStep is returned for an index outside the tour.
	ErrNoSuchStep = errors.New("no such step")
	// ErrNoSuchRepo is returned for a repo the tour doesn't define.
	ErrNoSuchRepo = errors.New("no such repo")
)

var _ Walker = (*walker)(nil)

// NewWalker returns a Walker.
func NewWalker(deps *Dependencies) Walker {
	vars := deps.Vars
	if vars == nil {
		vars = links.NewVars(nil, nil)
	}

	return &walker{
		resolver: anchor.NewResolver(deps.Source, deps.Logger, deps.TracerProvider),
		source:   deps.Source,
		repos:    deps.Repos,
		vars:     vars,
		git:      deps.Git,
		refs:     map[string]*Ref{},
		logger:   logging.NewNamedLogger(deps.Logger, o11yName),
		tracer:   tracing.NewNamedTracer(deps.TracerProvider, o11yName),
	}
}

// KindOf classifies a step the way CodeTour does: a file wins over a
// directory, which wins over a URI; a step with none of them is content only.
func KindOf(s *tour.Step) Kind {
	switch {
	case s.File != "":
		return KindFile
	case s.Directory != "":
		return KindDirectory
	case s.URI != "":
		return KindURI
	default:
		return KindContent
	}
}

func (w *walker) Step(ctx context.Context, t *tour.Tour, index int) (*Step, error) {
	ctx, span := w.tracer.StartSpan(ctx)
	defer span.End()

	if index < 0 || index >= len(t.Steps) {
		return nil, fmt.Errorf("step %d of %d: %w", index, len(t.Steps), ErrNoSuchStep)
	}

	s := &t.Steps[index]
	out := &Step{Kind: KindOf(s), Root: t.Root, Repo: s.Repo, Links: make([]links.Card, 0, len(s.Links))}
	logger := w.logger.WithValues(map[string]any{"tour": t.Path, "step": index})

	for i := range s.Links {
		out.Links = append(out.Links, w.vars.Expand(&s.Links[i], t.Vars))
	}

	w.body(ctx, t, s, out)

	switch out.Kind {
	case KindFile:
		out.Path = s.File
	case KindDirectory:
		out.Path = s.Directory
	case KindURI, KindContent:
	}

	if out.Kind == KindFile || out.Kind == KindDirectory {
		if err := w.code(ctx, t, s, out); err != nil {
			return nil, observability.PrepareAndLogError(err, logger, span, "resolving step")
		}
	}

	for i := range s.Anchors {
		a := &s.Anchors[i]

		// An extra anchor is a file reference like the step's own, in the
		// step's repo unless it names another.
		pseudo := &tour.Step{
			Repo: a.Repo, File: a.File, Line: a.Line, Pattern: a.Pattern,
			Selection: a.Selection, ContentHash: a.ContentHash, Ref: s.Ref,
		}
		if pseudo.Repo == "" {
			pseudo.Repo = s.Repo
		}

		extra := &Step{Kind: KindFile, Root: t.Root, Repo: pseudo.Repo, Path: a.File}
		if err := w.code(ctx, t, pseudo, extra); err != nil {
			return nil, observability.PrepareAndLogError(err, logger, span, "resolving anchor %q", a.Label)
		}

		out.Extras = append(out.Extras, Extra{Label: a.Label, Code: extra})
	}

	return out, nil
}

// code resolves what a file or directory step points at: which repo, at
// which ref, and where in the file.
func (w *walker) code(ctx context.Context, t *tour.Tour, s *tour.Step, out *Step) error {
	repo, problem, detail, err := w.locateRepo(ctx, t, s, out)
	if err != nil {
		return fmt.Errorf("locating repo: %w", err)
	}

	if problem == "" {
		out.Ref = w.resolveRef(ctx, out.Root, stepRef(t, s, repo))

		if out.Ref.Problem == diag.CodeRefMissing {
			problem, detail = out.Ref.Problem, out.Ref.Detail
		}
	}

	switch {
	case problem != "" && out.Kind == KindFile:
		out.Anchor = &anchor.Resolution{File: s.File, Location: anchor.Location{Status: anchor.StatusMissing, Problem: problem, Detail: detail}}
	case problem != "":
		out.Problem, out.Detail = problem, detail
	case out.Kind == KindFile:
		if out.Anchor, err = w.file(ctx, s, repo, out); err != nil {
			return fmt.Errorf("resolving anchor: %w", err)
		}

		out.Path = out.Anchor.File
	default:
		if err = w.directory(ctx, out); err != nil {
			return fmt.Errorf("listing directory: %w", err)
		}
	}

	return nil
}

// stepRef picks the ref a step is read at: its own, else its repo's, else,
// for a step in the tour's own root, the tour's CodeTour `ref`.
func stepRef(t *tour.Tour, s *tour.Step, repo *tour.Repo) string {
	switch {
	case s.Ref != "":
		return s.Ref
	case repo != nil:
		return repo.Ref
	default:
		return t.Ref
	}
}

// file resolves a file step's anchor, following it forward from the repo's
// stamp when there is one.
func (w *walker) file(ctx context.Context, s *tour.Step, repo *tour.Repo, out *Step) (*anchor.Resolution, error) {
	req := &anchor.Request{
		Root:        out.Root,
		Ref:         out.Ref.Commit,
		File:        s.File,
		ContentHash: s.ContentHash,
		Spec:        anchor.Spec{Line: s.Line, Pattern: s.Pattern, Selection: s.Selection},
	}

	unreachable := false

	// A step with its own ref is pinned on purpose ("how it worked before"),
	// so it isn't followed anywhere.
	if repo != nil && repo.Stamp != "" && s.Ref == "" && out.Ref.Commit != "" && w.git != nil {
		stamp, err := w.git.Commit(ctx, out.Root, repo.Stamp)
		if err != nil {
			unreachable = true
		} else {
			req.Drift = &anchor.Drift{Stamp: stamp, Tip: out.Ref.Commit}
			// The hash describes the code at the stamp, and following the
			// diff from the stamp already says whether it changed.
			req.ContentHash = ""

			if stamp == out.Ref.Commit {
				req.ContentHash = s.ContentHash
			}
		}
	}

	res, err := w.resolver.Resolve(ctx, req)
	if err != nil {
		return nil, err
	}

	if unreachable && (res.Location.Status == anchor.StatusOK || res.Location.Status == anchor.StatusMoved) {
		res.Location.Status = anchor.StatusUnverified
		res.Location.Problem = diag.CodeStampUnreachable
		res.Location.Detail = fmt.Sprintf("the tour was written against %s, which isn't in the local repository, so this code can't be confirmed unchanged", repo.Stamp)
	}

	return res, nil
}

func (w *walker) SetWorkingTree(on bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.workingTree = on
}

func (w *walker) Reset() {
	w.mu.Lock()
	w.refs = map[string]*Ref{}
	w.mu.Unlock()

	// A clone since the last look changes which repos are here.
	if resettable, ok := w.repos.(interface{ Reset() }); ok {
		resettable.Reset()
	}
}

func (w *walker) Repo(ctx context.Context, t *tour.Tour, name string) (*RepoState, error) {
	ctx, span := w.tracer.StartSpan(ctx)
	defer span.End()

	var repo *tour.Repo

	for i := range t.Repos {
		if t.Repos[i].Name == name {
			repo = &t.Repos[i]
		}
	}

	if repo == nil {
		return nil, fmt.Errorf("repo %q: %w", name, ErrNoSuchRepo)
	}

	state := &RepoState{Name: repo.Name, Remote: repo.Remote, CloneCommand: []string{}}

	if w.repos == nil {
		return state, nil
	}

	checkout, err := w.repos.Resolve(ctx, repo)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, w.logger.WithValue("repo", name), span, "resolving repo")
	}

	if !checkout.Found {
		state.CloneCommand = checkout.CloneCommand
		return state, nil
	}

	state.Found, state.Root = true, checkout.Root
	state.Ref = w.resolveRef(ctx, checkout.Root, repo.Ref)

	if w.git == nil {
		return state, nil
	}

	if fetched, ok := w.git.LastFetch(ctx, checkout.Root); ok {
		state.LastFetch = fetched
	}

	if repo.Stamp == "" {
		return state, nil
	}

	stamp, err := w.git.Describe(ctx, checkout.Root, repo.Stamp)
	if err != nil {
		state.StampUnreachable = true
		return state, nil
	}

	state.Stamp, state.StampedAt = stamp.SHA, stamp.Time

	if state.Ref.Commit != "" {
		if behind, countErr := w.git.CountBetween(ctx, checkout.Root, stamp.SHA, state.Ref.Commit); countErr == nil {
			state.Behind = behind
		}
	}

	return state, nil
}

// locateRepo points out.Root at the step's repo. A step without `repo` stays
// rooted at the tour. A repo that is unknown or not checked out is a problem
// to report, not an error.
func (w *walker) locateRepo(ctx context.Context, t *tour.Tour, s *tour.Step, out *Step) (repo *tour.Repo, problem diag.Code, detail string, err error) {
	if s.Repo == "" {
		return nil, "", "", nil
	}

	for i := range t.Repos {
		if t.Repos[i].Name == s.Repo {
			repo = &t.Repos[i]
			break
		}
	}

	if repo == nil {
		return nil, diag.CodeRepoUnknown, fmt.Sprintf("repo %q is not defined in the tour's `repos`", s.Repo), nil
	}

	if w.repos == nil {
		return repo, diag.CodeRepoMissing, fmt.Sprintf("repo %q is not checked out on this machine", s.Repo), nil
	}

	checkout, err := w.repos.Resolve(ctx, repo)
	if err != nil {
		return nil, "", "", fmt.Errorf("resolving repo %q: %w", s.Repo, err)
	}

	if !checkout.Found {
		out.CloneCommand = checkout.CloneCommand
		return repo, diag.CodeRepoMissing, fmt.Sprintf("repo %q is not checked out on this machine", s.Repo), nil
	}

	out.Root = checkout.Root

	return repo, "", "", nil
}

// body loads the step's Markdown body, which lives beside the tour file.
func (w *walker) body(ctx context.Context, t *tour.Tour, s *tour.Step, out *Step) {
	if s.Body == "" {
		return
	}

	content, err := w.source.ReadFile(ctx, filepath.Dir(t.Path), "", s.Body)
	if err != nil {
		out.BodyProblem = diag.CodeBodyMissing

		switch {
		case errors.Is(err, gitsrc.ErrEscapesRoot):
			out.BodyProblem = diag.CodePathEscapesRoot
			out.BodyDetail = fmt.Sprintf("body %q resolves outside the tour's folder and was not read", s.Body)
		case errors.Is(err, gitsrc.ErrNotFound):
			out.BodyDetail = fmt.Sprintf("body %q does not exist; paths are relative to the tour file", s.Body)
		default:
			out.BodyDetail = fmt.Sprintf("body %q could not be read: %v", s.Body, err)
		}

		return
	}

	out.Body = string(content)
}

func (w *walker) directory(ctx context.Context, out *Step) error {
	entries, err := w.source.ReadDir(ctx, out.Root, out.Ref.Commit, out.Path)
	if err != nil {
		switch {
		case errors.Is(err, gitsrc.ErrEscapesRoot):
			out.Problem = diag.CodePathEscapesRoot
			out.Detail = fmt.Sprintf("%q resolves outside its root and was not read", out.Path)
		case errors.Is(err, gitsrc.ErrNotFound):
			out.Problem = diag.CodeFileMissing
			out.Detail = fmt.Sprintf("%q does not exist", out.Path)
		case errors.Is(err, gitsrc.ErrNotDirectory):
			out.Problem = diag.CodeFileMissing
			out.Detail = fmt.Sprintf("%q is a file, not a directory", out.Path)
		default:
			return err
		}

		return nil
	}

	out.Entries = entries

	for _, e := range entries {
		if e.IsDir || !strings.EqualFold(e.Name, "README.md") {
			continue
		}

		readme, readErr := w.source.ReadFile(ctx, out.Root, out.Ref.Commit, strings.TrimSuffix(out.Path, "/")+"/"+e.Name)
		if readErr != nil {
			// The listing is still worth showing without its README.
			w.logger.WithError(readErr).Warn("directory README is unreadable")
			break
		}

		out.Readme = string(readme)

		break
	}

	return nil
}

func (w *walker) Map(ctx context.Context, t *tour.Tour) *Map {
	if t.Map == nil {
		return nil
	}

	out := &Map{Source: t.Map.Mermaid, Defined: &mermaid.Map{}}

	if t.Map.File != "" {
		content, err := w.source.ReadFile(ctx, filepath.Dir(t.Path), "", t.Map.File)
		if err != nil {
			out.Problem = diag.CodeMapMissing
			out.Detail = fmt.Sprintf("map file %q could not be read; paths are relative to the tour file", t.Map.File)

			if errors.Is(err, gitsrc.ErrEscapesRoot) {
				out.Problem = diag.CodePathEscapesRoot
				out.Detail = fmt.Sprintf("map file %q resolves outside the tour's folder and was not read", t.Map.File)
			}

			return out
		}

		out.Source = string(content)
	}

	if !mermaid.IsFlowchart(out.Source) {
		out.Source = ""
		out.Problem = diag.CodeMapMissing
		out.Detail = "the system map must be a Mermaid flowchart (it should start with `flowchart`)"

		return out
	}

	out.Defined = mermaid.Parse(out.Source)

	return out
}
