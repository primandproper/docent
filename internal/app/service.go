// Package app is the desktop app: a thin Wails binding layer over the same
// services the CLI calls. It is the trust boundary, so every argument that
// arrives from the frontend is validated here, whatever the frontend did.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/links"
	"github.com/primandproper/docent/internal/orderedjson"
	"github.com/primandproper/docent/internal/stamp"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const (
	o11yName = "app_service"
	// tourKey is the log key for the tour a call is about.
	tourKey = "tour"
	// stepKey is the log key for the step a call is about.
	stepKey = "step"

	// maxFileSize is the largest file the code pane is sent. Past it the step
	// says so instead of shipping the bytes to the webview.
	maxFileSize = 8 << 20
)

var (
	// ErrUnknownTour is returned for a tour path the workspace didn't discover.
	ErrUnknownTour = errors.New("not a tour in this workspace")
	// ErrTourUnreadable is returned when a tour can't be parsed at all.
	ErrTourUnreadable = errors.New("tour could not be read")
	// ErrNotAFile is returned when a step with no file is asked to open in an editor.
	ErrNotAFile = errors.New("this step doesn't point at a file")
	// ErrNoHost is returned when the folder can't be changed from here.
	ErrNoHost = errors.New("the folder can't be changed from inside the app")
	// ErrNoMapWindow is returned when there is no desktop shell to open the map window.
	ErrNoMapWindow = errors.New("the map can't be given a window of its own here")
	// ErrBadLink is returned for a docent:// link that isn't one Docent understands.
	ErrBadLink = errors.New("not a docent link")
	// ErrNotCloneable is returned for a repo that is already here, or whose
	// remote isn't a plain https or ssh git URL.
	ErrNotCloneable = errors.New("this repo can't be cloned from the app")
	// ErrNotRead is returned when a review is asked for before every step of
	// the tour has been read in this session.
	ErrNotRead = errors.New("this tour hasn't been read through yet")
	// ErrBadName is returned for a reviewer's name that can't be recorded.
	ErrBadName = errors.New("not a name a review can be signed with")
	// ErrBadVariable is returned for a variable or value the tour doesn't allow.
	ErrBadVariable = errors.New("not a value this tour's variable accepts")
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
		// Repos are the logical names of the repos the tour touches.
		Repos []string `json:"repos"`
		Steps int      `json:"steps"`
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

	// VarView is a link variable as the picker shows it.
	VarView struct {
		_           struct{} `json:"-"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		// Value is the effective value, and Source where it came from.
		Value   string   `json:"value"`
		Source  string   `json:"source"`
		Choices []string `json:"choices"`
		// Locked is set when the value was given on the command line, which
		// outranks the picker.
		Locked bool `json:"locked"`
	}

	// TourView is an opened tour.
	TourView struct {
		_           struct{}          `json:"-"`
		Tour        *tour.Tour        `json:"tour"`
		Diagnostics []diag.Diagnostic `json:"diagnostics"`
		// Steps runs parallel to Tour.Steps.
		Steps []StepSummary `json:"steps"`
		Vars  []VarView     `json:"vars"`
		// Map is the tour's system map as Mermaid source, or "".
		Map string `json:"map"`
		// MapProblem says why a map the tour names isn't shown.
		MapProblem string `json:"mapProblem"`
		// Repos is where each of the tour's repos stands on this machine.
		Repos []walk.RepoState `json:"repos"`
		// Review is how far the reader is from being able to vouch for the tour.
		Review ReviewState `json:"review"`
		// WorkingTree is set while files are read from disk, not at each ref.
		WorkingTree bool `json:"workingTree"`
	}

	// ReviewState is what stands between the reader and marking a tour
	// reviewed. A review says "I walked this and it is right", so it is only
	// offered once every step has been read since the tour was last opened
	// unchanged, in this run of the app.
	ReviewState struct {
		_ struct{} `json:"-"`
		// Name is the reviewer's name to offer: git's user.name, or "".
		Name string `json:"name"`
		// Blocker says why the tour can't be reviewed yet, or is "" when it can.
		Blocker string `json:"blocker"`
		// Read runs parallel to the tour's steps: which have been read.
		Read []bool `json:"read"`
		// Available is false for a plain CodeTour file, which has no reviews.
		Available bool `json:"available"`
		// Ready is set once nothing stands in the way.
		Ready bool `json:"ready"`
	}

	// progress is how much of one tour has been read. It is never saved: a
	// read-through has to happen in one sitting.
	progress struct {
		_ struct{}
		// mark identifies the tour's text, so an edit starts the read again.
		mark string
		// problem is what, other than unread steps, blocks a review.
		problem string
		name    string
		read    []bool
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
		Readme string `json:"readme"`
		// Body is the step's Markdown body file. When empty, the step's
		// description is the narrative.
		Body string `json:"body"`
		// BodyProblem says why a body file couldn't be shown.
		BodyProblem string `json:"bodyProblem"`
		// Repo is the logical repo the step lives in, when the tour names one.
		Repo string `json:"repo"`
		// Ref is what the code is being read at.
		Ref *walk.Ref `json:"ref"`
		// Then is the anchored code as it was when the tour was written, when
		// it has changed or been removed since.
		Then *anchor.Then `json:"then"`
		// Removed is the commit that removed the code, when one was found.
		Removed *gitsrc.Commit `json:"removed"`
		// Permalink is a GitHub URL for the anchored lines at the commit being
		// read, or "" when there isn't one.
		Permalink string `json:"permalink"`
		// CloneCommand fetches the step's repo when it isn't checked out,
		// quoted for pasting into a shell.
		CloneCommand string         `json:"cloneCommand"`
		Entries      []gitsrc.Entry `json:"entries"`
		Links        []links.Card   `json:"links"`
		// Anchors are the step's extra code references, shown as tabs.
		Anchors  []AnchorView    `json:"anchors"`
		Location anchor.Location `json:"location"`
		Index    int             `json:"index"`
	}

	// AnchorView is one of a step's extra anchors, ready to show as a tab.
	AnchorView struct {
		_            struct{}        `json:"-"`
		Ref          *walk.Ref       `json:"ref"`
		Then         *anchor.Then    `json:"then"`
		Removed      *gitsrc.Commit  `json:"removed"`
		Label        string          `json:"label"`
		Repo         string          `json:"repo"`
		Path         string          `json:"path"`
		Content      string          `json:"content"`
		Unavailable  string          `json:"unavailable"`
		Permalink    string          `json:"permalink"`
		CloneCommand string          `json:"cloneCommand"`
		Location     anchor.Location `json:"location"`
	}

	// FileView is a file opened from the file-tree drawer.
	FileView struct {
		_           struct{} `json:"-"`
		Path        string   `json:"path"`
		Content     string   `json:"content"`
		Unavailable string   `json:"unavailable"`
	}

	// Service is the API the frontend calls.
	// LaunchRequest is a tour the app was asked to open from outside: by
	// `docent open`, or by a docent:// link.
	LaunchRequest struct {
		_ struct{} `json:"-"`
		// Tour is the tour's path, or "" when there is nothing to open.
		Tour string `json:"tour"`
		// Step is the id (or 1-based number) of the step to start at, or "".
		Step string `json:"step"`
	}

	// Host is what the service needs from the desktop shell around it.
	Host struct {
		_ struct{}
		// Rebuild assembles the services for a different folder.
		Rebuild func(root string) (*core.Core, error)
		// ChooseFolder asks the reader to pick a folder. "" means they cancelled.
		ChooseFolder func() (string, error)
		// Rooted is told when the folder changes, to move the file watcher
		// and retitle the window.
		Rooted func(root string, services *core.Core)
		// ToggleMapWindow opens the window that shows only the system map, or
		// closes it if it is open.
		ToggleMapWindow func()
	}

	Service struct {
		_        struct{}
		core     *core.Core
		host     *Host
		pending  LaunchRequest
		launcher editor.Launcher
		logger   logging.Logger
		tracer   tracing.Tracer
		open     map[string]*tour.Tour
		progress map[string]*progress
		// write saves a tour. Tests replace it.
		write func(path string, raw *orderedjson.Value) error
		root  string
		known []string
		mu    sync.Mutex
		// workingTree mirrors the walker's toggle, for reporting.
		workingTree bool
	}
)

// NewService returns the frontend-facing service for a workspace root.
func NewService(root string, services *core.Core, launcher editor.Launcher, logger logging.Logger, tracerProvider tracing.Provider) *Service {
	return &Service{
		root:     root,
		core:     services,
		launcher: launcher,
		logger:   logging.NewNamedLogger(logger, o11yName),
		tracer:   tracing.NewNamedTracer(tracerProvider, o11yName),
		open:     map[string]*tour.Tour{},
		progress: map[string]*progress{},
		write:    tour.Write,
	}
}

// svc returns the services for the folder currently open.
func (s *Service) svc() *core.Core {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.core
}

// setHost connects the service to the desktop shell. Without one, the folder
// can't be changed from inside the app. It and the other unexported methods
// below are unexported on purpose: every exported method of Service is
// callable from the webview.
func (s *Service) setHost(host *Host) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.host = host
}

// request queues a tour to open, for the frontend to pick up with Launch.
func (s *Service) request(request LaunchRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pending = request
}

// Launch hands over the tour the app was asked to open from outside, once.
func (s *Service) Launch() LaunchRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	request := s.pending
	s.pending = LaunchRequest{}

	return request
}

// reroot points the service at a different folder.
func (s *Service) reroot(root string) error {
	s.mu.Lock()
	host := s.host
	s.mu.Unlock()

	if host == nil || host.Rebuild == nil {
		return ErrNoHost
	}

	services, err := host.Rebuild(root)
	if err != nil {
		return fmt.Errorf("opening %s: %w", root, err)
	}

	s.mu.Lock()
	previous := s.core
	s.core, s.root = services, root
	s.open = map[string]*tour.Tour{}
	s.progress = map[string]*progress{}
	s.known = nil
	s.mu.Unlock()

	if previous != nil {
		previous.Close()
	}

	if host.Rooted != nil {
		host.Rooted(root, services)
	}

	s.logger.WithValue("root", root).Info("opened folder")

	return nil
}

// ChooseFolder asks the reader for a folder and opens it. It returns the
// workspace as it now is, which is unchanged if they cancelled.
func (s *Service) ChooseFolder(ctx context.Context) (Workspace, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	s.mu.Lock()
	host := s.host
	s.mu.Unlock()

	if host == nil || host.ChooseFolder == nil {
		return s.Workspace(), observability.PrepareAndLogError(ErrNoHost, s.logger, span, "choosing a folder")
	}

	chosen, err := host.ChooseFolder()
	if err != nil {
		return s.Workspace(), observability.PrepareAndLogError(err, s.logger, span, "choosing a folder")
	}

	if chosen == "" {
		return s.Workspace(), nil
	}

	if err = s.reroot(chosen); err != nil {
		return s.Workspace(), observability.PrepareAndLogError(err, s.logger, span, "opening the chosen folder")
	}

	return s.Workspace(), nil
}

// ToggleMapWindow pops the system map out into a window of its own, or closes
// that window if it is open. It takes nothing from the caller: the window is
// always the app's own frontend.
func (s *Service) ToggleMapWindow(ctx context.Context) error {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	s.mu.Lock()
	host := s.host
	s.mu.Unlock()

	if host == nil || host.ToggleMapWindow == nil {
		return observability.PrepareAndLogError(ErrNoMapWindow, s.logger, span, "toggling the map window")
	}

	host.ToggleMapWindow()

	return nil
}

// openLink handles a docent://open link, which names a tour and a step in its query. The link is untrusted:
// all it can do is make the app open a .tour file that exists, which is no
// more than the reader could do by opening the folder it is in.
func (s *Service) openLink(ctx context.Context, link string) error {
	parsed, err := url.Parse(link)
	if err != nil || parsed.Scheme != "docent" || parsed.Host != "open" {
		return fmt.Errorf("%q: %w", link, ErrBadLink)
	}

	tourPath := parsed.Query().Get("tour")
	if tourPath == "" || !filepath.IsAbs(tourPath) || filepath.Ext(tourPath) != tour.Extension {
		return fmt.Errorf("%q: the tour must be an absolute path to a .tour file: %w", link, ErrBadLink)
	}

	tourPath = filepath.Clean(tourPath)

	if info, statErr := os.Stat(tourPath); statErr != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%q: no such tour: %w", link, ErrBadLink)
	}

	return s.requestTour(ctx, tourPath, parsed.Query().Get("step"))
}

// requestTour queues a tour to open, first moving to its folder if it isn't
// one of this workspace's tours.
func (s *Service) requestTour(ctx context.Context, tourPath, step string) error {
	s.mu.Lock()
	root := s.root
	s.mu.Unlock()

	known := false

	if root != "" {
		if paths, err := s.discover(ctx); err == nil {
			known = slices.Contains(paths, tourPath)
		}
	}

	if !known {
		if err := s.reroot(tour.RootFor(tourPath)); err != nil {
			return err
		}
	}

	s.request(LaunchRequest{Tour: tourPath, Step: step})

	return nil
}

// discover lists the workspace's tours, including the configured tour paths.
func (s *Service) discover(ctx context.Context) ([]string, error) {
	s.mu.Lock()
	root, services := s.root, s.core
	s.mu.Unlock()

	if root == "" {
		return nil, nil
	}

	return services.Store.Discover(ctx, root, services.Config.TourPaths()...)
}

// Workspace reports the folder the app was opened on.
func (s *Service) Workspace() Workspace {
	s.mu.Lock()
	defer s.mu.Unlock()

	// No root means the app was launched without a folder (from Finder).
	if s.root == "" {
		return Workspace{}
	}

	return Workspace{Root: s.root, Name: filepath.Base(s.root)}
}

// ListTours discovers the workspace's tours and summarizes each for the picker.
func (s *Service) ListTours(ctx context.Context) ([]TourSummary, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	// Listing is what a reload does first, so this is where a fetch or a new
	// commit since the last look gets noticed.
	s.svc().Walker.Reset()

	paths, err := s.discover(ctx)
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
			return nil, observability.PrepareAndLogError(sumErr, s.logger.WithValue(tourKey, path), span, "summarizing tour")
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
	summary := TourSummary{Path: path, DisplayPath: path, Tags: []string{}, Repos: []string{}}

	if rel, err := filepath.Rel(s.Workspace().Root, path); err == nil && filepath.IsLocal(rel) {
		summary.DisplayPath = rel
	}

	diags, err := s.svc().Validator.Tour(ctx, path)
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

	t, _, err := s.svc().Store.Load(ctx, path)
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

	for i := range t.Repos {
		summary.Repos = append(summary.Repos, t.Repos[i].Name)
	}

	return summary, nil
}

// OpenTour loads a tour by the path ListTours gave for it.
func (s *Service) OpenTour(ctx context.Context, path string) (*TourView, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue(tourKey, path)

	if err := s.checkKnown(ctx, path); err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "opening tour")
	}

	t, diags, err := s.svc().Store.Load(ctx, path)
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

	s.mu.Lock()
	workingTree := s.workingTree
	s.mu.Unlock()

	view := &TourView{
		Tour: t, Diagnostics: diags, Steps: make([]StepSummary, len(t.Steps)), Vars: s.vars(t),
		Repos: make([]walk.RepoState, 0, len(t.Repos)), WorkingTree: workingTree,
	}

	if m := s.svc().Walker.Map(ctx, t); m != nil {
		view.Map, view.MapProblem = m.Source, m.Detail
	}

	for i := range t.Repos {
		state, repoErr := s.svc().Walker.Repo(ctx, t, t.Repos[i].Name)
		if repoErr != nil {
			return nil, observability.PrepareAndLogError(repoErr, logger, span, "inspecting repo %s", t.Repos[i].Name)
		}

		view.Repos = append(view.Repos, *state)
	}

	if view.Diagnostics == nil {
		view.Diagnostics = []diag.Diagnostic{}
	}

	steps := make([]*walk.Step, len(t.Steps))

	for i := range t.Steps {
		step, stepErr := s.svc().Walker.Step(ctx, t, i)
		if stepErr != nil {
			return nil, observability.PrepareAndLogError(stepErr, logger, span, "resolving step %d", i)
		}

		steps[i] = step
		view.Steps[i] = StepSummary{Kind: step.Kind, Status: statusOf(step)}
	}

	problem := reviewProblem(t, steps, view.Repos, workingTree)

	name := ""
	if git := s.svc().Git; git != nil && t.IsDocent() {
		// Asked where the tour file lives, since that is the repo the review
		// will be committed to.
		name = git.UserName(ctx, filepath.Dir(t.Path))
	}

	s.mu.Lock()
	s.open[path] = t
	view.Review = s.track(t, problem, name)
	s.mu.Unlock()

	logger.WithValue("steps", len(t.Steps)).Info("opened tour")

	return view, nil
}

// Step resolves one step of a tour OpenTour has loaded. It answers from that
// load, so the indices the frontend holds stay meaningful until it reopens.
func (s *Service) Step(ctx context.Context, path string, index int) (*StepView, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, stepKey: index})

	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "resolving step")
	}

	step, err := s.svc().Walker.Step(ctx, t, index)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "resolving step")
	}

	view := &StepView{
		Index:        index,
		Kind:         step.Kind,
		Path:         step.Path,
		Readme:       step.Readme,
		Body:         step.Body,
		BodyProblem:  step.BodyDetail,
		Repo:         step.Repo,
		CloneCommand: shellJoin(step.CloneCommand),
		Entries:      step.Entries,
		Links:        step.Links,
		Location: anchor.Location{
			Status:  statusOf(step),
			Problem: step.Problem,
			Detail:  step.Detail,
		},
	}

	if view.Entries == nil {
		view.Entries = []gitsrc.Entry{}
	}

	if view.Links == nil {
		view.Links = []links.Card{}
	}

	view.Ref = step.Ref
	view.Permalink = s.permalink(ctx, t, step)

	if step.Anchor != nil {
		view.Location = step.Anchor.Location
		view.Then = step.Anchor.Then
		view.Removed = step.Anchor.Removed
		view.Content, view.Unavailable = showable(step.Anchor.Content)
	}

	view.Anchors = make([]AnchorView, 0, len(step.Extras))

	for i := range step.Extras {
		code := step.Extras[i].Code
		extra := AnchorView{
			Label:        step.Extras[i].Label,
			Repo:         code.Repo,
			Path:         code.Path,
			Ref:          code.Ref,
			Location:     code.Anchor.Location,
			Then:         code.Anchor.Then,
			Removed:      code.Anchor.Removed,
			Permalink:    s.permalink(ctx, t, code),
			CloneCommand: shellJoin(code.CloneCommand),
		}
		extra.Content, extra.Unavailable = showable(code.Anchor.Content)
		view.Anchors = append(view.Anchors, extra)
	}

	return view, nil
}

// showable turns a file's bytes into text for the code pane, or says why
// they can't be shown.
func showable(content []byte) (text, unavailable string) {
	switch {
	case content == nil:
		return "", ""
	case len(content) > maxFileSize:
		return "", fmt.Sprintf("This file is %d MB, too large to show.", len(content)>>20)
	case bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content):
		return "", "This is a binary file."
	default:
		return string(content), ""
	}
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

	paths, err := s.discover(ctx)
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
	return stamp.StatusOf(step)
}

// vars lists a tour's link variables with their effective values.
func (s *Service) vars(t *tour.Tour) []VarView {
	views := make([]VarView, 0, len(t.Vars))

	for i := range t.Vars {
		v := &t.Vars[i]
		value, source, _ := s.svc().Vars.Lookup(v.Name, t.Vars)

		choices := v.Choices
		if choices == nil {
			choices = []string{}
		}

		views = append(views, VarView{
			Name:        v.Name,
			Description: v.Description,
			Value:       value,
			Source:      source,
			Choices:     choices,
			Locked:      source == links.SourceFlag,
		})
	}

	return views
}

// SetVar records the value picked in the app for one of an open tour's link
// variables, and returns the variables as they now stand. An empty value
// clears the pick.
func (s *Service) SetVar(ctx context.Context, path, name, value string) ([]VarView, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, "var": name})

	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "setting variable")
	}

	var defined *tour.Var

	for i := range t.Vars {
		if t.Vars[i].Name == name {
			defined = &t.Vars[i]
		}
	}

	switch {
	case defined == nil:
		return nil, observability.PrepareAndLogError(fmt.Errorf("variable %q: %w", name, ErrBadVariable), logger, span, "setting variable")
	case value != "" && len(defined.Choices) > 0 && !slices.Contains(defined.Choices, value):
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q for %q: %w", value, name, ErrBadVariable), logger, span, "setting variable")
	}

	s.svc().Vars.Pick(name, value)

	return s.vars(t), nil
}

// Editors lists the editors installed right now.
func (s *Service) Editors(ctx context.Context) []editor.Editor {
	if found := s.launcher.Detect(ctx); found != nil {
		return found
	}

	return []editor.Editor{}
}

// OpenInEditor opens a file step of an open tour in an editor, at the line its
// anchor resolved to. extra picks one of the step's extra anchors, or -1 for
// its own file. editorID picks one from Editors; empty means whatever the
// reader's rules and default say. The frontend names a step, never a path.
func (s *Service) OpenInEditor(ctx context.Context, path string, index, extra int, editorID string) (*editor.Result, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, stepKey: index})

	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "opening in editor")
	}

	step, err := s.svc().Walker.Step(ctx, t, index)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "resolving step")
	}

	// extra picks one of the step's extra anchors; -1 means the step's own file.
	if extra >= 0 {
		if extra >= len(step.Extras) {
			return nil, observability.PrepareAndLogError(ErrNotAFile, logger, span, "opening in editor")
		}

		step = step.Extras[extra].Code
	}

	if step.Kind != walk.KindFile || step.Anchor == nil || step.Anchor.Content == nil {
		return nil, observability.PrepareAndLogError(ErrNotAFile, logger, span, "opening in editor")
	}

	abs, _, err := gitsrc.Confine(step.Root, step.Path)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "locating file")
	}

	location := step.Anchor.Location.Range

	result, err := s.launcher.Open(ctx, &editor.Target{
		Path: abs, Line: location.StartLine, Col: location.StartCol, Repo: step.Repo, RelPath: step.Path,
	}, editorID)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "opening in editor")
	}

	return result, nil
}

// shellJoin renders an argv as one line that is safe to paste into a shell.
func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))

	for i, arg := range argv {
		if arg != "" && !strings.ContainsFunc(arg, func(r rune) bool {
			return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@%+=:,./-_~", r)
		}) {
			quoted[i] = arg
			continue
		}

		quoted[i] = "'" + strings.ReplaceAll(arg, "'", `'\''`) + "'"
	}

	return strings.Join(quoted, " ")
}

// permalink builds the GitHub link for a step's anchored lines, at the commit
// being read (HEAD when reading the working tree).
func (s *Service) permalink(ctx context.Context, t *tour.Tour, step *walk.Step) string {
	if s.svc().Git == nil || (step.Kind != walk.KindFile && step.Kind != walk.KindDirectory) || step.Path == "" {
		return ""
	}

	if step.Anchor != nil && step.Anchor.Content == nil {
		return ""
	}

	_, prefix, err := s.svc().Git.Locate(ctx, step.Root)
	if err != nil {
		return ""
	}

	remote := s.svc().Git.Origin(ctx, step.Root)

	for i := range t.Repos {
		if t.Repos[i].Name == step.Repo {
			remote = t.Repos[i].Remote
		}
	}

	sha := ""
	if step.Ref != nil {
		sha = step.Ref.Commit
	}

	if sha == "" {
		if sha, err = s.svc().Git.Commit(ctx, step.Root, "HEAD"); err != nil {
			return ""
		}
	}

	start, end := 0, 0
	if step.Anchor != nil {
		start, end = step.Anchor.Location.Range.StartLine, step.Anchor.Location.Range.EndLine
	}

	return links.Permalink(remote, sha, prefix+step.Path, start, end)
}

// SetWorkingTree switches between reading code at each repo's ref and reading
// it as it is on disk, uncommitted changes included.
func (s *Service) SetWorkingTree(on bool) {
	s.mu.Lock()
	s.workingTree = on
	// What was read one way wasn't read the other.
	s.progress = map[string]*progress{}
	s.mu.Unlock()

	s.svc().Walker.SetWorkingTree(on)
	s.logger.WithValue("working_tree", on).Info("switched what is read")
}

// repoState finds one of an open tour's repos.
func (s *Service) repoState(ctx context.Context, path, name string) (*walk.RepoState, error) {
	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%q is not open: %w", path, ErrUnknownTour)
	}

	state, err := s.svc().Walker.Repo(ctx, t, name)
	if err != nil {
		return nil, err
	}

	return state, nil
}

// Fetch runs `git fetch` in one of an open tour's repos. The frontend calls
// it only after the reader has said yes: it is one of the two things Docent
// ever does to a repo.
func (s *Service) Fetch(ctx context.Context, path, repo string) error {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, "repo": repo})

	state, err := s.repoState(ctx, path, repo)
	if err != nil {
		return observability.PrepareAndLogError(err, logger, span, "fetching")
	}

	if !state.Found {
		return observability.PrepareAndLogError(fmt.Errorf("repo %q: %w", repo, walk.ErrNoSuchRepo), logger, span, "fetching")
	}

	if err = s.svc().Git.Fetch(ctx, state.Root); err != nil {
		return observability.PrepareAndLogError(err, logger, span, "fetching")
	}

	s.svc().Walker.Reset()
	logger.Info("fetched repo")

	return nil
}

// Clone clones one of an open tour's missing repos to where the clone command
// shown to the reader says. Like Fetch, it runs only on the reader's say-so.
func (s *Service) Clone(ctx context.Context, path, repo string) error {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, "repo": repo})

	state, err := s.repoState(ctx, path, repo)
	if err != nil {
		return observability.PrepareAndLogError(err, logger, span, "cloning")
	}

	// The remote comes from the tour, which is untrusted, and git will run a
	// program for some kinds of "remote". Only plain network URLs get through.
	if state.Found || len(state.CloneCommand) != 4 || !cloneable(state.Remote) {
		return observability.PrepareAndLogError(fmt.Errorf("repo %q: %w", repo, ErrNotCloneable), logger, span, "cloning")
	}

	if err = s.svc().Git.Clone(ctx, state.Remote, state.CloneCommand[3]); err != nil {
		return observability.PrepareAndLogError(err, logger, span, "cloning")
	}

	s.svc().Walker.Reset()
	logger.WithValue("dest", state.CloneCommand[3]).Info("cloned repo")

	return nil
}

// cloneable reports whether a remote is an https or ssh git URL, in URL or
// scp-like form, and nothing else: no local paths, no transport helpers.
func cloneable(remote string) bool {
	if strings.ContainsAny(remote, " \t\n") {
		return false
	}

	if strings.HasPrefix(remote, "https://") || strings.HasPrefix(remote, "ssh://") {
		return true
	}

	user, rest, ok := strings.Cut(remote, "@")
	if !ok {
		return false
	}

	host, repoPath, ok := strings.Cut(rest, ":")

	return ok && isUser(user) && isHost(host) && repoPath != "" && !strings.HasPrefix(repoPath, "-")
}

func isUser(s string) bool {
	return s != "" && !strings.ContainsFunc(s, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._", r)
	})
}

func isHost(s string) bool {
	return s != "" && !strings.HasPrefix(s, "-") && !strings.ContainsFunc(s, func(r rune) bool {
		return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-", r)
	})
}

// ExpandLinks turns the links written in a docent-links fence into cards,
// filling in variables the same way a step's own `links` are. The frontend
// hands over the fence body as written; nothing in it is trusted.
func (s *Service) ExpandLinks(ctx context.Context, path, body string) ([]links.Card, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue(tourKey, path)

	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "expanding links")
	}

	fenced, err := validate.FencedLinks(body)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "reading links")
	}

	cards := make([]links.Card, 0, len(fenced))
	for i := range fenced {
		cards = append(cards, s.svc().Vars.Expand(&fenced[i], t.Vars))
	}

	return cards, nil
}

// browseRoot finds where a step's code (or one of its extra anchors') is read
// from, for the file-tree drawer.
func (s *Service) browseRoot(ctx context.Context, path string, index, extra int) (*walk.Step, error) {
	s.mu.Lock()
	t, ok := s.open[path]
	s.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("%q is not open: %w", path, ErrUnknownTour)
	}

	step, err := s.svc().Walker.Step(ctx, t, index)
	if err != nil {
		return nil, err
	}

	if extra >= 0 {
		if extra >= len(step.Extras) {
			return nil, ErrNotAFile
		}

		step = step.Extras[extra].Code
	}

	// A step whose repo or ref is missing has nothing to browse.
	if step.Ref == nil || (step.Kind != walk.KindFile && step.Kind != walk.KindDirectory) {
		return nil, ErrNotAFile
	}

	return step, nil
}

// Browse lists a directory of the repo a step's code is in, at the ref the
// step is read at. dir is relative to that repo's root; "" is the root.
func (s *Service) Browse(ctx context.Context, path string, index, extra int, dir string) ([]gitsrc.Entry, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, stepKey: index, "dir": dir})

	step, err := s.browseRoot(ctx, path, index, extra)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "browsing")
	}

	entries, err := s.svc().Source.ReadDir(ctx, step.Root, step.Ref.Commit, dir)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "listing directory")
	}

	if entries == nil {
		entries = []gitsrc.Entry{}
	}

	return entries, nil
}

// BrowseFile reads a file of the repo a step's code is in, at the ref the
// step is read at.
func (s *Service) BrowseFile(ctx context.Context, path string, index, extra int, file string) (*FileView, error) {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, stepKey: index, "file": file})

	step, err := s.browseRoot(ctx, path, index, extra)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "browsing")
	}

	content, err := s.svc().Source.ReadFile(ctx, step.Root, step.Ref.Commit, file)
	if err != nil {
		return nil, observability.PrepareAndLogError(err, logger, span, "reading file")
	}

	view := &FileView{Path: file}
	view.Content, view.Unavailable = showable(content)

	return view, nil
}
