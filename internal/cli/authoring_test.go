package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/primandproper/docent/internal/guide"
	"github.com/primandproper/docent/schema"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const crossRepoTour = `{
  "docent": 1,
  "title": "Across repos",
  "repos": {
    "api": { "remote": "git@github.com:acme/api.git" },
    "worker": { "remote": "git@github.com:acme/worker.git" }
  },
  "vars": { "env": { "default": "staging" } },
  "steps": [
    { "id": "overview", "description": "d", "body": "across/01.md" },
    { "id": "handler", "description": "d", "repo": "api", "file": "items.go", "pattern": "^func Update", "line": 5,
      "selection": { "start": { "line": 5, "character": 1 }, "end": { "line": 7, "character": 2 } },
      "links": [{ "kind": "grafana", "title": "p99", "url": "{{grafana}}/d/x?env={{env}}" }] },
    { "id": "purge", "description": "d", "repo": "worker", "file": "purge.go", "pattern": "^func Purge", "line": 1 },
    { "id": "elsewhere", "description": "d", "repo": "billing", "file": "x.go" }
  ]
}`

// crossRepoWorkspace is a tours repo beside an api checkout, with the worker
// repo mapped by docent.json unless mapWorker is false.
func crossRepoWorkspace(t *testing.T, mapWorker bool) (root, tourPath string) {
	t.Helper()

	top, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	files := map[string]string{
		"team-tours/.tours/across.tour":  crossRepoTour,
		"team-tours/.tours/across/01.md": "# Overview\n",
		"api/items.go":                   "package items\n\nfunc Get() {}\n\nfunc Update() {\n\tsave()\n}\n",
		"elsewhere/worker-svc/purge.go":  "package worker\n\nfunc Purge() {}\n",
	}
	if mapWorker {
		files["docent.json"] = `{
			"repos": { "worker": "elsewhere/worker-svc" },
			"vars": { "grafana": "https://grafana.example.com" }, // hosts live here, not in tours
		}`
	}

	for name, content := range files {
		path := filepath.Join(top, filepath.FromSlash(name))
		must.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		must.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}

	return top, filepath.Join(top, "team-tours", ".tours", "across.tour")
}

func TestValidate_CrossRepo(T *testing.T) {
	T.Parallel()

	type finding struct{ Step, Code, Pointer, Severity string }

	findings := func(t *testing.T, stdout string) map[string]string {
		t.Helper()

		var report struct{ Diagnostics []finding }
		must.NoError(t, json.Unmarshal([]byte(stdout), &report))

		got := map[string]string{}
		for _, d := range report.Diagnostics {
			got[d.Step] = d.Code + " " + d.Pointer + " " + d.Severity
		}

		return got
	}

	T.Run("finds repos beside the tours repo and through docent.json", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, true)

		code, stdout, _ := run(t, "validate", "--json", tourPath)
		test.Eq(t, ExitErrors, code)
		test.Eq(t, map[string]string{
			"purge":     "anchor-moved /steps/2/line warning",
			"elsewhere": "repo-unknown /steps/3/repo error",
		}, findings(t, stdout))
	})

	T.Run("without config, an unmapped repo is missing and link hosts are unresolved", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, false)

		code, stdout, _ := run(t, "validate", "--json", tourPath)
		test.Eq(t, ExitErrors, code)
		test.Eq(t, map[string]string{
			"handler":   "link-var-unresolved /steps/1/links/0/url warning",
			"purge":     "repo-missing /steps/2/repo error",
			"elsewhere": "repo-unknown /steps/3/repo error",
		}, findings(t, stdout))

		_, stdout, _ = run(t, "validate", tourPath)
		test.StrContains(t, stdout, `link "p99": no value for {{grafana}}`)
	})
}

func TestResolve(T *testing.T) {
	T.Parallel()

	T.Run("prints the anchored code for one step", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, true)

		code, stdout, _ := run(t, "resolve", tourPath, "--step", "handler")
		test.Eq(t, ExitClean, code)
		test.Eq(t, `step handler (#2)  api › items.go
status: ok  lines 5-7

  3 | func Get() {}
  4 | 
> 5 | func Update() {
> 6 | 	save()
> 7 | }
`, stdout)
	})

	T.Run("accepts a step number, and explains a drifted anchor", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, true)

		code, stdout, _ := run(t, "resolve", tourPath, "--step", "3")
		test.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "step purge (#3)  worker › purge.go\nstatus: moved  line 3\npattern matched on line 3, not line 1\n")
		test.StrContains(t, stdout, "> 3 | func Purge() {}")
	})

	T.Run("without --step, summarizes every step", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, true)

		code, stdout, _ := run(t, "resolve", tourPath)
		test.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "  1  ok         overview               (content only)\n")
		test.StrContains(t, stdout, "  2  ok         handler                api › items.go  lines 5-7\n")
		test.StrContains(t, stdout, "  4  missing    elsewhere              billing › x.go  (repo \"billing\" is not defined in the tour's `repos`)\n")
		test.StrContains(t, stdout, "  3  moved      purge                  worker › purge.go  line 3")
	})

	T.Run("--json is machine readable", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, true)

		code, stdout, _ := run(t, "resolve", tourPath, "--step", "purge", "--json")
		test.Eq(t, ExitClean, code)

		var steps []map[string]any
		must.NoError(t, json.Unmarshal([]byte(stdout), &steps))
		must.Len(t, 1, steps)
		test.Eq(t, "purge", steps[0]["step"])
		test.Eq(t, "moved", steps[0]["status"])
		test.Eq(t, "anchor-moved", steps[0]["problem"])
		test.Eq(t, "worker", steps[0]["repo"])
	})

	T.Run("an unknown step is an error that says what would work", func(t *testing.T) {
		t.Parallel()

		_, tourPath := crossRepoWorkspace(t, true)

		code, _, stderr := run(t, "resolve", tourPath, "--step", "nope")
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, `no step "nope"`)
		test.StrContains(t, stderr, "a number from 1 to 4")
	})
}

func TestScaffold(T *testing.T) {
	T.Parallel()

	T.Run("writes a tour that validates clean, and never overwrites", func(t *testing.T) {
		t.Parallel()

		dir, err := filepath.EvalSymlinks(t.TempDir())
		must.NoError(t, err)

		a := &application{streams: Streams{Stdout: io.Discard, Stderr: io.Discard}}
		must.NoError(t, a.scaffold(dir, "cache-invalidation"))

		code, stdout, _ := run(t, "validate", "--strict", dir)
		test.Eq(t, ExitClean, code, test.Sprint(stdout))

		written, err := os.ReadFile(filepath.Join(dir, ".tours", "cache-invalidation.tour"))
		must.NoError(t, err)
		test.StrContains(t, string(written), `"title": "Cache invalidation"`)

		test.ErrorContains(t, a.scaffold(dir, "cache-invalidation"), "already exists")
		test.ErrorContains(t, a.scaffold(dir, "Not A Slug"), "not a slug")
	})
}

func TestSchemaAndGuide(T *testing.T) {
	T.Parallel()

	T.Run("print exactly what is embedded", func(t *testing.T) {
		t.Parallel()

		code, stdout, _ := run(t, "schema")
		test.Eq(t, ExitClean, code)
		test.Eq(t, string(schema.TourV1), stdout)

		code, stdout, _ = run(t, "guide")
		test.Eq(t, ExitClean, code)
		test.Eq(t, guide.Markdown, stdout)
	})

	T.Run("the guide's example tour passes the schema", func(t *testing.T) {
		t.Parallel()

		// The guide teaches by example, so the example must be true.
		start := indexAfter(guide.Markdown, "```json\n")
		end := start + indexOf(guide.Markdown[start:], "\n```")
		example := guide.Markdown[start:end]

		dir, err := filepath.EvalSymlinks(t.TempDir())
		must.NoError(t, err)

		path := filepath.Join(dir, ".tours", "example.tour")
		must.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
		must.NoError(t, os.WriteFile(path, []byte(example), 0o600))

		_, stdout, _ := run(t, "validate", path)
		test.StrNotContains(t, stdout, " schema ")
		test.StrNotContains(t, stdout, "json-syntax")
		test.StrContains(t, stdout, "repo-missing")
	})
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}

func indexAfter(s, sub string) int {
	return indexOf(s, sub) + len(sub)
}

func TestDoctor(T *testing.T) {
	T.Parallel()

	T.Run("describes the config, the editors and each tour's repos", func(t *testing.T) {
		t.Parallel()

		top, tourPath := crossRepoWorkspace(t, true)

		code, stdout, stderr := run(t, "doctor", filepath.Join(top, "team-tours"))
		must.Eq(t, ExitClean, code, must.Sprint(stderr))

		for _, want := range []string{
			"version          dev",
			"workspace file   " + filepath.Join(top, "docent.json"),
			"config.json (not present)",
			"library.json",
			"folders          (none)",
			tourPath,
			"api          " + filepath.Join(top, "api") + " (by convention), reading the working tree",
			"worker       " + filepath.Join(top, "elsewhere", "worker-svc") + " (by workspace-mapping)",
			"git              ",
			"webview          WebKit ",
		} {
			test.StrContains(t, stdout, want)
		}
	})

	T.Run("still reports on a folder with no tours", func(t *testing.T) {
		t.Parallel()

		code, stdout, _ := run(t, "doctor", t.TempDir())
		test.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "none found")
	})
}
