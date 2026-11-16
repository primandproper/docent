package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"

	"github.com/spf13/cobra"
)

// validateReport is the shape of `docent validate --json`. It is a stable
// contract: fields are added, never renamed or removed.
type validateReport struct {
	_           struct{}          `json:"-"`
	Diagnostics []diag.Diagnostic `json:"diagnostics"`
	Tours       int               `json:"tours"`
	Errors      int               `json:"errors"`
	Warnings    int               `json:"warnings"`
}

func (a *application) newValidateCommand() *cobra.Command {
	var asJSON, strict bool

	cmd := &cobra.Command{
		Use:   "validate [paths...]",
		Short: "Check tours against the schema and the code they point at.",
		Long: "Validate checks each tour's schema, that its files exist, that lines are in range\n" +
			"and that each pattern matches exactly once.\n\n" +
			"A path is a .tour file or a folder to search (default: the current folder).\n" +
			"Exit status: 0 clean, 1 errors, 2 warnings only and --strict.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.validate(cmd.Context(), args, asJSON, strict)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print diagnostics as JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 2 when there are warnings and no errors")

	return cmd
}

func (a *application) validate(ctx context.Context, args []string, asJSON, strict bool) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	store := tour.NewStore(a.log(), a.tracerProvider)
	walker := walk.NewWalker(gitsrc.NewWorkingTree(a.log(), a.tracerProvider), a.log(), a.tracerProvider)
	validator := validate.NewValidator(store, walker, a.log(), a.tracerProvider)

	paths, err := tourPaths(ctx, store, args)
	if err != nil {
		return err
	}

	report := validateReport{Tours: len(paths), Diagnostics: []diag.Diagnostic{}}

	for _, path := range paths {
		diags, tourErr := validator.Tour(ctx, path)
		if tourErr != nil {
			return fmt.Errorf("validating %q: %w", path, tourErr)
		}

		report.Diagnostics = append(report.Diagnostics, diags...)
	}

	for i := range report.Diagnostics {
		if report.Diagnostics[i].Severity == diag.SeverityError {
			report.Errors++
		} else {
			report.Warnings++
		}
	}

	if asJSON {
		enc := json.NewEncoder(a.streams.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)

		if err = enc.Encode(report); err != nil {
			return fmt.Errorf("writing report: %w", err)
		}
	} else if err = printReport(a.streams.Stdout, &report); err != nil {
		return fmt.Errorf("writing report: %w", err)
	}

	a.log().WithValues(map[string]any{"tours": report.Tours, "errors": report.Errors, "warnings": report.Warnings}).Info("validated tours")

	switch {
	case report.Errors > 0:
		return &exitError{code: ExitErrors}
	case report.Warnings > 0 && strict:
		return &exitError{code: ExitWarnings}
	default:
		return nil
	}
}

// tourPaths expands arguments into tour files: a file is taken as is, a folder
// is searched. A folder with no tours is an error, so a mistyped path can't
// pass for a clean run.
func tourPaths(ctx context.Context, store tour.Store, args []string) ([]string, error) {
	var paths []string

	for _, arg := range args {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return nil, fmt.Errorf("resolving %q: %w", arg, err)
		}

		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("%q does not exist", arg)
		}

		if !info.IsDir() {
			paths = append(paths, abs)
			continue
		}

		found, err := store.Discover(ctx, abs)
		if err != nil {
			return nil, fmt.Errorf("searching %q: %w", arg, err)
		}

		if len(found) == 0 {
			return nil, fmt.Errorf("no tours found under %q (looked in .tours, .vscode/tours and .github/tours)", arg)
		}

		paths = append(paths, found...)
	}

	return paths, nil
}

func printReport(w io.Writer, report *validateReport) error {
	var b strings.Builder

	lastFile := ""

	for i := range report.Diagnostics {
		d := &report.Diagnostics[i]

		if d.File != lastFile {
			if lastFile != "" {
				b.WriteByte('\n')
			}

			b.WriteString(d.File + "\n")
			lastFile = d.File
		}

		where := d.Pointer
		if d.Step != "" {
			where = "step " + d.Step + " " + where
		}

		fmt.Fprintf(&b, "  %-7s %s  %s\n          %s\n", d.Severity, d.Code, strings.TrimSpace(where), d.Message)

		if d.Hint != "" {
			fmt.Fprintf(&b, "          hint: %s\n", d.Hint)
		}
	}

	if lastFile != "" {
		b.WriteByte('\n')
	}

	fmt.Fprintf(&b, "%s, %s in %s\n", plural(report.Errors, "error"), plural(report.Warnings, "warning"), plural(report.Tours, "tour"))

	_, err := io.WriteString(w, b.String())

	return err
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}

	return fmt.Sprintf("%d %ss", n, noun)
}
