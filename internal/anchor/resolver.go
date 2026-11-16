package anchor

import (
	"context"
	"errors"
	"fmt"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const o11yName = "anchor_resolver"

type (
	// Request names a file and where in it to point.
	Request struct {
		_ struct{}
		// Root is the directory File is relative to.
		Root string
		// Ref is the git revision to read at, or "" for the working tree.
		Ref  string
		File string
		Spec Spec
	}

	// Resolution is a Request answered: the file's content and where the
	// anchor landed in it.
	Resolution struct {
		_ struct{}
		// Content is nil when the file couldn't be read.
		Content  []byte
		Location Location
		// LineCount is the number of lines in Content.
		LineCount int
	}

	// Resolver turns anchors into concrete ranges.
	Resolver interface {
		Resolve(ctx context.Context, req *Request) (*Resolution, error)
	}

	resolver struct {
		_      struct{}
		source gitsrc.FileSource
		logger logging.Logger
		tracer tracing.Tracer
	}
)

var _ Resolver = (*resolver)(nil)

// NewResolver returns a Resolver that reads files through source.
func NewResolver(source gitsrc.FileSource, logger logging.Logger, tracerProvider tracing.Provider) Resolver {
	return &resolver{
		source: source,
		logger: logging.NewNamedLogger(logger, o11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, o11yName),
	}
}

// Resolve reads the file and locates the anchor in it. A file that is missing
// or outside its root is an answer (status missing), not an error.
func (r *resolver) Resolve(ctx context.Context, req *Request) (*Resolution, error) {
	ctx, span := r.tracer.StartSpan(ctx)
	defer span.End()

	logger := r.logger.WithValues(map[string]any{"root": req.Root, "ref": req.Ref, "file": req.File})

	content, err := r.source.ReadFile(ctx, req.Root, req.Ref, req.File)
	if err != nil {
		loc := Location{Status: StatusMissing}

		switch {
		case errors.Is(err, gitsrc.ErrEscapesRoot):
			loc.Problem = diag.CodePathEscapesRoot
			loc.Detail = fmt.Sprintf("%q resolves outside its root and was not read", req.File)
		case errors.Is(err, gitsrc.ErrNotFound):
			loc.Problem = diag.CodeFileMissing
			loc.Detail = fmt.Sprintf("%q does not exist", req.File)
		case errors.Is(err, gitsrc.ErrIsDirectory):
			loc.Problem = diag.CodeFileMissing
			loc.Detail = fmt.Sprintf("%q is a directory, not a file", req.File)
		default:
			return nil, observability.PrepareAndLogError(err, logger, span, "reading anchored file")
		}

		logger.WithValue("problem", string(loc.Problem)).Debug("anchored file is unavailable")

		return &Resolution{Location: loc}, nil
	}

	lines := SplitLines(content)

	return &Resolution{
		Content:   content,
		Location:  Locate(lines, req.Spec),
		LineCount: len(lines),
	}, nil
}
