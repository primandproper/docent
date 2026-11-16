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
	"github.com/wailsapp/wails/v3/pkg/events"
)

// ChangedEvent is emitted to the frontend when a tour, or a file beside one,
// changes on disk.
const ChangedEvent = "docent:changed"

// OpenEvent is emitted to the frontend when a docent:// link asks for a tour.
const OpenEvent = "docent:open"

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
	// Root is the absolute path of the folder to open, or "" to start with
	// none, which is how the app starts from Finder.
	Root string
	// Tour and Step name a tour to open straight away.
	Tour string
	Step string
}

// Run opens a window and blocks until the app quits. It must be called from
// the main goroutine: AppKit insists on the main thread.
func Run(ctx context.Context, opts *Options) error {
	logger := logging.EnsureLogger(opts.Logger)

	rebuild := func(root string) (*core.Core, error) {
		return core.New(&core.Options{
			Root:           root,
			VarFlags:       opts.VarFlags,
			Logger:         opts.Logger,
			TracerProvider: opts.TracerProvider,
		})
	}

	// With no folder there is still a (rootless) set of services, so nothing
	// downstream has to care.
	services, err := rebuild(opts.Root)
	if err != nil {
		return fmt.Errorf("setting up: %w", err)
	}

	launcher := editor.NewLauncher(services.Config, opts.Logger, opts.TracerProvider)
	service := NewService(opts.Root, services, launcher, opts.Logger, opts.TracerProvider)

	// The service owns whichever services are current when the app quits.
	defer func() { service.svc().Close() }()

	if opts.Tour != "" {
		service.request(LaunchRequest{Tour: opts.Tour, Step: opts.Step})
	}

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

	window := wails.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            title(opts.Root),
		Width:            1440,
		Height:           900,
		MinWidth:         900,
		MinHeight:        560,
		BackgroundColour: application.NewRGB(24, 24, 27),
		URL:              "/",
	})

	// Live reload: the app is the preview for whoever is editing a tour. The
	// watch follows the folder, so it is restarted whenever that changes.
	// Without it the reader still has ⌘R, so a failed watch isn't fatal.
	var stopWatching context.CancelFunc

	watch := func(root string, current *core.Core) {
		if stopWatching != nil {
			stopWatching()
		}

		if root == "" {
			return
		}

		var watchCtx context.Context
		watchCtx, stopWatching = context.WithCancel(ctx)

		if watchErr := tour.Watch(watchCtx, opts.Logger, func() { wails.Event.Emit(ChangedEvent) }, root, current.Config.TourPaths()...); watchErr != nil {
			logger.Error("watching tours for changes", watchErr)
		}
	}
	watch(opts.Root, services)

	service.setHost(&Host{
		Rebuild: rebuild,
		ChooseFolder: func() (string, error) {
			return wails.Dialog.OpenFile().
				SetTitle("Open a folder of tours").
				CanChooseDirectories(true).
				CanChooseFiles(false).
				PromptForSingleSelection()
		},
		Rooted: func(root string, current *core.Core) {
			watch(root, current)
			window.SetTitle(title(root))
		},
	})

	// A docent://open link (tour and step in its query) arrives here, whether
	// it launched the app or came in while it was running.
	wails.Event.OnApplicationEvent(events.Common.ApplicationLaunchedWithUrl, func(event *application.ApplicationEvent) {
		if linkErr := service.openLink(ctx, event.Context().URL()); linkErr != nil {
			logger.Error("opening a docent link", linkErr)
			return
		}

		wails.Event.Emit(OpenEvent)
	})

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

func title(root string) string {
	if root == "" {
		return "Docent"
	}

	return filepath.Base(root)
}
