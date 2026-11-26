// Package cli is Docent's command line. With no arguments or a folder it opens
// the app; with a subcommand it runs headless and never starts the webview.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/primandproper/docent/internal/app"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/logs"
	"github.com/primandproper/docent/internal/secrets"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/version"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
	tracingnoop "github.com/primandproper/primitives-go/v2/observability/tracing/noop"

	"github.com/spf13/cobra"
)

// LogLevelEnvVar seeds the --log-level flag.
const LogLevelEnvVar = "DOCENT_LOG_LEVEL"

// Exit codes. Anything that isn't a validation verdict exits with ExitErrors.
const (
	ExitClean    = 0
	ExitErrors   = 1
	ExitWarnings = 2
)

type (
	// Streams are where a command reads and writes. Stdout carries only a
	// command's result, so it stays safe to pipe; logs go to a file.
	Streams struct {
		_      struct{}
		Stdout io.Writer
		Stderr io.Writer
		// Stdin is where a secret is read from. Nil means there is none.
		Stdin io.Reader
	}

	// exitError carries a specific exit code out of a command without
	// printing anything more.
	exitError struct {
		_    struct{}
		code int
	}

	// Options are what main hands the command line.
	Options struct {
		_ struct{}
		// Assets is the built frontend, served to the webview when the app opens.
		Assets  fs.FS
		Streams Streams
		// LogDir overrides where logs are written. Empty means the default.
		LogDir string
		// UserConfigPath overrides ~/.config/docent/config.json.
		UserConfigPath string
		// LibraryPath overrides ~/.config/docent/library.json, the folders
		// the app was told to keep.
		LibraryPath string
		// Secrets overrides the operating system's credential store. Tests
		// set it, so they never touch the real one.
		Secrets secrets.Store
		// Getenv overrides os.Getenv for what the services read. Tests set it.
		Getenv func(string) string
		// Args are the command-line arguments, without the program name.
		Args []string
	}

	application struct {
		_              struct{}
		streams        Streams
		assets         fs.FS
		logger         logging.Logger
		tracerProvider tracing.Provider
		logFile        io.Closer
		systemLogger   *slog.Logger
		secrets        secrets.Store
		getenv         func(string) string
		logDir         string
		userConfigPath string
		libraryPath    string
	}
)

func (e *exitError) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

// Execute runs the command line and returns the process exit code.
func Execute(ctx context.Context, opts *Options) int {
	a := &application{
		assets:         opts.Assets,
		streams:        opts.Streams,
		logDir:         opts.LogDir,
		userConfigPath: opts.UserConfigPath,
		libraryPath:    opts.LibraryPath,
		secrets:        opts.Secrets,
		getenv:         opts.Getenv,
		tracerProvider: tracingnoop.NewTracerProvider(),
	}
	defer a.shutdown()

	streams := opts.Streams

	root := a.newRootCommand()
	root.SetArgs(opts.Args)
	root.SetOut(streams.Stdout)
	root.SetErr(streams.Stderr)

	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitClean
	}

	var exit *exitError
	if errors.As(err, &exit) {
		return exit.code
	}

	a.log().Error("command failed", err)
	fmt.Fprintf(streams.Stderr, "docent: %v\n", err) //nolint:errcheck // nothing to do if stderr is gone

	return ExitErrors
}

func (a *application) newRootCommand() *cobra.Command {
	var (
		logLevel string
		varFlags []string
	)

	root := &cobra.Command{
		Use:   "docent [path]",
		Short: "Walk guided tours of code that span several repos.",
		Long: "Docent opens a window for walking the tours found under a folder (default: the\n" +
			"current one). Subcommands run headless.",
		Args:          cobra.MaximumNArgs(1),
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			a.bootstrap(logLevel)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := parseVars(varFlags)
			if err != nil {
				return err
			}

			return a.open(cmd.Context(), args, vars)
		},
	}

	root.Flags().StringArrayVar(&varFlags, "var", nil, "set a link variable, as name=value (repeatable)")

	root.PersistentFlags().StringVar(&logLevel, "log-level", envOr(LogLevelEnvVar, string(logging.InfoLevel)), "log level: debug, info, warn or error")

	root.AddCommand(
		a.newOpenCommand(),
		a.newValidateCommand(),
		a.newResolveCommand(),
		a.newNewCommand(),
		a.newFmtCommand(),
		a.newStampCommand(),
		a.newReviewCommand(),
		a.newStaleCommand(),
		a.newSchemaCommand(),
		a.newGuideCommand(),
		a.newDoctorCommand(),
		a.newIntegrationsCommand(),
	)

	return root
}

// bootstrap opens the log file. Logging is best effort: a machine where the
// log directory can't be written still gets a working docent, with a note on
// stderr and no logs.
func (a *application) bootstrap(logLevel string) {
	level := logging.Level(strings.ToLower(strings.TrimSpace(logLevel)))
	if !level.Valid() {
		fmt.Fprintf(a.streams.Stderr, "docent: unknown log level %q, using info\n", logLevel) //nolint:errcheck // best effort

		level = logging.InfoLevel
	}

	dir, err := a.logDir, error(nil)
	if dir == "" {
		dir, err = logs.DefaultDir()
	}

	if err == nil {
		a.logger, a.systemLogger, a.logFile, err = logs.Open(dir, level)
	}

	if err != nil {
		fmt.Fprintf(a.streams.Stderr, "docent: logging is disabled: %v\n", err) //nolint:errcheck // best effort
	}
}

func (a *application) shutdown() {
	if a.logFile == nil {
		return
	}

	if err := a.logFile.Close(); err != nil {
		fmt.Fprintf(a.streams.Stderr, "docent: closing the log file: %v\n", err) //nolint:errcheck // best effort
	}
}

func (a *application) log() logging.Logger {
	return logging.EnsureLogger(a.logger)
}

// open starts the app rooted at the given folder.
func (a *application) open(ctx context.Context, args []string, vars map[string]string) error {
	path := "."
	if len(args) > 0 {
		path = args[0]
	}

	root, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", path, err)
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%q is not a folder and not a docent command; run `docent --help` for the commands", path)
	}

	// Launched from Finder there is no folder: the working directory is the
	// filesystem root, which nobody means. The app starts empty and asks.
	if len(args) == 0 && root == filepath.Dir(root) {
		root = ""
	}

	return a.runApp(ctx, &app.Options{Root: root, VarFlags: vars})
}

// runApp starts the desktop app.
func (a *application) runApp(ctx context.Context, opts *app.Options) error {
	opts.Assets = a.assets
	opts.Logger = a.log()
	opts.SystemLogger = a.systemLogger
	opts.TracerProvider = a.tracerProvider
	opts.LibraryPath = a.libraryPath
	opts.Secrets = a.secrets
	opts.Getenv = a.getenv

	a.log().WithValues(map[string]any{"root": opts.Root, "tour": opts.Tour}).Info("opening app")

	return app.Run(ctx, opts)
}

func (a *application) newOpenCommand() *cobra.Command {
	var (
		step     string
		varFlags []string
	)

	cmd := &cobra.Command{
		Use:   "open <tour>",
		Short: "Open the app on one tour, optionally at a step.",
		Long: "Open starts the app in the tour's folder with that tour open. A link of the form\n" +
			"docent://open?tour=<absolute path>&step=<id> does the same from anywhere.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			vars, err := parseVars(varFlags)
			if err != nil {
				return err
			}

			abs, err := filepath.Abs(args[0])
			if err != nil {
				return fmt.Errorf("resolving %q: %w", args[0], err)
			}

			if info, statErr := os.Stat(abs); statErr != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("%q is not a tour file", args[0])
			}

			return a.runApp(cmd.Context(), &app.Options{Root: tour.RootFor(abs), Tour: abs, Step: step, VarFlags: vars})
		},
	}

	cmd.Flags().StringVar(&step, "step", "", "the step to start at: its id, or its number counting from 1")
	cmd.Flags().StringArrayVar(&varFlags, "var", nil, "set a link variable, as name=value (repeatable)")

	return cmd
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return fallback
}

// parseVars turns repeated name=value flags into a map.
func parseVars(flags []string) (map[string]string, error) {
	vars := make(map[string]string, len(flags))

	for _, flag := range flags {
		name, value, ok := strings.Cut(flag, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--var %q: want name=value", flag)
		}

		vars[name] = value
	}

	return vars, nil
}

// coreFor assembles the services for a folder.
func (a *application) coreFor(root string) (*core.Core, error) {
	return core.New(&core.Options{
		Root:           root,
		UserConfigPath: a.userConfigPath,
		Secrets:        a.secrets,
		Getenv:         a.getenv,
		Logger:         a.log(),
		TracerProvider: a.tracerProvider,
	})
}
