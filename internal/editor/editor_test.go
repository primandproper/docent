package editor

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/primandproper/docent/internal/workspace"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// fakeSystem is a machine with the given commands and apps installed.
type fakeSystem struct {
	commands []string
	apps     []string
	failing  []string
	ran      [][]string
}

func (f *fakeSystem) LookPath(name string) (string, error) {
	if slices.Contains(f.commands, name) {
		return "/bin/" + name, nil
	}

	return "", errors.New("not found")
}

func (f *fakeSystem) AppInstalled(name string) bool {
	return slices.Contains(f.apps, name)
}

func (f *fakeSystem) Run(_ context.Context, argv []string) error {
	f.ran = append(f.ran, argv)

	if slices.Contains(f.failing, argv[0]) {
		return errors.New("boom")
	}

	return nil
}

func config(defaultEditor string, custom map[string]workspace.CustomEditor) *workspace.Config {
	return &workspace.Config{User: workspace.File{Editors: workspace.Editors{Default: defaultEditor, Custom: custom}}}
}

var target = &Target{Path: "/w/my repo/a b.go", Line: 12, Col: 3}

func TestLauncher_Detect(T *testing.T) {
	T.Parallel()

	T.Run("finds editors by shim or by app bundle, custom ones first", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"code", "ghostty"}, apps: []string{"GoLand.app"}}
		cfg := config("", map[string]workspace.CustomEditor{
			"helix":   {Argv: []string{"ghostty", "-e", "hx", "{path}:{line}"}},
			"missing": {Argv: []string{"nope"}},
		})

		got := NewLauncherOn(system, cfg, nil, nil).Detect(t.Context())
		test.Eq(t, []Editor{{ID: "helix", Name: "helix"}, {ID: "code", Name: "VS Code"}, {ID: "goland", Name: "GoLand"}}, got)
	})

	T.Run("finds nothing on a bare machine", func(t *testing.T) {
		t.Parallel()

		test.Len(t, 0, NewLauncherOn(&fakeSystem{}, config("", nil), nil, nil).Detect(t.Context()))
	})
}

func TestLauncher_Open(T *testing.T) {
	T.Parallel()

	T.Run("uses the configured default through its shim", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"code", "zed"}}

		result, err := NewLauncherOn(system, config("code", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, &Result{Via: ViaEditor, Editor: Editor{ID: "code", Name: "VS Code"}}, result)
		test.Eq(t, [][]string{{"/bin/code", "--goto", "/w/my repo/a b.go:12:3"}}, system.ran)
	})

	T.Run("drives an app without a shim by its URL scheme", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{apps: []string{"IntelliJ IDEA CE.app"}}

		_, err := NewLauncherOn(system, config("idea", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, [][]string{{"open", "-a", "IntelliJ IDEA CE.app", "idea://open?file=%2Fw%2Fmy+repo%2Fa+b.go&line=12"}}, system.ran)
	})

	T.Run("an explicit editor wins over the default", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"code", "zed"}}

		result, err := NewLauncherOn(system, config("code", nil), nil, nil).Open(t.Context(), target, "zed")
		must.NoError(t, err)
		test.Eq(t, "zed", result.Editor.ID)
		test.Eq(t, [][]string{{"/bin/zed", "/w/my repo/a b.go:12:3"}}, system.ran)
	})

	T.Run("a custom editor's template is filled without splitting arguments", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"ghostty"}}
		cfg := config("helix", map[string]workspace.CustomEditor{"helix": {Argv: []string{"ghostty", "-e", "hx", "{path}:{line}:{col}"}}})

		result, err := NewLauncherOn(system, cfg, nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, "helix", result.Editor.Name)
		test.Eq(t, [][]string{{"ghostty", "-e", "hx", "/w/my repo/a b.go:12:3"}}, system.ran)
	})

	T.Run("a missing default says so and falls to the next detected editor", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"zed"}}

		result, err := NewLauncherOn(system, config("idea", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, &Result{Via: ViaEditor, Editor: Editor{ID: "zed", Name: "Zed"}, Note: "IntelliJ IDEA isn't installed"}, result)
	})

	T.Run("with no default, the first detected editor is used silently", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"code"}}

		result, err := NewLauncherOn(system, config("", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, "code", result.Editor.ID)
		test.Eq(t, "", result.Note)
	})

	T.Run("an editor that fails to start falls through too", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{commands: []string{"code", "zed"}, failing: []string{"/bin/code"}}

		result, err := NewLauncherOn(system, config("code", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, "zed", result.Editor.ID)
		test.Eq(t, "VS Code couldn't be started", result.Note)
	})

	T.Run("with no editor at all, the default app opens the file", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{}

		result, err := NewLauncherOn(system, config("", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, &Result{Via: ViaDefaultApp, Note: "No editor was found"}, result)
		test.Eq(t, [][]string{{"open", "/w/my repo/a b.go"}}, system.ran)
	})

	T.Run("and if that fails, Finder reveals it", func(t *testing.T) {
		t.Parallel()

		system := &failFirstOpen{}

		result, err := NewLauncherOn(system, config("", nil), nil, nil).Open(t.Context(), target, "")
		must.NoError(t, err)
		test.Eq(t, ViaFinder, result.Via)
	})

	T.Run("when nothing works it is an error", func(t *testing.T) {
		t.Parallel()

		system := &fakeSystem{failing: []string{"open"}}

		_, err := NewLauncherOn(system, config("", nil), nil, nil).Open(t.Context(), target, "")
		test.ErrorIs(t, err, ErrNothingWorked)
	})
}

// failFirstOpen fails a plain `open <path>` and lets `open -R` through.
type failFirstOpen struct{ fakeSystem }

func (f *failFirstOpen) Run(_ context.Context, argv []string) error {
	if len(argv) == 2 {
		return errors.New("no default app")
	}

	return nil
}

func TestFill(T *testing.T) {
	T.Parallel()

	T.Run("defaults line and column to one", func(t *testing.T) {
		t.Parallel()

		got := fill([]string{"{path}:{line}:{col}", "{col0}", "{urlpath}"}, &Target{Path: "/a b/c#d.go"})
		test.Eq(t, []string{"/a b/c#d.go:1:1", "0", "/a%20b/c%23d.go"}, got)
	})
}
