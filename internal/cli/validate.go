package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/tour"

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
	var asJSON, strict, workingTree bool

	cmd := &cobra.Command{
		Use:   "validate [paths...]",
		Short: "Check tours against the schema and the code they point at.",
		Long: "Validate checks each tour's schema, that its files exist, that lines are in range\n" +
			"and that each pattern matches exactly once.\n\n" +
			"A path is a .tour file or a folder to search (default: the current folder).\n" +
			"Exit status: 0 clean, 1 errors, 2 warnings only and --strict.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.validate(cmd.Context(), args, asJSON, strict, workingTree)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "print diagnostics as JSON")
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 2 when there are warnings and no errors")
	cmd.Flags().BoolVar(&workingTree, "working-tree", false, "check against files as they are on disk, not at each repo's ref")

	return cmd
}

func (a *application) validate(ctx context.Context, args []string, asJSON, strict, workingTree bool) error {
	if len(args) == 0 {
		args = []string{"."}
	}

	report := validateReport{Diagnostics: []diag.Diagnostic{}}

	for _, arg := range args {
		services, paths, err := a.tourPaths(ctx, arg)
		if err != nil {
			return err
		}

		report.Tours += len(paths)
		services.Walker.SetWorkingTree(workingTree)

		for _, path := range paths {
			diags, tourErr := services.Validator.Tour(ctx, path)
			if tourErr != nil {
				services.Close()
				return fmt.Errorf("validating %q: %w", path, tourErr)
			}

			report.Diagnostics = append(report.Diagnostics, diags...)
		}

		services.Close()
	}

	for i := range report.Diagnostics {
		if report.Diagnostics[i].Severity == diag.SeverityError {
			report.Errors++
		} else {
			report.Warnings++
		}
	}

	if asJSON {
		if err := writeJSON(a.streams.Stdout, report); err != nil {
			return err
		}
	} else if err := printReport(a.streams.Stdout, &report); err != nil {
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

// tourPaths expands one argument into tour files and the services to check
// them with. A file is taken as is, and its configuration is its tour root's;
// a folder is searched. A folder with no tours is an error, so a mistyped
// path can't pass for a clean run.
func (a *application) tourPaths(ctx context.Context, arg string) (*core.Core, []string, error) {
	abs, err := filepath.Abs(arg)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving %q: %w", arg, err)
	}

	info, err := os.Stat(abs)
	if err != nil {
		return nil, nil, fmt.Errorf("%q does not exist", arg)
	}

	if !info.IsDir() {
		services, coreErr := a.coreFor(tour.RootFor(abs))
		if coreErr != nil {
			return nil, nil, coreErr
		}

		return services, []string{abs}, nil
	}

	services, err := a.coreFor(abs)
	if err != nil {
		return nil, nil, err
	}

	found, err := services.Store.Discover(ctx, abs)
	if err != nil {
		return nil, nil, fmt.Errorf("searching %q: %w", arg, err)
	}

	if len(found) == 0 {
		return nil, nil, fmt.Errorf("no tours found under %q (looked in .tours, .vscode/tours and .github/tours)", arg)
	}

	return services, found, nil
}

func writeJSON(w io.Writer, value any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if err := enc.Encode(value); err != nil {
		return fmt.Errorf("writing output: %w", err)
	}

	return nil
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
