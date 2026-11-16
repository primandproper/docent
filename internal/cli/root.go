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
	"github.com/primandproper/docent/internal/logs"

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
		logDir         string
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
	var logLevel string

	root := &cobra.Command{
		Use:   "docent [path]",
		Short: "Walk guided tours of code that span several repos.",
		Long: "Docent opens a window for walking the tours found under a folder (default: the\n" +
			"current one). Subcommands run headless.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			a.bootstrap(logLevel)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.open(cmd.Context(), args)
		},
	}

	root.PersistentFlags().StringVar(&logLevel, "log-level", envOr(LogLevelEnvVar, string(logging.InfoLevel)), "log level: debug, info, warn or error")

	root.AddCommand(a.newValidateCommand())

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
func (a *application) open(ctx context.Context, args []string) error {
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

	a.log().WithValue("root", root).Info("opening app")

	return app.Run(ctx, &app.Options{
		Root:           root,
		Assets:         a.assets,
		Logger:         a.log(),
		SystemLogger:   a.systemLogger,
		TracerProvider: a.tracerProvider,
	})
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}

	return fallback
}
