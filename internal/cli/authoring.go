package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/guide"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"
	"github.com/primandproper/docent/schema"

	"github.com/spf13/cobra"
)

const (
	// resolveContext is how many lines are printed either side of an anchor.
	resolveContext = 2
	// resolveMaxLines caps how much of a long anchored range is printed.
	resolveMaxLines = 60
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type (
	// resolvedLine is one line of code in `docent resolve` output.
	resolvedLine struct {
		_        struct{} `json:"-"`
		Text     string   `json:"text"`
		Number   int      `json:"number"`
		Anchored bool     `json:"anchored"`
	}

	// resolvedStep is the shape of `docent resolve --json`, one per step. It
	// is a stable contract, like validate's.
	resolvedStep struct {
		_ struct{} `json:"-"`
		// Step is the step's id, or "#N".
		Step string    `json:"step"`
		Kind walk.Kind `json:"kind"`
		Repo string    `json:"repo"`
		// Root is the directory Path is relative to on this machine.
		Root    string        `json:"root"`
		Path    string        `json:"path"`
		Status  anchor.Status `json:"status"`
		Problem string        `json:"problem"`
		Detail  string        `json:"detail"`
		// Lines is the anchored code with a little context.
		Lines []resolvedLine `json:"lines"`
		Range anchor.Range   `json:"range"`
		// Number is the step's position, counting from one.
		Number int `json:"number"`
	}
)

func (a *application) newSchemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the tour JSON Schema.",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			_, err := a.streams.Stdout.Write(schema.TourV1)
			return err
		},
	}
}

func (a *application) newGuideCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "guide",
		Short: "Print the tour authoring guide, written for an agent to follow.",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			_, err := io.WriteString(a.streams.Stdout, guide.Markdown)
			return err
		},
	}
}

func (a *application) newNewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold .tours/<slug>.tour and a folder for its step bodies.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("finding the current folder: %w", err)
			}

			return a.scaffold(dir, args[0])
		},
	}
}

// scaffold writes a starter tour under dir/.tours. It never overwrites.
func (a *application) scaffold(dir, slug string) error {
	if !slugPattern.MatchString(slug) {
		return fmt.Errorf("%q is not a slug: use lowercase letters, digits and hyphens", slug)
	}

	tourPath := filepath.Join(dir, ".tours", slug+tour.Extension)
	bodyPath := filepath.Join(dir, ".tours", slug, "01-overview.md")

	for _, path := range []string{tourPath, bodyPath} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists", path)
		}
	}

	title := strings.ToUpper(slug[:1]) + strings.ReplaceAll(slug[1:], "-", " ")

	tourJSON := fmt.Sprintf(`{
  "$schema": %q,
  "docent": 1,
  "title": %q,
  "description": "",
  "steps": [
    {
      "id": "overview",
      "title": "Overview",
      "description": "What this tour covers.",
      "body": %q
    }
  ]
}
`, schema.TourV1ID, title, slug+"/01-overview.md")

	body := "Say what the reader will understand by the end of this tour, and who it is for.\n"

	if err := os.MkdirAll(filepath.Dir(bodyPath), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(bodyPath), err)
	}

	for path, content := range map[string]string{tourPath: tourJSON, bodyPath: body} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
	}

	_, err := fmt.Fprintf(a.streams.Stdout, "%s\n%s\n", tourPath, bodyPath)

	return err
}

func (a *application) newResolveCommand() *cobra.Command {
	var (
		stepRef string
		asJSON  bool
	)

	cmd := &cobra.Command{
		Use:   "resolve <tour>",
		Short: "Print the code a step's anchor points at, and how it resolved.",
		Long: "Resolve prints, for one step, the file and lines its anchor lands on, with line\n" +
			"numbers and the anchor's status. Without --step it prints one line per step.\n" +
			"This is how to check that an anchor points where you think.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.resolve(cmd.Context(), args[0], stepRef, asJSON)
		},
	}

	cmd.Flags().StringVar(&stepRef, "step", "", "the step to resolve: its id, or its number counting from 1")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print as JSON")

	return cmd
}

func (a *application) resolve(ctx context.Context, arg, stepRef string, asJSON bool) error {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", arg, err)
	}

	services, err := a.coreFor(tour.RootFor(abs), nil)
	if err != nil {
		return err
	}

	t, diags, err := services.Store.Load(ctx, abs)
	if err != nil {
		return fmt.Errorf("loading %q: %w", arg, err)
	}

	if t == nil {
		message := "it is not a tour"
		if len(diags) > 0 {
			message = diags[0].Message
		}

		return fmt.Errorf("%s can't be read: %s", arg, message)
	}

	indexes := make([]int, 0, len(t.Steps))

	if stepRef == "" {
		for i := range t.Steps {
			indexes = append(indexes, i)
		}
	} else {
		index, findErr := findStep(t, stepRef)
		if findErr != nil {
			return findErr
		}

		indexes = append(indexes, index)
	}

	resolved := make([]resolvedStep, 0, len(indexes))

	for _, index := range indexes {
		step, stepErr := services.Walker.Step(ctx, t, index)
		if stepErr != nil {
			return fmt.Errorf("resolving step %s: %w", t.StepLabel(index), stepErr)
		}

		resolved = append(resolved, describe(t, index, step, stepRef != ""))
	}

	if asJSON {
		return writeJSON(a.streams.Stdout, resolved)
	}

	var b strings.Builder

	for i := range resolved {
		if stepRef == "" {
			printSummary(&b, &resolved[i])
		} else {
			printDetail(&b, &resolved[i])
		}
	}

	_, err = io.WriteString(a.streams.Stdout, b.String())

	return err
}

// findStep locates a step by id, or by its number counting from one.
func findStep(t *tour.Tour, ref string) (int, error) {
	for i := range t.Steps {
		if t.Steps[i].ID != "" && t.Steps[i].ID == ref {
			return i, nil
		}
	}

	if n, err := strconv.Atoi(strings.TrimPrefix(ref, "#")); err == nil && n >= 1 && n <= len(t.Steps) {
		return n - 1, nil
	}

	return 0, fmt.Errorf("no step %q in %s: use a step id, or a number from 1 to %d", ref, t.Path, len(t.Steps))
}

func describe(t *tour.Tour, index int, step *walk.Step, withLines bool) resolvedStep {
	out := resolvedStep{
		Step:   t.StepLabel(index),
		Number: index + 1,
		Kind:   step.Kind,
		Repo:   step.Repo,
		Root:   step.Root,
		Path:   step.Path,
		Status: anchor.StatusOK,
		Lines:  []resolvedLine{},
	}

	switch {
	case step.Anchor != nil:
		loc := step.Anchor.Location
		out.Status, out.Problem, out.Detail, out.Range = loc.Status, string(loc.Problem), loc.Detail, loc.Range

		if withLines {
			out.Lines = excerpt(anchor.SplitLines(step.Anchor.Content), loc.Range)
		}
	case step.Problem != "":
		out.Status, out.Problem, out.Detail = anchor.StatusMissing, string(step.Problem), step.Detail
	}

	if step.BodyProblem != "" && out.Problem == "" {
		out.Problem, out.Detail = string(step.BodyProblem), step.BodyDetail
	}

	return out
}

// excerpt returns the anchored lines with a little context either side. A
// file with no anchor gives its first lines.
func excerpt(lines []string, r anchor.Range) []resolvedLine {
	if len(lines) == 0 {
		return []resolvedLine{}
	}

	from, to := 1, min(len(lines), 2*resolveContext+1)
	if r.StartLine > 0 {
		from = max(1, r.StartLine-resolveContext)
		to = min(len(lines), min(r.EndLine, r.StartLine+resolveMaxLines-1)+resolveContext)
	}

	out := make([]resolvedLine, 0, to-from+1)
	for n := from; n <= to; n++ {
		out = append(out, resolvedLine{Number: n, Text: lines[n-1], Anchored: r.StartLine > 0 && n >= r.StartLine && n <= r.EndLine})
	}

	return out
}

func where(r *resolvedStep) string {
	switch r.Kind {
	case walk.KindFile, walk.KindDirectory:
		if r.Repo != "" {
			return r.Repo + " › " + r.Path
		}

		return r.Path
	case walk.KindURI:
		return "(link)"
	default:
		return "(content only)"
	}
}

func lineRange(r anchor.Range) string {
	if r.StartLine == 0 {
		return ""
	}

	if r.StartLine == r.EndLine {
		return fmt.Sprintf("line %d", r.StartLine)
	}

	return fmt.Sprintf("lines %d-%d", r.StartLine, r.EndLine)
}

func printSummary(b *strings.Builder, r *resolvedStep) {
	fmt.Fprintf(b, "%3d  %-10s %-22s %s", r.Number, r.Status, r.Step, where(r))

	if lr := lineRange(r.Range); lr != "" {
		fmt.Fprintf(b, "  %s", lr)
	}

	if r.Detail != "" {
		fmt.Fprintf(b, "  (%s)", r.Detail)
	}

	b.WriteByte('\n')
}

func printDetail(b *strings.Builder, r *resolvedStep) {
	fmt.Fprintf(b, "step %s (#%d)  %s\n", r.Step, r.Number, where(r))
	fmt.Fprintf(b, "status: %s", r.Status)

	if lr := lineRange(r.Range); lr != "" {
		fmt.Fprintf(b, "  %s", lr)
	}

	b.WriteByte('\n')

	if r.Detail != "" {
		fmt.Fprintf(b, "%s\n", r.Detail)
	}

	if len(r.Lines) == 0 {
		return
	}

	b.WriteByte('\n')

	width := len(strconv.Itoa(r.Lines[len(r.Lines)-1].Number))

	for i := range r.Lines {
		marker := ' '
		if r.Lines[i].Anchored {
			marker = '>'
		}

		fmt.Fprintf(b, "%c %*d | %s\n", marker, width, r.Lines[i].Number, r.Lines[i].Text)
	}

	if r.Range.EndLine > r.Lines[len(r.Lines)-1].Number {
		fmt.Fprintf(b, "  … %d more anchored lines\n", r.Range.EndLine-r.Lines[len(r.Lines)-1].Number)
	}
}
