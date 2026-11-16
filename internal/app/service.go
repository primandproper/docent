// Package app is the desktop app: a thin Wails binding layer over the same
// services the CLI calls. It is the trust boundary, so every argument that
// arrives from the frontend is validated here, whatever the frontend did.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"unicode/utf8"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const (
	o11yName = "app_service"

	// maxFileSize is the largest file the code pane is sent. Past it the step
	// says so instead of shipping the bytes to the webview.
	maxFileSize = 8 << 20
)

var (
	// ErrUnknownTour is returned for a tour path the workspace didn't discover.
	ErrUnknownTour = errors.New("not a tour in this workspace")
	// ErrTourUnreadable is returned when a tour can't be parsed at all.
	ErrTourUnreadable = errors.New("tour could not be read")
)

type (
	// Workspace describes the folder the app was opened on.
	Workspace struct {
		_    struct{} `json:"-"`
		Root string   `json:"root"`
		Name string   `json:"name"`
	}

	// TourSummary is one row of the tour picker.
	TourSummary struct {
		_ struct{} `json:"-"`
		// Path identifies the tour in later calls.
		Path string `json:"path"`
		// DisplayPath is Path relative to the workspace, when it is inside it.
		DisplayPath string   `json:"displayPath"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		Steps       int      `json:"steps"`
		// Errors and Warnings count the tour's diagnostics: its health.
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
		// Unreadable is set when the file couldn't be parsed at all.
		Unreadable bool `json:"unreadable"`
		// Unreviewed is set for an AI-drafted tour nobody has vouched for.
		Unreviewed bool `json:"unreviewed"`
		IsPrimary  bool `json:"isPrimary"`
	}

	// StepSummary is what the step rail needs to know about a step.
	StepSummary struct {
		_      struct{}      `json:"-"`
		Kind   walk.Kind     `json:"kind"`
		Status anchor.Status `json:"status"`
	}

	// TourView is an opened tour.
	TourView struct {
		_           struct{}          `json:"-"`
		Tour        *tour.Tour        `json:"tour"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
		// Steps runs parallel to Tour.Steps.
		Steps []StepSummary `json:"steps"`
	}

	// StepView is one step resolved against the code, ready to render.
	StepView struct {
		_    struct{}  `json:"-"`
		Kind walk.Kind `json:"kind"`
		// Path is the step's file or directory, relative to its root.
		Path string `json:"path"`
		// Content is the file's text, for file steps.
		Content string `json:"content"`
		// Unavailable says why Content is empty when the file exists but
		// can't be shown (binary, or too large).
		Unavailable string `json:"unavailable"`
		// Readme is the directory's README, for directory steps.
		Readme   string          `json:"readme"`
		Entries  []gitsrc.Entry  `json:"entries"`
		Location anchor.Location `json:"location"`
		Index    int             `json:"index"`
	}

	// Service is the API the frontend calls.
	Service struct {
		_         struct{}
		store     tour.Store
		walker    walk.Walker
		validator validate.Validator
		logger    logging.Logger
		tracer    tracing.Tracer
		open      map[string]*tour.Tour
		root      string
		known     []string
		mu        sync.Mutex
	}
)

// NewService returns the frontend-facing service for a workspace root.
func NewService(root string, store tour.Store, walker walk.Walker, validator validate.Validator, logger logging.Logger, tracerProvider tracing.Provider) *Service {
	return &Service{
		root:      root,
		store:     store,
		walker:    walker,
		validator: validator,
		logger:    logging.NewNamedLogger(logger, o11yName),
		tracer:    tracing.NewNamedTracer(tracerProvider, o11yName),
		open:      map[string]*tour.Tour{},
	}
}

// Workspace reports the folder the app was opened on.
func (s *Service) Workspace() Workspace {
	return Workspace{Root: s.root, Name: filepath.Base(s.root)}
}

// ListTours discovers the workspace's tours and summarizes each for the picker.
func (s *Service) ListTours(ctx context.Context) ([]TourSummary, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	paths, err := s.store.Discover(ctx, s.root)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, s.logger, span, "discovering tours")
	}

	s.mu.Lock()
	s.known = paths
	s.mu.Unlock()

	summaries := make([]TourSummary, 0, len(paths))

	for _, path := range paths {
		summary, sumErr := s.summarize(ctx, path)
		if sumErr != nil {
			return nil, observability.PrepareAndLogError(sumErr, s.logger.WithValue("tour", path), span, "summarizing tour")
		}

		summaries = append(summaries, summary)
	}

	s.logger.WithValue("count", len(summaries)).Info("listed tours")

	// Primary tours lead, as they do in CodeTour.
	slices.SortStableFunc(summaries, func(a, b TourSummary) int {
		switch {
		case a.IsPrimary && !b.IsPrimary:
			return -1
		case b.IsPrimary && !a.IsPrimary:
			return 1
		default:
			return 0
		}
	})

	return summaries, nil
}

func (s *Service) summarize(ctx context.Context, path string) (TourSummary, error) {
	summary := TourSummary{Path: path, DisplayPath: path, Tags: []string{}}

	if rel, err := filepath.Rel(s.root, path); err == nil && filepath.IsLocal(rel) {
		summary.DisplayPath = rel
	}

	diags, err := s.validator.Tour(ctx, path)
	if err != nil {
		return summary, fmt.Errorf("validating tour: %w", err)
	}

	for i := range diags {
		if diags[i].Severity == diag.SeverityError {
			summary.Errors++
		} else {
			summary.Warnings++
		}
	}

	t, _, err := s.store.Load(ctx, path)
	if err != nil {
		return summary, fmt.Errorf("loading tour: %w", err)
	}

	if t == nil {
		summary.Unreadable = true
		summary.Title = filepath.Base(path)

		return summary, nil
	}

	summary.Title = t.Title
	summary.Description = t.Description
	summary.Steps = len(t.Steps)
	summary.IsPrimary = t.IsPrimary
	summary.Unreviewed = t.GeneratedBy != nil && len(t.ReviewedBy) == 0

	if t.Tags != nil {
		summary.Tags = t.Tags
	}

	return summary, nil
}

// OpenTour loads a tour by the path ListTours gave for it.
func (s *Service) OpenTour(ctx context.Context, path string) (*TourView, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue("tour", path)

	if err := s.checkKnown(ctx, path); err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "opening tour")
	}

	t, diags, err := s.store.Load(ctx, path)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "loading tour")
	}

	if t == nil {
		message := "the file is not a tour"
		if len(diags) > 0 {
			message = diags[0].Message
		}

		return nil, observability.PrepareAndLogError(fmt.Errorf("%w: %s", ErrTourUnreadable, message), logger, span, "opening tour")
	}

	view := &TourView{Tour: t, Diagnostics: diags, Steps: make([]StepSummary, len(t.Steps))}
	if view.Diagnostics == nil {
		view.Diagnostics = []diag.Diagnostic{}
	}

	for i := range t.Steps {
		step, stepErr := s.walker.Step(ctx, t, i)
		if stepErr != nil {
			return nil, observability.PrepareAndLogError(stepErr, logger, span, "resolving step %d", i)
		}

		view.Steps[i] = StepSummary{Kind: step.Kind, Status: statusOf(step)}
	}

	s.mu.Lock()
	s.open[path] = t
	s.mu.Unlock()

	logger.WithValue("steps", len(t.Steps)).Info("opened tour")

	return view, nil
}

// Step resolves one step of a tour OpenTour has loaded. It answers from that
// load, so the indices the frontend holds stay meaningful until it reopens.
func (s *Service) Step(ctx context.Context, path string, index int) (*StepView, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{"tour": path, "step": index})

	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "resolving step")
	}

	step, err := s.walker.Step(ctx, t, index)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "resolving step")
	}

	view := &StepView{
		Index:   index,
		Kind:    step.Kind,
		Path:    step.Path,
		Readme:  step.Readme,
		Entries: step.Entries,
		Location: anchor.Location{
			Status:  statusOf(step),
			Problem: step.Problem,
			Detail:  step.Detail,
		},
	}

	if view.Entries == nil {
		view.Entries = []gitsrc.Entry{}
	}

	if step.Anchor != nil {
		view.Location = step.Anchor.Location

		switch content := step.Anchor.Content; {
		case content == nil:
		case len(content) > maxFileSize:
			view.Unavailable = fmt.Sprintf("This file is %d MB, too large to show.", len(content)>>20)
		case bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content):
			view.Unavailable = "This is a binary file."
		default:
			view.Content = string(content)
		}
	}

	return view, nil
}

// checkKnown confirms path is a tour this workspace discovered, looking again
// once in case it was created since the picker last listed.
func (s *Service) checkKnown(ctx context.Context, path string) error {
	s.mu.Lock()
	known := slices.Contains(s.known, path)
	s.mu.Unlock()

	if known {
		return nil
	}

	paths, err := s.store.Discover(ctx, s.root)
	if err != nil {
		return fmt.Errorf("discovering tours: %w", err)
	}

	s.mu.Lock()
	s.known = paths
	s.mu.Unlock()

	if !slices.Contains(paths, path) {
		return fmt.Errorf("%q: %w", path, ErrUnknownTour)
	}

	return nil
}

func statusOf(step *walk.Step) anchor.Status {
	switch {
	case step.Anchor != nil:
		return step.Anchor.Location.Status
	case step.Problem != "":
		return anchor.StatusMissing
	default:
		return anchor.StatusOK
	}
}
