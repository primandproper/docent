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

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/links"
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
	}

	// Dependencies are what a Walker resolves steps with.
	Dependencies struct {
		_      struct{}
		Source gitsrc.FileSource
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
	}

	walker struct {
		_        struct{}
		resolver anchor.Resolver
		source   gitsrc.FileSource
		repos    workspace.RepoResolver
		vars     *links.Vars
		logger   logging.Logger
		tracer   tracing.Tracer
	}
)

// ErrNoSuchStep is returned for an index outside the tour.
var ErrNoSuchStep = errors.New("no such step")

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
		return out, nil
	}

	problem, detail, err := w.locateRepo(ctx, t, s, out)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "locating step repo")
	}

	switch {
	case problem != "" && out.Kind == KindFile:
		out.Anchor = &anchor.Resolution{Location: anchor.Location{Status: anchor.StatusMissing, Problem: problem, Detail: detail}}
	case problem != "":
		out.Problem, out.Detail = problem, detail
	case out.Kind == KindFile:
		res, resolveErr := w.resolver.Resolve(ctx, &anchor.Request{
			Root: out.Root,
			File: s.File,
			Spec: anchor.Spec{Line: s.Line, Pattern: s.Pattern, Selection: s.Selection},
		})
		if resolveErr != nil {
			return nil, observability.PrepareAndLogError(resolveErr, logger, span, "resolving step anchor")
		}

		out.Anchor = res
	default:
		if dirErr := w.directory(ctx, out); dirErr != nil {
			return nil, observability.PrepareAndLogError(dirErr, logger, span, "listing step directory")
		}
	}

	return out, nil
}

// locateRepo points out.Root at the step's repo. A step without `repo` stays
// rooted at the tour. A repo that is unknown or not checked out is a problem
// to report, not an error.
func (w *walker) locateRepo(ctx context.Context, t *tour.Tour, s *tour.Step, out *Step) (problem diag.Code, detail string, err error) {
	if s.Repo == "" {
		return "", "", nil
	}

	var repo *tour.Repo

	for i := range t.Repos {
		if t.Repos[i].Name == s.Repo {
			repo = &t.Repos[i]
			break
		}
	}

	if repo == nil {
		return diag.CodeRepoUnknown, fmt.Sprintf("repo %q is not defined in the tour's `repos`", s.Repo), nil
	}

	if w.repos == nil {
		return diag.CodeRepoMissing, fmt.Sprintf("repo %q is not checked out on this machine", s.Repo), nil
	}

	checkout, err := w.repos.Resolve(ctx, repo)
	if err != nil {
		return "", "", fmt.Errorf("resolving repo %q: %w", s.Repo, err)
	}

	if !checkout.Found {
		out.CloneCommand = checkout.CloneCommand
		return diag.CodeRepoMissing, fmt.Sprintf("repo %q is not checked out on this machine", s.Repo), nil
	}

	out.Root = checkout.Root

	return "", "", nil
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
	entries, err := w.source.ReadDir(ctx, out.Root, "", out.Path)
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

		readme, readErr := w.source.ReadFile(ctx, out.Root, "", strings.TrimSuffix(out.Path, "/")+"/"+e.Name)
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
