package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/orderedjson"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"

	"github.com/spf13/cobra"
)

// Defaults for how far behind its refs a tour may fall before `docent stale`
// lists it.
const (
	defaultMaxCommits = 50
	defaultMaxDays    = 30
)

type (
	// staleRepo is one repo's distance from the tour's stamp.
	staleRepo struct {
		_      struct{} `json:"-"`
		Name   string   `json:"name"`
		Stamp  string   `json:"stamp"`
		Tip    string   `json:"tip"`
		Ref    string   `json:"ref"`
		Behind int      `json:"behind"`
		Days   int      `json:"days"`
	}

	// staleAnchor is one anchored file of a stale tour, with what changed in
	// it since the stamp.
	staleAnchor struct {
		_      struct{}      `json:"-"`
		Step   string        `json:"step"`
		Repo   string        `json:"repo"`
		File   string        `json:"file"`
		Status anchor.Status `json:"status"`
		Detail string        `json:"detail"`
		// Diff is the unified diff of the file from the stamp to the tip, or
		// "" when it is unchanged or there is no stamp to diff from.
		Diff string `json:"diff"`
	}

	// staleTour is the shape of one entry of `docent stale --json`: the brief
	// for refreshing a tour. Like the other JSON outputs it only grows.
	staleTour struct {
		_       struct{}      `json:"-"`
		File    string        `json:"file"`
		Title   string        `json:"title"`
		Reasons []string      `json:"reasons"`
		Repos   []staleRepo   `json:"repos"`
		Anchors []staleAnchor `json:"anchors"`
	}

	staleReport struct {
		_     struct{}    `json:"-"`
		Tours []staleTour `json:"tours"`
	}
)

// loadTour reads the tour at arg and the services for its root.
func (a *application) loadTour(ctx context.Context, arg string) (*core.Core, *tour.Tour, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving %q: %w", arg, err)
	}

	services, err := a.coreFor(tour.RootFor(abs), nil)
	if err != nil {
		return nil, nil, err
	}

	t, diags, err := services.Store.Load(ctx, abs)
	if err != nil {
		services.Close()
		return nil, nil, fmt.Errorf("loading %q: %w", arg, err)
	}

	if t == nil {
		services.Close()

		message := "it is not a tour"
		if len(diags) > 0 {
			message = diags[0].Message
		}

		return nil, nil, fmt.Errorf("%s can't be read: %s", arg, message)
	}

	return services, t, nil
}

func (a *application) newFmtCommand() *cobra.Command {
	var check bool

	cmd := &cobra.Command{
		Use:   "fmt [paths...]",
		Short: "Rewrite tours in the canonical layout. Changes no values.",
		Long: "Fmt re-indents tours: two spaces, key order kept, small objects on one line.\n" +
			"A path is a .tour file or a folder to search (default: the current folder).",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.format(cmd.Context(), args, check)
		},
	}

	cmd.Flags().BoolVar(&check, "check", false, "list files that would change and exit 1, without writing")

	return cmd
}

func (a *application) format(ctx context.Context, args []string, check bool) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	changed := 0

	for _, arg := range args {
		services, paths, err := a.tourPaths(ctx, arg)
		if err != nil {
			return err
		}

		services.Close()

		for _, path := range paths {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return fmt.Errorf("reading %s: %w", path, readErr)
			}

			raw, parseErr := orderedjson.Parse(data)
			if parseErr != nil {
				return fmt.Errorf("%s is not valid JSON: %w", path, parseErr)
			}

			formatted, fmtErr := raw.Format()
			if fmtErr != nil {
				return fmt.Errorf("formatting %s: %w", path, fmtErr)
			}

			if bytes.Equal(formatted, data) {
				continue
			}

			changed++

			fmt.Fprintln(a.streams.Stdout, path) //nolint:errcheck // nothing to do if stdout is gone

			if !check {
				if err = tour.Write(path, raw); err != nil {
					return err
				}
			}
		}
	}

	if check && changed > 0 {
		return &exitError{code: ExitErrors}
	}

	return nil
}

func (a *application) newStampCommand() *cobra.Command {
	var hashes, force bool

	cmd := &cobra.Command{
		Use:   "stamp <tour>",
		Short: "Record the commits a tour is correct against.",
		Long: "Stamp moves each repo's `stamp` to the commit its `ref` resolves to now, which\n" +
			"says: the tour is right as of here. Docent measures drift from the stamp.\n\n" +
			"It refuses while any anchor has drifted, because stamping would hide that.\n" +
			"Fix the anchors first (`docent stale` shows them).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.stamp(cmd.Context(), args[0], hashes, force, nil)
		},
	}

	cmd.Flags().BoolVar(&hashes, "hashes", false, "also record each anchor's contentHash at the new stamp")
	cmd.Flags().BoolVar(&force, "force", false, "stamp even though anchors have drifted")

	return cmd
}

func (a *application) newReviewCommand() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "review <tour> --name <name>",
		Short: "Vouch for a tour you have walked: adds a reviewedBy entry and stamps it.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(name) == "" {
				return errors.New("--name is required: who is vouching for this tour?")
			}

			return a.stamp(cmd.Context(), args[0], true, false, &tour.Review{Name: strings.TrimSpace(name), At: time.Now().UTC().Format(time.RFC3339)})
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "the reviewer's name")

	return cmd
}

// stamp moves a tour's stamps to its refs' current commits, optionally
// recording anchor hashes and a review.
func (a *application) stamp(ctx context.Context, arg string, hashes, force bool, review *tour.Review) error {
	services, t, err := a.loadTour(ctx, arg)
	if err != nil {
		return err
	}
	defer services.Close()

	if !t.IsDocent() {
		return fmt.Errorf("%s is a plain CodeTour file: stamps need \"docent\": 1 and `repos`", arg)
	}

	steps := make([]*walk.Step, len(t.Steps))

	var drifted []string

	for i := range t.Steps {
		if steps[i], err = services.Walker.Step(ctx, t, i); err != nil {
			return fmt.Errorf("resolving step %s: %w", t.StepLabel(i), err)
		}

		if status := statusOf(steps[i]); status != anchor.StatusOK {
			drifted = append(drifted, fmt.Sprintf("  %-10s %s", status, t.StepLabel(i)))
		}

		for j := range steps[i].Extras {
			if status := statusOf(steps[i].Extras[j].Code); status != anchor.StatusOK {
				drifted = append(drifted, fmt.Sprintf("  %-10s %s, anchor %q", status, t.StepLabel(i), steps[i].Extras[j].Label))
			}
		}
	}

	if len(drifted) > 0 && !force {
		return fmt.Errorf("%d of this tour's anchors don't resolve cleanly, and stamping now would hide that:\n%s\nfix them first, or pass --force",
			len(drifted), strings.Join(drifted, "\n"))
	}

	var out strings.Builder

	commits := map[string]string{}

	for i := range t.Repos {
		repo := &t.Repos[i]

		state, repoErr := services.Walker.Repo(ctx, t, repo.Name)
		if repoErr != nil {
			return repoErr
		}

		switch {
		case !state.Found:
			return fmt.Errorf("repo %q isn't checked out here, so it can't be stamped", repo.Name)
		case state.Ref == nil || state.Ref.Commit == "":
			return fmt.Errorf("repo %q has no commit to stamp: give it a `ref`, and make sure %s is a git repository", repo.Name, state.Root)
		}

		commits[repo.Name] = state.Ref.Commit
		tour.SetStamp(t.Raw, repo.Name, state.Ref.Commit)
		fmt.Fprintf(&out, "%-12s %s -> %s\n", repo.Name, orDash(shortSHA(repo.Stamp)), shortSHA(state.Ref.Commit))
	}

	for i := range t.Steps {
		hash := ""

		// A hash describes the code at its stamp, so moving the stamp without
		// recomputing it leaves a hash that describes nothing: it is cleared.
		if res := steps[i].Anchor; hashes && res != nil && res.Content != nil {
			hash = anchor.Hash(anchor.SplitLines(res.Content), res.Location.Range)
		}

		tour.SetHash(t.Raw, i, hash)

		for j := range steps[i].Extras {
			extraHash := ""
			if res := steps[i].Extras[j].Code.Anchor; hashes && res != nil && res.Content != nil {
				extraHash = anchor.Hash(anchor.SplitLines(res.Content), res.Location.Range)
			}

			tour.SetAnchorHash(t.Raw, i, j, extraHash)
		}
	}

	if review != nil {
		if len(commits) == 1 {
			for _, sha := range commits {
				review.Ref = sha
			}
		}

		tour.AppendReview(t.Raw, review)
		fmt.Fprintf(&out, "reviewed by %s\n", review.Name)
	}

	if err = tour.Write(t.Path, t.Raw); err != nil {
		return err
	}

	_, err = io.WriteString(a.streams.Stdout, out.String())

	return err
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

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}

	return sha
}

func orDash(s string) string {
	if s == "" {
		return "(none)"
	}

	return s
}

func (a *application) newStaleCommand() *cobra.Command {
	var (
		asJSON     bool
		maxCommits int
		maxDays    int
	)

	cmd := &cobra.Command{
		Use:   "stale [paths...]",
		Short: "List tours that have fallen behind the code, with what changed.",
		Long: "Stale lists tours whose stamp is far behind its tracked ref, or that have an\n" +
			"anchor that no longer resolves cleanly. For each it prints the diff, from the\n" +
			"stamp to now, of every anchored file. That output is the brief for refreshing\n" +
			"the tour.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.stale(cmd.Context(), args, asJSON, maxCommits, maxDays)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")
	cmd.Flags().IntVar(&maxCommits, "max-commits", defaultMaxCommits, "how many commits behind its ref a stamp may be")
	cmd.Flags().IntVar(&maxDays, "max-days", defaultMaxDays, "how many days old a stamp may be while its ref has moved")

	return cmd
}

func (a *application) stale(ctx context.Context, args []string, asJSON bool, maxCommits, maxDays int) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	report := staleReport{Tours: []staleTour{}}

	for _, arg := range args {
		services, paths, err := a.tourPaths(ctx, arg)
		if err != nil {
			return err
		}

		for _, path := range paths {
			entry, staleErr := staleness(ctx, services, path, maxCommits, maxDays)
			if staleErr != nil {
				services.Close()
				return staleErr
			}

			if entry != nil {
				report.Tours = append(report.Tours, *entry)
			}
		}

		services.Close()
	}

	if asJSON {
		return writeJSON(a.streams.Stdout, report)
	}

	var b strings.Builder

	for i := range report.Tours {
		printStale(&b, &report.Tours[i])
	}

	fmt.Fprintf(&b, "%s\n", plural(len(report.Tours), "stale tour"))

	_, err := io.WriteString(a.streams.Stdout, b.String())

	return err
}

// staleness describes why a tour is stale, or returns nil when it isn't.
func staleness(ctx context.Context, services *core.Core, path string, maxCommits, maxDays int) (*staleTour, error) {
	t, _, err := services.Store.Load(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("loading %s: %w", path, err)
	}

	if t == nil {
		return &staleTour{File: path, Reasons: []string{"the file can't be read as a tour"}, Repos: []staleRepo{}, Anchors: []staleAnchor{}}, nil
	}

	entry := &staleTour{File: path, Title: t.Title, Reasons: []string{}, Repos: []staleRepo{}, Anchors: []staleAnchor{}}
	states := map[string]*walk.RepoState{}

	for i := range t.Repos {
		state, repoErr := services.Walker.Repo(ctx, t, t.Repos[i].Name)
		if repoErr != nil {
			return nil, repoErr
		}

		states[state.Name] = state

		if !state.Found || state.Ref == nil || state.Stamp == "" {
			continue
		}

		days := 0
		if state.Behind > 0 {
			days = int(time.Since(state.StampedAt).Hours() / 24)
		}

		entry.Repos = append(entry.Repos, staleRepo{
			Name: state.Name, Stamp: state.Stamp, Tip: state.Ref.Commit, Ref: state.Ref.Name, Behind: state.Behind, Days: days,
		})

		switch {
		case state.Behind > maxCommits:
			entry.Reasons = append(entry.Reasons, fmt.Sprintf("%s is %d commits behind %s", state.Name, state.Behind, state.Ref.Name))
		case days > maxDays:
			entry.Reasons = append(entry.Reasons, fmt.Sprintf("%s was stamped %d days ago and %s has moved since", state.Name, days, state.Ref.Name))
		}
	}

	notOK := 0

	for i := range t.Steps {
		step, stepErr := services.Walker.Step(ctx, t, i)
		if stepErr != nil {
			return nil, fmt.Errorf("resolving step %s of %s: %w", t.StepLabel(i), path, stepErr)
		}

		if step.Kind != walk.KindFile || step.Anchor == nil {
			if step.Problem != "" {
				notOK++

				entry.Anchors = append(entry.Anchors, staleAnchor{Step: t.StepLabel(i), Repo: step.Repo, File: step.Path, Status: anchor.StatusMissing, Detail: step.Detail})
			}

			continue
		}

		loc := step.Anchor.Location
		item := staleAnchor{Step: t.StepLabel(i), Repo: step.Repo, File: t.Steps[i].File, Status: loc.Status, Detail: loc.Detail}

		if loc.Status != anchor.StatusOK {
			notOK++
		}

		if state := states[step.Repo]; state != nil && state.Stamp != "" && state.Ref != nil && state.Ref.Commit != "" && state.Stamp != state.Ref.Commit && services.Git != nil {
			// A file that is gone, or was never there, has no diff to show.
			if diff, diffErr := services.Git.DiffText(ctx, state.Root, state.Stamp, state.Ref.Commit, t.Steps[i].File, step.Anchor.File); diffErr == nil {
				item.Diff = diff
			}
		}

		entry.Anchors = append(entry.Anchors, item)
	}

	if notOK > 0 {
		entry.Reasons = append(entry.Reasons, fmt.Sprintf("%s resolve cleanly", pluralVerb(notOK, "anchor", "doesn't", "don't")))
	}

	if len(entry.Reasons) == 0 {
		return nil, nil //nolint:nilnil // not stale is an answer
	}

	return entry, nil
}

func pluralVerb(n int, noun, singular, pluralForm string) string {
	if n == 1 {
		return "1 " + noun + " " + singular
	}

	return fmt.Sprintf("%d %ss %s", n, noun, pluralForm)
}

func printStale(b *strings.Builder, entry *staleTour) {
	fmt.Fprintf(b, "%s\n", entry.File)

	for _, reason := range entry.Reasons {
		fmt.Fprintf(b, "  %s\n", reason)
	}

	for i := range entry.Repos {
		r := &entry.Repos[i]
		fmt.Fprintf(b, "  repo %s: stamped at %s, %s is at %s (%d commits)\n", r.Name, shortSHA(r.Stamp), r.Ref, shortSHA(r.Tip), r.Behind)
	}

	for i := range entry.Anchors {
		item := &entry.Anchors[i]
		if item.Status == anchor.StatusOK && item.Diff == "" {
			continue
		}

		where := item.File
		if item.Repo != "" {
			where = item.Repo + " › " + where
		}

		fmt.Fprintf(b, "\n  step %s  %s  %s\n", item.Step, item.Status, where)

		if item.Detail != "" {
			fmt.Fprintf(b, "  %s\n", item.Detail)
		}

		if item.Diff != "" {
			for line := range strings.SplitSeq(item.Diff, "\n") {
				fmt.Fprintf(b, "    %s\n", line)
			}
		}
	}

	b.WriteByte('\n')
}
