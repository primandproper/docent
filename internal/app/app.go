package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/tour"

	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// ChangedEvent is emitted to the frontend when a tour, or a file beside one,
// changes on disk.
const ChangedEvent = "docent:changed"

// Options configure the desktop app.
type Options struct {
	_ struct{}
	// Assets is the built frontend.
	Assets fs.FS
	Logger logging.Logger
	// SystemLogger receives Wails' own messages. Nil discards them.
	SystemLogger   *slog.Logger
	TracerProvider tracing.Provider
	// VarFlags are link variables given on the command line.
	VarFlags map[string]string
	// Root is the absolute path of the folder to open.
	Root string
}

// Run opens a window on the workspace and blocks until the app quits. It must
// be called from the main goroutine: AppKit insists on the main thread.
func Run(ctx context.Context, opts *Options) error {
	services, err := core.New(&core.Options{
		Root:           opts.Root,
		VarFlags:       opts.VarFlags,
		Logger:         opts.Logger,
		TracerProvider: opts.TracerProvider,
	})
	if err != nil {
		return fmt.Errorf("setting up: %w", err)
	}

	defer services.Close()

	launcher := editor.NewLauncher(services.Config, opts.Logger, opts.TracerProvider)
	service := NewService(opts.Root, services, launcher, opts.Logger, opts.TracerProvider)

	systemLogger := opts.SystemLogger
	if systemLogger == nil {
		systemLogger = slog.New(slog.DiscardHandler)
	}

	wails := application.New(application.Options{
		Name:        "Docent",
		Description: "Guided tours of code across repos",
		Logger:      systemLogger,
		Services: []application.Service{
			application.NewService(service),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(opts.Assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            filepath.Base(opts.Root),
		Width:            1440,
		Height:           900,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(24, 24, 27),
		URL:              "/",
	})

	// Live reload: the app is the preview for whoever is editing a tour.
	// Without it the reader still has ⌘R, so a failed watch isn't fatal.
	watchErr := tour.Watch(ctx, opts.Logger, func() { wails.Event.Emit(ChangedEvent) }, opts.Root, services.Config.TourPaths()...)
	if watchErr != nil {
		logging.EnsureLogger(opts.Logger).Error("watching tours for changes", watchErr)
	}

	// Ctrl-C in the launching terminal should close the window, not orphan it.
	go func() {
		<-ctx.Done()
		wails.Quit()
	}()

	if err = wails.Run(); err != nil {
		return fmt.Errorf("running the app: %w", err)
	}

	return nil
}
