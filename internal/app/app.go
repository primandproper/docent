package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"sync"

	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/library"
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

// MapEvent carries messages between the main window and the map window. Only
// the frontend emits it; Wails delivers it to every window.
const MapEvent = "docent:map"

// MapClosedEvent is emitted to the frontend when the map window closes.
const MapClosedEvent = "docent:map-closed"

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
	// Root is the absolute path of a folder to open beside the ones saved in
	// the library, or "" for none, which is how the app starts from Finder.
	Root string
	// LibraryPath overrides ~/.config/docent/library.json. Tests set it.
	LibraryPath string
	// Tour and Step name a tour to open straight away.
	Tour string
	Step string
}

// Run opens a window and blocks until the app quits. It must be called from
// the main goroutine: AppKit insists on the main thread.
func Run(ctx context.Context, opts *Options) error {
	logger := logging.EnsureLogger(opts.Logger)

	libraryPath := opts.LibraryPath
	if libraryPath == "" {
		var pathErr error
		if libraryPath, pathErr = library.Path(); pathErr != nil {
			return fmt.Errorf("locating the library: %w", pathErr)
		}
	}

	// These services belong to no folder. Each folder that is opened gets a
	// set of its own.
	bare, err := core.New(&core.Options{VarFlags: opts.VarFlags, Logger: opts.Logger, TracerProvider: opts.TracerProvider})
	if err != nil {
		return fmt.Errorf("setting up: %w", err)
	}

	service := NewService("", bare, editor.NewLauncher(bare.Config, opts.Logger, opts.TracerProvider), opts.Logger, opts.TracerProvider)
	service.setLibrary(library.NewStore(libraryPath, opts.Logger, opts.TracerProvider))

	defer service.close()

	rebuild := func(root string) (*core.Core, error) {
		return core.New(&core.Options{
			Root:           root,
			VarFlags:       opts.VarFlags,
			Elsewhere:      service.elsewhere(root),
			Logger:         opts.Logger,
			TracerProvider: opts.TracerProvider,
		})
	}

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
		Title:            title(nil),
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
	var (
		stopWatching context.CancelFunc
		watchMu      sync.Mutex
	)

	watch := func(folders []*core.Core) {
		watchMu.Lock()
		defer watchMu.Unlock()

		if stopWatching != nil {
			stopWatching()
			stopWatching = nil
		}

		if len(folders) == 0 {
			return
		}

		var watchCtx context.Context
		watchCtx, stopWatching = context.WithCancel(ctx)

		for _, current := range folders {
			root := current.Config.Root

			if watchErr := tour.Watch(watchCtx, opts.Logger, func() { wails.Event.Emit(ChangedEvent) }, root, current.Config.TourPaths()...); watchErr != nil {
				logger.WithValue("root", root).Error("watching tours for changes", watchErr)
			}
		}
	}

	// The system map can be popped out into a window of its own, which floats
	// above the main one. It is the same frontend at another URL, and it is
	// told what to draw by the main window, over MapEvent.
	var (
		mapWindow   *application.WebviewWindow
		mapWindowMu sync.Mutex
	)

	toggleMapWindow := func() {
		mapWindowMu.Lock()
		existing := mapWindow
		mapWindowMu.Unlock()

		if existing != nil {
			existing.Close()
			return
		}

		popup := wails.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:             "map",
			Title:            "System map",
			Width:            560,
			Height:           420,
			MinWidth:         240,
			MinHeight:        160,
			AlwaysOnTop:      true,
			BackgroundColour: application.NewRGB(24, 24, 27),
			URL:              mapWindowURL,
		})

		popup.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			mapWindowMu.Lock()
			mapWindow = nil
			mapWindowMu.Unlock()

			wails.Event.Emit(MapClosedEvent)
		})

		mapWindow = popup
	}

	// The map window is an accessory of the main one: it doesn't keep the app
	// open once the main window is closed.
	window.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		wails.Quit()
	})

	service.setHost(&Host{
		Rebuild:         rebuild,
		ToggleMapWindow: toggleMapWindow,
		Launcher: func(current *core.Core) editor.Launcher {
			return editor.NewLauncher(current.Config, opts.Logger, opts.TracerProvider)
		},
		ChooseFolder: func() (string, error) {
			return wails.Dialog.OpenFile().
				SetTitle("Add a folder of tours").
				CanChooseDirectories(true).
				CanChooseFiles(false).
				PromptForSingleSelection()
		},
		Opened: func(folders []*core.Core) {
			watch(folders)
			window.SetTitle(title(folders))
		},
	})

	// The folder asked for on the command line leads, then the library's.
	if opts.Root != "" {
		if openErr := service.openFolder(ctx, opts.Root, false); openErr != nil {
			return fmt.Errorf("opening %s: %w", opts.Root, openErr)
		}
	}

	// A library that can't be read costs the saved folders, not the app.
	if restoreErr := service.restore(ctx); restoreErr != nil {
		logger.Error("opening the saved folders", restoreErr)
	}

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

// mapWindowURL is where the frontend draws only the system map. It must match
// the check in frontend/src/main.ts.
const mapWindowURL = "/#map"

// title names the window after the one folder that is open, or the app.
func title(folders []*core.Core) string {
	if len(folders) != 1 {
		return "Docent"
	}

	return filepath.Base(folders[0].Config.Root)
}
