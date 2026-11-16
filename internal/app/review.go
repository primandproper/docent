package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/primandproper/docent/internal/stamp"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"

	"github.com/primandproper/primitives-go/v2/observability"
)

// maxNameLength is the longest reviewer's name a review is signed with.
const maxNameLength = 100

// fingerprint identifies a tour's text. Two loads of the same file agree; any
// edit to it disagrees.
func fingerprint(t *tour.Tour) string {
	formatted, err := t.Raw.Format()
	if err != nil {
		// A tour that can't be written back can't be reviewed either, and a
		// mark that never matches says exactly that.
		return ""
	}

	sum := sha256.Sum256(formatted)

	return hex.EncodeToString(sum[:])
}

// reviewProblem says what, other than unread steps, stops a tour being marked
// reviewed. These are the conditions stamp.Apply refuses on, found early so
// the reader hears about them before reading to the end.
func reviewProblem(t *tour.Tour, steps []*walk.Step, repos []walk.RepoState, workingTree bool) string {
	switch {
	case !t.IsDocent():
		return "A plain CodeTour file has nowhere to record a review."
	case len(t.Steps) == 0:
		return "This tour has no steps to review."
	case workingTree:
		return "Turn off Working tree: a review vouches for committed code."
	}

	for i := range repos {
		switch {
		case !repos[i].Found:
			return fmt.Sprintf("The %s repo isn't checked out here.", repos[i].Name)
		case repos[i].Ref == nil || repos[i].Ref.Commit == "":
			return fmt.Sprintf("The %s repo has no commit to review against.", repos[i].Name)
		}
	}

	switch drifted := len(stamp.Drifted(t, steps)); drifted {
	case 0:
		return ""
	case 1:
		return "1 anchor doesn't resolve cleanly. Fix it first."
	default:
		return fmt.Sprintf("%d anchors don't resolve cleanly. Fix them first.", drifted)
	}
}

// track starts, or carries on, the count of which steps of a tour have been
// read, and reports where it stands. The caller holds s.mu.
func (s *Service) track(t *tour.Tour, problem, name string) ReviewState {
	mark := fingerprint(t)

	p := s.progress[t.Path]
	if p == nil || p.mark != mark || len(p.read) != len(t.Steps) {
		p = &progress{mark: mark, read: make([]bool, len(t.Steps))}
		s.progress[t.Path] = p
	}

	p.problem, p.name = problem, name

	return p.state(t)
}

// state reports how far a tour is from being reviewable.
func (p *progress) state(t *tour.Tour) ReviewState {
	state := ReviewState{
		Available: t.IsDocent(),
		Name:      p.name,
		Read:      slices.Clone(p.read),
		Blocker:   p.problem,
	}

	if state.Blocker == "" {
		state.Blocker = unreadBlocker(p.read)
	}

	state.Ready = state.Available && state.Blocker == ""

	return state
}

// unreadBlocker names the steps still to read, or returns "".
func unreadBlocker(read []bool) string {
	var unread []string

	for i, done := range read {
		if !done {
			unread = append(unread, fmt.Sprint(i+1))
		}
	}

	const shown = 6

	switch {
	case len(unread) == 0:
		return ""
	case len(unread) == len(read):
		return "Read every step first."
	case len(unread) == 1:
		return fmt.Sprintf("Read step %s first.", unread[0])
	case len(unread) > shown:
		return fmt.Sprintf("Read steps %s and %d more first.", strings.Join(unread[:shown], ", "), len(unread)-shown)
	default:
		return fmt.Sprintf("Read steps %s and %s first.", strings.Join(unread[:len(unread)-1], ", "), unread[len(unread)-1])
	}
}

// MarkRead records that the reader has had a step of an open tour in front of
// them, and reports what a review still waits on. The frontend calls it once
// a step has been on screen for a moment, not as it is passed through.
func (s *Service) MarkRead(ctx context.Context, path string, index int) (*ReviewState, error) {
	_, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValues(map[string]any{tourKey: path, stepKey: index})

	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.open[path]
	p := s.progress[path]

	if !ok || p == nil {
		return nil, observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "marking a step read")
	}

	if index < 0 || index >= len(p.read) {
		return nil, observability.PrepareAndLogError(fmt.Errorf("step %d: %w", index, walk.ErrNoSuchStep), logger, span, "marking a step read")
	}

	p.read[index] = true
	state := p.state(t)

	return &state, nil
}

// reviewerName cleans up a name to sign a review with, or refuses it.
func reviewerName(name string) (string, error) {
	name = strings.TrimSpace(name)

	switch {
	case name == "":
		return "", fmt.Errorf("a review needs a name: %w", ErrBadName)
	case utf8.RuneCountInString(name) > maxNameLength:
		return "", fmt.Errorf("longer than %d characters: %w", maxNameLength, ErrBadName)
	case strings.ContainsFunc(name, unicode.IsControl):
		return "", fmt.Errorf("control characters: %w", ErrBadName)
	}

	return name, nil
}

// Review vouches for an open tour in the reader's name: it appends a
// `reviewedBy` entry and stamps the tour, as `docent review` does. It is the
// only edit the app ever makes to a tour, the frontend calls it only once the
// reader has confirmed, and it refuses unless every step of the tour, as it
// is on disk now, has been read in this session.
func (s *Service) Review(ctx context.Context, path, name string) error {
	ctx, span := s.tracer.StartSpan(ctx)
	defer span.End()

	logger := s.logger.WithValue(tourKey, path)

	name, err := reviewerName(name)
	if err != nil {
		return observability.PrepareAndLogError(err, logger, span, "reviewing")
	}

	s.mu.Lock()
	_, isOpen := s.open[path]
	workingTree := s.workingTree
	s.mu.Unlock()

	if !isOpen {
		return observability.PrepareAndLogError(fmt.Errorf("%q is not open: %w", path, ErrUnknownTour), logger, span, "reviewing")
	}

	if workingTree {
		return observability.PrepareAndLogError(fmt.Errorf("%w: turn off Working tree, a review vouches for committed code", ErrNotRead), logger, span, "reviewing")
	}

	// The file is read again, and what was read is checked against it: a tour
	// edited since it was walked hasn't been walked.
	t, _, err := s.svc().Store.Load(ctx, path)
	if err != nil {
		return observability.PrepareAndLogError(err, logger, span, "loading tour")
	}

	if t == nil {
		return observability.PrepareAndLogError(fmt.Errorf("%q: %w", path, ErrTourUnreadable), logger, span, "reviewing")
	}

	mark := fingerprint(t)

	s.mu.Lock()
	p := s.progress[path]
	read := p != nil && mark != "" && p.mark == mark && len(p.read) > 0 && !slices.Contains(p.read, false)
	s.mu.Unlock()

	if !read {
		return observability.PrepareAndLogError(ErrNotRead, logger, span, "reviewing")
	}

	review := &tour.Review{Name: name, At: time.Now().UTC().Format(time.RFC3339)}

	if _, err = stamp.Apply(ctx, s.svc().Walker, t, &stamp.Options{Hashes: true, Review: review}); err != nil {
		return observability.PrepareAndLogError(err, logger, span, "stamping")
	}

	if err = s.write(path, t.Raw); err != nil {
		return observability.PrepareAndLogError(err, logger, span, "saving the review")
	}

	logger.WithValue("reviewer", name).Info("marked tour reviewed")

	return nil
}
