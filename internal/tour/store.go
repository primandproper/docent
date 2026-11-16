package tour

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/primandproper/docent/internal/diag"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const o11yName = "tour_store"

type (
	// Store finds and loads tours.
	Store interface {
		// Discover lists the tour files visible from a workspace root.
		Discover(ctx context.Context, root string, extraPaths ...string) ([]string, error)
		// Load reads and parses one tour. The Tour is nil when the file can't
		// be read or isn't a JSON object, and the diagnostics say why; the
		// error is reserved for failures that aren't about the tour itself.
		Load(ctx context.Context, path string) (*Tour, []diag.Diagnostic, error)
	}

	fileStore struct {
		_      struct{}
		logger logging.Logger
		tracer tracing.Tracer
	}
)

var _ Store = (*fileStore)(nil)

// NewStore returns a Store backed by the filesystem.
func NewStore(logger logging.Logger, tracerProvider tracing.Provider) Store {
	return &fileStore{
		logger: logging.NewNamedLogger(logger, o11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, o11yName),
	}
}

func (s *fileStore) Discover(ctx context.Context, root string, extraPaths ...string) ([]string, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue("root", root)

	paths, err := Discover(root, extraPaths...)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "discovering tours")
	}

	logger.WithValue("count", len(paths)).Debug("discovered tours")

	return paths, nil
}

func (s *fileStore) Load(ctx context.Context, path string) (*Tour, []diag.Diagnostic, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue("path", path)

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, observability.PrepareAndLogError(err, logger, span, "resolving tour path")
	}

	data, err := os.ReadFile(abs)
	if err != nil {
		logger.WithError(err).Warn("tour file is unreadable")

		return nil, []diag.Diagnostic{{
			File:     abs,
			Severity: diag.SeverityError,
			Code:     diag.CodeReadFailed,
			Message:  fmt.Sprintf("reading the tour file: %v", err),
		}}, nil
	}

	t, diags, err := Parse(abs, data)
	if err != nil {
		return nil, nil, observability.PrepareAndLogError(err, logger, span, "parsing tour")
	}

	if t != nil {
		t.Root = RootFor(abs)
	}

	logger.WithValue("diagnostics", len(diags)).Debug("loaded tour")

	return t, diags, nil
}
