// Package editor opens a file at a line in the reader's editor. Which editor
// is always the reader's choice, from their own config: a tour can't name one,
// because that would be running a command a stranger wrote.
package editor

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/primandproper/docent/internal/workspace"

	"github.com/primandproper/primitives-go/v2/observability"
	"github.com/primandproper/primitives-go/v2/observability/logging"
	"github.com/primandproper/primitives-go/v2/observability/tracing"
)

const o11yName = "editor_launcher"

// How a file ended up being opened.
const (
	// ViaEditor: an editor opened it at the line.
	ViaEditor = "editor"
	// ViaDefaultApp: no editor was available, so macOS's default app for the
	// file opened it, without a line.
	ViaDefaultApp = "default-app"
	// ViaFinder: nothing could open it, so it was revealed in Finder.
	ViaFinder = "finder"
)

const (
	openCommand = "open"
	// atLine is the path:line:col form most editors accept.
	atLine = "{path}:{line}:{col}"
)

// ErrNothingWorked is returned when even revealing the file in Finder failed.
var ErrNothingWorked = errors.New("the file could not be opened or revealed")

type (
	// Editor is an editor Docent knows how to launch.
	Editor struct {
		_    struct{} `json:"-"`
		ID   string   `json:"id"`
		Name string   `json:"name"`
	}

	// Target is a place in a file. Path is absolute; Line and Col are 1-based.
	Target struct {
		_    struct{}
		Path string
		// Repo and RelPath say which tour repo the file is in and where,
		// which is what editor rules match on. Both may be empty.
		Repo    string
		RelPath string
		Line    int
		Col     int
	}

	// Result says what Open actually did.
	Result struct {
		_ struct{} `json:"-"`
		// Via is one of ViaEditor, ViaDefaultApp or ViaFinder.
		Via string `json:"via"`
		// Editor is the editor used, when Via is ViaEditor.
		Editor Editor `json:"editor"`
		// Note explains a fallback: the editor asked for wasn't available.
		Note string `json:"note"`
	}

	// Launcher finds installed editors and opens files in them.
	Launcher interface {
		// Detect lists the editors installed right now, best guess first.
		Detect(ctx context.Context) []Editor
		// Open opens the target in the editor with the given id. With an
		// empty id it uses the first rule that matches the file, else the
		// configured default. When that editor is gone it falls back: the
		// next detected editor, then the default app for the file, then Finder.
		Open(ctx context.Context, target *Target, id string) (*Result, error)
	}

	// preset is a built-in editor: how to find it and how to hand it a line.
	preset struct {
		_    struct{}
		id   string
		name string
		cli  string
		url  string
		args []string
		apps []string
	}

	// System is the launcher's view of the machine, replaceable in tests.
	System interface {
		// LookPath reports where a command is on the PATH.
		LookPath(name string) (string, error)
		// AppInstalled reports whether an app bundle with this name exists.
		AppInstalled(name string) bool
		// Run starts a command and waits for it to hand off.
		Run(ctx context.Context, argv []string) error
	}

	launcher struct {
		_      struct{}
		system System
		config *workspace.Config
		logger logging.Logger
		tracer tracing.Tracer
	}

	hostSystem struct{ _ struct{} }
)

// presetList is the built-in editors, in the order Detect reports them.
var presetList = []preset{
	{
		id: "zed", name: "Zed", cli: "zed", args: []string{atLine},
		apps: []string{"Zed.app"}, url: "zed://file{urlpath}:{line}:{col}",
	},
	{
		id: "cursor", name: "Cursor", cli: "cursor", args: []string{"--goto", atLine},
		apps: []string{"Cursor.app"}, url: "cursor://file{urlpath}:{line}:{col}",
	},
	{
		id: "code", name: "VS Code", cli: "code", args: []string{"--goto", atLine},
		apps: []string{"Visual Studio Code.app"}, url: "vscode://file{urlpath}:{line}:{col}",
	},
	{
		id: "idea", name: "IntelliJ IDEA", cli: "idea", args: []string{"--line", "{line}", "--column", "{col0}", "{path}"},
		apps: []string{"IntelliJ IDEA.app", "IntelliJ IDEA CE.app", "IntelliJ IDEA Ultimate.app"},
		url:  "idea://open?file={query}&line={line}",
	},
	{
		id: "goland", name: "GoLand", cli: "goland", args: []string{"--line", "{line}", "--column", "{col0}", "{path}"},
		apps: []string{"GoLand.app"}, url: "goland://open?file={query}&line={line}",
	},
	{
		id: "subl", name: "Sublime Text", cli: "subl", args: []string{atLine},
		apps: []string{"Sublime Text.app"}, url: "subl://open?url=file://{urlpath}&line={line}&column={col}",
	},
}

// presetByID finds a built-in editor.
func presetByID(id string) (*preset, bool) {
	for i := range presetList {
		if presetList[i].id == id {
			return &presetList[i], true
		}
	}

	return nil, false
}

var (
	_ Launcher = (*launcher)(nil)
	_ System   = hostSystem{}
)

// NewLauncher returns a Launcher for this machine.
func NewLauncher(config *workspace.Config, logger logging.Logger, tracerProvider tracing.Provider) Launcher {
	return NewLauncherOn(hostSystem{}, config, logger, tracerProvider)
}

// NewLauncherOn returns a Launcher over a given System.
func NewLauncherOn(system System, config *workspace.Config, logger logging.Logger, tracerProvider tracing.Provider) Launcher {
	return &launcher{
		system: system,
		config: config,
		logger: logging.NewNamedLogger(logger, o11yName),
		tracer: tracing.NewNamedTracer(tracerProvider, o11yName),
	}
}

func (l *launcher) Detect(ctx context.Context) []Editor {
	_, span := l.tracer.StartSpan(ctx)
	defer span.End()

	var found []Editor

	// Custom editors are the reader's own, so they lead.
	custom := l.config.CustomEditors()

	ids := make([]string, 0, len(custom))
	for id := range custom {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	for _, id := range ids {
		if argv := custom[id].Argv; len(argv) > 0 {
			if _, err := l.system.LookPath(argv[0]); err == nil {
				found = append(found, Editor{ID: id, Name: id})
			}
		}
	}

	for i := range presetList {
		id := presetList[i].id
		if _, isCustom := custom[id]; isCustom {
			continue
		}

		if _, ok := l.command(id, &Target{Path: "/", Line: 1, Col: 1}); ok {
			found = append(found, Editor{ID: id, Name: presetList[i].name})
		}
	}

	return found
}

func (l *launcher) Open(ctx context.Context, target *Target, id string) (*Result, error) {
	ctx, span := l.tracer.StartSpan(ctx)
	defer span.End()

	logger := l.logger.WithValues(map[string]any{"path": target.Path, "line": target.Line})

	// An explicit choice wins, then the reader's rules for this file, then
	// their default.
	wanted := id
	if wanted == "" {
		wanted = l.config.EditorFor(target.Repo, target.RelPath)
	}

	if wanted == "" {
		wanted = l.config.DefaultEditor()
	}

	candidates := []string{}
	if wanted != "" {
		candidates = append(candidates, wanted)
	}

	detected := l.Detect(ctx)
	for i := range detected {
		if detected[i].ID != wanted {
			candidates = append(candidates, detected[i].ID)
		}
	}

	note := ""

	for _, candidate := range candidates {
		argv, ok := l.command(candidate, target)
		if !ok {
			if candidate == wanted {
				note = l.name(wanted) + " isn't installed"
			}

			continue
		}

		if err := l.system.Run(ctx, argv); err != nil {
			observability.AcknowledgeError(err, logger.WithValue("editor", candidate), span, "launching editor")

			if candidate == wanted {
				note = l.name(wanted) + " couldn't be started"
			}

			continue
		}

		logger.WithValue("editor", candidate).Info("opened in editor")

		return &Result{Via: ViaEditor, Editor: Editor{ID: candidate, Name: l.name(candidate)}, Note: note}, nil
	}

	if note == "" {
		note = "No editor was found"
	}

	if err := l.system.Run(ctx, []string{openCommand, target.Path}); err == nil {
		logger.Info("opened in the default app")
		return &Result{Via: ViaDefaultApp, Note: note}, nil
	}

	if err := l.system.Run(ctx, []string{openCommand, "-R", target.Path}); err == nil {
		logger.Info("revealed in Finder")
		return &Result{Via: ViaFinder, Note: note}, nil
	}

	return nil, observability.PrepareAndLogError(ErrNothingWorked, logger, span, "opening %s", target.Path)
}

func (l *launcher) name(id string) string {
	if p, ok := presetByID(id); ok {
		if _, isCustom := l.config.CustomEditors()[id]; !isCustom {
			return p.name
		}
	}

	return id
}

// command builds the argv that opens target in an editor, or reports that the
// editor isn't installed.
func (l *launcher) command(id string, target *Target) ([]string, bool) {
	if custom, ok := l.config.CustomEditors()[id]; ok {
		if len(custom.Argv) == 0 {
			return nil, false
		}

		if _, err := l.system.LookPath(custom.Argv[0]); err != nil {
			return nil, false
		}

		return fill(custom.Argv, target), true
	}

	p, ok := presetByID(id)
	if !ok {
		return nil, false
	}

	if path, err := l.system.LookPath(p.cli); err == nil {
		return append([]string{path}, fill(p.args, target)...), true
	}

	for _, app := range p.apps {
		if l.system.AppInstalled(app) {
			return []string{openCommand, "-a", app, fill([]string{p.url}, target)[0]}, true
		}
	}

	return nil, false
}

// fill substitutes a target into argument templates. Each template stays one
// argument whatever the path contains: nothing is ever passed through a shell.
func fill(templates []string, target *Target) []string {
	line, col := max(target.Line, 1), max(target.Col, 1)

	replacer := strings.NewReplacer(
		"{path}", target.Path,
		"{line}", strconv.Itoa(line),
		"{col}", strconv.Itoa(col),
		"{col0}", strconv.Itoa(col-1),
		"{urlpath}", (&url.URL{Path: target.Path}).EscapedPath(),
		"{query}", url.QueryEscape(target.Path),
	)

	out := make([]string, len(templates))
	for i, t := range templates {
		out[i] = replacer.Replace(t)
	}

	return out
}

func (hostSystem) LookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("looking for %s: %w", name, err)
	}

	return path, nil
}

func (hostSystem) AppInstalled(name string) bool {
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}

	for _, dir := range dirs {
		if info, err := os.Stat(filepath.Join(dir, name)); err == nil && info.IsDir() {
			return true
		}
	}

	return false
}

func (hostSystem) Run(ctx context.Context, argv []string) error {
	// The editor outlives this call and the request that made it.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), argv[0], argv[1:]...) //nolint:gosec // argv comes from built-in presets or the reader's own config, never from a tour

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", argv[0], err)
	}

	// Editor shims hand off and exit at once; reap them without blocking on a
	// terminal editor that stays in the foreground.
	go func() { _ = cmd.Wait() }() //nolint:errcheck // nothing to do with an editor's exit status

	return nil
}
