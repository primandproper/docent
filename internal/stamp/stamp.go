// Package stamp records the commits a tour is correct against, and who vouched
// for it. `docent stamp`, `docent review` and the app's Mark reviewed button
// all go through Apply, so they can't disagree about what a stamp means.
package stamp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"
)

// ErrPlainTour is returned for a CodeTour file, which has nowhere to keep a stamp.
var ErrPlainTour = errors.New("a plain CodeTour file can't be stamped: stamps need \"docent\": 1 and `repos`")

type (
	// Options say what Apply records besides the stamps.
	Options struct {
		_ struct{}
		// Review, when set, is appended to `reviewedBy`. Apply fills in its Ref
		// when the tour has exactly one repo.
		Review *tour.Review
		// Hashes also records each anchor's contentHash at the new stamp.
		Hashes bool
		// Force stamps even though anchors have drifted.
		Force bool
	}

	// Move is one repo's stamp before and after.
	Move struct {
		_    struct{}
		Name string
		From string
		To   string
	}

	// Result is what Apply changed.
	Result struct {
		_     struct{}
		Moves []Move
	}

	// DriftError is returned when anchors don't resolve cleanly, which a new
	// stamp would hide.
	DriftError struct {
		_ struct{}
		// Anchors names each one, as "status  step" lines.
		Anchors []string
	}
)

func (e *DriftError) Error() string {
	return fmt.Sprintf("%d of this tour's anchors don't resolve cleanly, and stamping now would hide that:\n%s",
		len(e.Anchors), strings.Join(e.Anchors, "\n"))
}

// StatusOf is a resolved step's anchor status, counting a step that couldn't
// be resolved at all as missing.
func StatusOf(step *walk.Step) anchor.Status {
	switch {
	case step.Anchor != nil:
		return step.Anchor.Location.Status
	case step.Problem != "":
		return anchor.StatusMissing
	default:
		return anchor.StatusOK
	}
}

// Drifted lists the anchors of a tour that don't resolve cleanly.
func Drifted(t *tour.Tour, steps []*walk.Step) []string {
	var drifted []string

	for i := range steps {
		if status := StatusOf(steps[i]); status != anchor.StatusOK {
			drifted = append(drifted, fmt.Sprintf("  %-10s %s", status, t.StepLabel(i)))
		}

		for j := range steps[i].Extras {
			if status := StatusOf(steps[i].Extras[j].Code); status != anchor.StatusOK {
				drifted = append(drifted, fmt.Sprintf("  %-10s %s, anchor %q", status, t.StepLabel(i), steps[i].Extras[j].Label))
			}
		}
	}

	return drifted
}

// Apply moves each of a tour's stamps to the commit its ref resolves to now,
// in t.Raw. It writes nothing: the caller saves the document with tour.Write
// once Apply has succeeded.
func Apply(ctx context.Context, walker walk.Walker, t *tour.Tour, opts *Options) (*Result, error) {
	if !t.IsDocent() {
		return nil, ErrPlainTour
	}

	steps := make([]*walk.Step, len(t.Steps))

	for i := range t.Steps {
		step, err := walker.Step(ctx, t, i)
		if err != nil {
			return nil, fmt.Errorf("resolving step %s: %w", t.StepLabel(i), err)
		}

		steps[i] = step
	}

	if drifted := Drifted(t, steps); len(drifted) > 0 && !opts.Force {
		return nil, &DriftError{Anchors: drifted}
	}

	result := &Result{}

	for i := range t.Repos {
		repo := &t.Repos[i]

		state, err := walker.Repo(ctx, t, repo.Name)
		if err != nil {
			return nil, err
		}

		switch {
		case !state.Found:
			return nil, fmt.Errorf("repo %q isn't checked out here, so it can't be stamped", repo.Name)
		case state.Ref == nil || state.Ref.Commit == "":
			return nil, fmt.Errorf("repo %q has no commit to stamp: give it a `ref`, and make sure %s is a git repository", repo.Name, state.Root)
		}

		result.Moves = append(result.Moves, Move{Name: repo.Name, From: repo.Stamp, To: state.Ref.Commit})
	}

	for i := range result.Moves {
		tour.SetStamp(t.Raw, result.Moves[i].Name, result.Moves[i].To)
	}

	for i := range t.Steps {
		// A hash describes the code at its stamp, so moving the stamp without
		// recomputing it leaves a hash that describes nothing: it is cleared.
		tour.SetHash(t.Raw, i, hashOf(steps[i], opts.Hashes))

		for j := range steps[i].Extras {
			tour.SetAnchorHash(t.Raw, i, j, hashOf(steps[i].Extras[j].Code, opts.Hashes))
		}
	}

	if opts.Review != nil {
		if len(result.Moves) == 1 {
			opts.Review.Ref = result.Moves[0].To
		}

		tour.AppendReview(t.Raw, opts.Review)
	}

	return result, nil
}

func hashOf(step *walk.Step, hashes bool) string {
	if res := step.Anchor; hashes && res != nil && res.Content != nil {
		return anchor.Hash(anchor.SplitLines(res.Content), res.Location.Range)
	}

	return ""
}
