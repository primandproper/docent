package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/diag"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const (
	cleanTour = `{"title": "Clean", "steps": [{ "description": "d", "file": "a.go", "line": 3, "pattern": "^func A" }]}`
	movedTour = `{"title": "Moved", "steps": [{ "description": "d", "file": "a.go", "line": 1, "pattern": "^func A" }]}`
	brokeTour = `{"title": "Broken", "steps": [{ "description": "d", "file": "gone.go" }]}`
)

// workspace builds a folder with one source file and the given tours.
func workspace(t *testing.T, tours map[string]string) string {
	t.Helper()

	root, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	must.NoError(t, os.MkdirAll(filepath.Join(root, ".tours"), 0o750))
	must.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("package a\n\nfunc A() {}\n"), 0o600))

	for name, content := range tours {
		must.NoError(t, os.WriteFile(filepath.Join(root, ".tours", name), []byte(content), 0o600))
	}

	return root
}

func run(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()

	var out, errOut bytes.Buffer

	code = Execute(t.Context(), &Options{
		Args:    args,
		Streams: Streams{Stdout: &out, Stderr: &errOut},
		LogDir:  t.TempDir(),
	})

	return code, out.String(), errOut.String()
}

func TestValidate(T *testing.T) {
	T.Parallel()

	T.Run("a clean workspace exits 0", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": cleanTour})

		code, stdout, stderr := run(t, "validate", root)
		test.Eq(t, ExitClean, code)
		test.Eq(t, "0 errors, 0 warnings in 1 tour\n", stdout)
		test.Eq(t, "", stderr)
	})

	T.Run("errors exit 1 and say where", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": cleanTour, "b.tour": brokeTour})

		code, stdout, _ := run(t, "validate", root)
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stdout, filepath.Join(root, ".tours", "b.tour"))
		test.StrContains(t, stdout, "error   file-missing  step #1 /steps/0/file")
		test.StrContains(t, stdout, "1 error, 0 warnings in 2 tours\n")
	})

	T.Run("warnings exit 0, or 2 under --strict", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": movedTour})

		code, _, _ := run(t, "validate", root)
		test.Eq(t, ExitClean, code)

		code, stdout, _ := run(t, "validate", "--strict", root)
		test.Eq(t, ExitWarnings, code)
		test.StrContains(t, stdout, "warning anchor-moved")
	})

	T.Run("errors outrank warnings under --strict", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": movedTour, "b.tour": brokeTour})

		code, _, _ := run(t, "validate", "--strict", root)
		test.Eq(t, ExitErrors, code)
	})

	T.Run("--json prints the contract and nothing else", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"b.tour": brokeTour})
		path := filepath.Join(root, ".tours", "b.tour")

		code, stdout, _ := run(t, "validate", "--json", path)
		test.Eq(t, ExitErrors, code)

		var report map[string]any
		must.NoError(t, json.Unmarshal([]byte(stdout), &report))
		test.Eq(t, 1.0, report["tours"])
		test.Eq(t, 1.0, report["errors"])
		test.Eq(t, 0.0, report["warnings"])

		diagnostics, ok := report["diagnostics"].([]any)
		must.True(t, ok)
		must.Len(t, 1, diagnostics)

		// Every documented key is present, even when empty.
		test.Eq[any](t, map[string]any{
			"file":     path,
			"pointer":  "/steps/0/file",
			"step":     "#1",
			"severity": string(diag.SeverityError),
			"code":     string(diag.CodeFileMissing),
			"message":  `"gone.go" does not exist`,
			"hint":     "Paths are relative to the folder that contains the tours directory.",
		}, diagnostics[0])
	})

	T.Run("--json on a clean run is an empty array, not null", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": cleanTour})

		code, stdout, _ := run(t, "validate", "--json", root)
		test.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, `"diagnostics": []`)
	})

	T.Run("a folder without tours is an error, not a clean run", func(t *testing.T) {
		t.Parallel()

		code, stdout, stderr := run(t, "validate", t.TempDir())
		test.Eq(t, ExitErrors, code)
		test.Eq(t, "", stdout)
		test.StrContains(t, stderr, "no tours found")
	})

	T.Run("a path that doesn't exist is an error", func(t *testing.T) {
		t.Parallel()

		code, _, stderr := run(t, "validate", filepath.Join(t.TempDir(), "nope.tour"))
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, "does not exist")
	})
}

func TestRoot(T *testing.T) {
	T.Parallel()

	T.Run("something that is neither a folder nor a command is refused", func(t *testing.T) {
		t.Parallel()

		code, _, stderr := run(t, "valdiate")
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, "not a folder and not a docent command")
	})

	T.Run("logs go to the log directory, not stdout", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": cleanTour})
		logDir := t.TempDir()

		var out, errOut bytes.Buffer

		code := Execute(t.Context(), &Options{
			Args:    []string{"--log-level", "debug", "validate", root},
			Streams: Streams{Stdout: &out, Stderr: &errOut},
			LogDir:  logDir,
		})
		must.Eq(t, ExitClean, code)
		test.Eq(t, "0 errors, 0 warnings in 1 tour\n", out.String())

		logged, err := os.ReadFile(filepath.Join(logDir, "docent.log"))
		must.NoError(t, err)
		test.StrContains(t, string(logged), `"msg":"validated tours"`)
		test.StrContains(t, string(logged), `"msg":"discovered tours"`)
	})
}
