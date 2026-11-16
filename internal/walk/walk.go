// Package walk resolves the steps of a tour against the code they point at.
// The app and `docent validate` both go through it, so what validates is what
// the reader sees.
package walk

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"

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
		// Entries are the directory's children, for directory steps.
		Entries []gitsrc.Entry
	}

	// Walker resolves steps.
	Walker interface {
		Step(ctx context.Context, t *tour.Tour, index int) (*Step, error)
	}

	walker struct {
		_        struct{}
		resolver anchor.Resolver
		source   gitsrc.FileSource
		logger   logging.Logger
		tracer   tracing.Tracer
	}
)

// ErrNoSuchStep is returned for an index outside the tour.
var ErrNoSuchStep = errors.New("no such step")

var _ Walker = (*walker)(nil)

// NewWalker returns a Walker that reads code through source.
func NewWalker(source gitsrc.FileSource, logger logging.Logger, tracerProvider tracing.Provider) Walker {
	return &walker{
		resolver: anchor.NewResolver(source, logger, tracerProvider),
		source:   source,
		logger:   logging.NewNamedLogger(logger, o11yName),
		tracer:   tracing.NewNamedTracer(tracerProvider, o11yName),
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
	out := &Step{Kind: KindOf(s), Root: t.Root}
	logger := w.logger.WithValues(map[string]any{"tour": t.Path, "step": index})

	switch out.Kind {
	case KindFile:
		out.Path = s.File

		res, err := w.resolver.Resolve(ctx, &anchor.Request{
			Root: t.Root,
			File: s.File,
			Spec: anchor.Spec{Line: s.Line, Pattern: s.Pattern, Selection: s.Selection},
		})
		if err != nil {
			return nil, observability.PrepareAndLogError(err, logger, span, "resolving step anchor")
		}

		out.Anchor = res
	case KindDirectory:
		out.Path = s.Directory

		if err := w.directory(ctx, out); err != nil {
			return nil, observability.PrepareAndLogError(err, logger, span, "listing step directory")
		}
	case KindURI, KindContent:
	}

	return out, nil
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
