package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/gitsrc/gittest"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const workerV1 = "package worker\n\nfunc pollWorker() {\n\tpoll()\n}\n\nfunc handle() {}\n"

// stampedWorkspace is a tours folder beside a real `worker` repository, with
// a tour of two anchors that both resolve at the repo's first commit.
func stampedWorkspace(t *testing.T) (repo *gittest.Repo, tourPath string) {
	t.Helper()

	top, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	must.NoError(t, os.MkdirAll(filepath.Join(top, "worker"), 0o750))
	repo = gittest.At(t, filepath.Join(top, "worker"))
	repo.Commit("add worker", map[string]string{"worker.go": workerV1})

	tourPath = filepath.Join(top, "tours", ".tours", "worker.tour")
	must.NoError(t, os.MkdirAll(filepath.Dir(tourPath), 0o750))
	must.NoError(t, os.WriteFile(tourPath, []byte(`{
  "docent": 1, "title": "The worker",
  "repos": {"worker": {"remote": "git@github.com:acme/worker.git", "ref": "main"}},
  "generatedBy": {"agent": "test", "at": "2026-11-16T10:00:00Z"},
  "steps": [
    {"id": "poll", "description": "d", "repo": "worker", "file": "worker.go", "pattern": "^func pollWorker", "line": 3,
      "selection": {"start": {"line": 3, "character": 1}, "end": {"line": 5, "character": 2}}},
    {"id": "handle", "description": "d", "repo": "worker", "file": "worker.go", "pattern": "^func handle", "line": 7}
  ]
}`), 0o600))

	return repo, tourPath
}

func readTour(t *testing.T, path string) map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	must.NoError(t, err)

	var doc map[string]any
	must.NoError(t, json.Unmarshal(data, &doc))

	return doc
}

func stampOf(t *testing.T, path string) string {
	t.Helper()

	repos, _ := readTour(t, path)["repos"].(map[string]any)
	worker, _ := repos["worker"].(map[string]any)
	stamp, _ := worker["stamp"].(string)

	return stamp
}

func TestStamp(T *testing.T) {
	T.Parallel()

	T.Run("records each repo's commit, and hashes only when asked", func(t *testing.T) {
		t.Parallel()

		repo, tourPath := stampedWorkspace(t)
		head := repo.Git("rev-parse", "HEAD")

		code, stdout, stderr := run(t, "stamp", tourPath)
		must.Eq(t, ExitClean, code, must.Sprint(stderr))
		test.Eq(t, "worker       (none) -> "+head[:7]+"\n", stdout)
		test.Eq(t, head, stampOf(t, tourPath))

		written, err := os.ReadFile(tourPath)
		must.NoError(t, err)
		test.StrNotContains(t, string(written), "contentHash")
		// The file comes back in the canonical layout.
		test.StrContains(t, string(written), `"selection": {`+"\n"+`        "start": { "line": 3, "character": 1 },`)

		code, _, _ = run(t, "stamp", "--hashes", tourPath)
		must.Eq(t, ExitClean, code)

		written, err = os.ReadFile(tourPath)
		must.NoError(t, err)
		test.Eq(t, 2, strings.Count(string(written), `"contentHash": "sha256:`))

		code, stdout, _ = run(t, "validate", "--strict", tourPath)
		test.Eq(t, ExitClean, code, test.Sprint(stdout))

		// Moving the stamp without --hashes clears them: a hash describes the
		// code at its stamp.
		repo.Commit("unrelated", map[string]string{"other.go": "package worker\n"})

		code, _, _ = run(t, "stamp", tourPath)
		must.Eq(t, ExitClean, code)

		written, err = os.ReadFile(tourPath)
		must.NoError(t, err)
		test.StrNotContains(t, string(written), "contentHash")
	})

	T.Run("refuses while anchors have drifted, unless forced", func(t *testing.T) {
		t.Parallel()

		repo, tourPath := stampedWorkspace(t)

		code, _, _ := run(t, "stamp", tourPath)
		must.Eq(t, ExitClean, code)

		first := stampOf(t, tourPath)

		repo.Commit("add a header", map[string]string{"worker.go": "// Package worker polls.\n" + workerV1})

		code, _, stderr := run(t, "stamp", tourPath)
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, "2 of this tour's anchors don't resolve cleanly")
		test.StrContains(t, stderr, "moved      poll")
		test.Eq(t, first, stampOf(t, tourPath))

		code, _, _ = run(t, "stamp", "--force", tourPath)
		test.Eq(t, ExitClean, code)
		test.NotEq(t, first, stampOf(t, tourPath))
	})

	T.Run("needs a docent tour whose repos are here", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": cleanTour})

		code, _, stderr := run(t, "stamp", filepath.Join(root, ".tours", "a.tour"))
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, "plain CodeTour file")

		_, tourPath := crossRepoWorkspace(t, false)

		code, _, stderr = run(t, "stamp", "--force", tourPath)
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, "has no commit to stamp")
	})
}

func TestReview(T *testing.T) {
	T.Parallel()

	T.Run("appends a review, stamps and hashes", func(t *testing.T) {
		t.Parallel()

		repo, tourPath := stampedWorkspace(t)
		head := repo.Git("rev-parse", "HEAD")

		code, stdout, stderr := run(t, "review", tourPath, "--name", "Jeffrey")
		must.Eq(t, ExitClean, code, must.Sprint(stderr))
		test.StrContains(t, stdout, "reviewed by Jeffrey\n")

		doc := readTour(t, tourPath)
		reviews, _ := doc["reviewedBy"].([]any)
		must.Len(t, 1, reviews)

		review, _ := reviews[0].(map[string]any)
		test.Eq[any](t, "Jeffrey", review["name"])
		test.Eq[any](t, head, review["ref"])
		test.NotEq[any](t, "", review["at"])
		test.Eq(t, head, stampOf(t, tourPath))

		// The agent's provenance stays: a reviewed draft is still a draft someone wrote.
		test.NotNil(t, doc["generatedBy"])

		code, _, _ = run(t, "review", tourPath, "--name", "Sam")
		must.Eq(t, ExitClean, code)

		reviews, _ = readTour(t, tourPath)["reviewedBy"].([]any)
		test.Len(t, 2, reviews)
	})

	T.Run("needs a name", func(t *testing.T) {
		t.Parallel()

		_, tourPath := stampedWorkspace(t)

		code, _, stderr := run(t, "review", tourPath)
		test.Eq(t, ExitErrors, code)
		test.StrContains(t, stderr, "--name is required")
	})
}

func TestFmt(T *testing.T) {
	T.Parallel()

	T.Run("rewrites to the canonical layout without changing values", func(t *testing.T) {
		t.Parallel()

		root := workspace(t, map[string]string{"a.tour": cleanTour})
		path := filepath.Join(root, ".tours", "a.tour")
		before := readTour(t, path)

		code, stdout, _ := run(t, "fmt", "--check", root)
		test.Eq(t, ExitErrors, code)
		test.Eq(t, path+"\n", stdout)

		code, stdout, _ = run(t, "fmt", root)
		test.Eq(t, ExitClean, code)
		test.Eq(t, path+"\n", stdout)
		test.Eq(t, before, readTour(t, path))

		written, err := os.ReadFile(path)
		must.NoError(t, err)
		test.Eq(t, `{
  "title": "Clean",
  "steps": [
    { "description": "d", "file": "a.go", "line": 3, "pattern": "^func A" }
  ]
}
`, string(written))

		code, stdout, _ = run(t, "fmt", "--check", root)
		test.Eq(t, ExitClean, code)
		test.Eq(t, "", stdout)
	})
}

func TestStale(T *testing.T) {
	T.Parallel()

	T.Run("a freshly stamped tour is not stale", func(t *testing.T) {
		t.Parallel()

		_, tourPath := stampedWorkspace(t)

		code, _, _ := run(t, "stamp", tourPath)
		must.Eq(t, ExitClean, code)

		code, stdout, _ := run(t, "stale", tourPath)
		test.Eq(t, ExitClean, code)
		test.Eq(t, "0 stale tours\n", stdout)
	})

	T.Run("lists drifted anchors with the diff since the stamp", func(t *testing.T) {
		t.Parallel()

		repo, tourPath := stampedWorkspace(t)

		code, _, _ := run(t, "stamp", tourPath)
		must.Eq(t, ExitClean, code)

		repo.Commit("replace polling worker with event stream", map[string]string{
			"worker.go": "package worker\n\nfunc subscribe() {\n\ton(handle)\n}\n\nfunc handle() {}\n",
		})

		code, stdout, _ := run(t, "stale", tourPath)
		test.Eq(t, ExitClean, code)
		test.StrContains(t, stdout, "1 anchor doesn't resolve cleanly")
		test.StrContains(t, stdout, "step poll  missing  worker › worker.go")
		test.StrContains(t, stdout, "-func pollWorker() {")
		test.StrContains(t, stdout, "+func subscribe() {")
		test.StrContains(t, stdout, "1 stale tour\n")

		code, stdout, _ = run(t, "stale", "--json", tourPath)
		test.Eq(t, ExitClean, code)

		var report struct {
			Tours []struct {
				Reasons []string
				Repos   []struct {
					Name   string
					Behind int
				}
				Anchors []struct{ Step, Status, Diff string }
			}
		}
		must.NoError(t, json.Unmarshal([]byte(stdout), &report))
		must.Len(t, 1, report.Tours)
		test.Eq(t, 1, report.Tours[0].Repos[0].Behind)
		must.Len(t, 2, report.Tours[0].Anchors)
		test.Eq(t, "missing", report.Tours[0].Anchors[0].Status)
		test.Eq(t, "ok", report.Tours[0].Anchors[1].Status)
		test.StrContains(t, report.Tours[0].Anchors[1].Diff, "+func subscribe() {")
	})

	T.Run("a stamp too many commits behind is stale even when anchors hold", func(t *testing.T) {
		t.Parallel()

		repo, tourPath := stampedWorkspace(t)

		code, _, _ := run(t, "stamp", tourPath)
		must.Eq(t, ExitClean, code)

		repo.Commit("one", map[string]string{"x.go": "package worker\n"})
		repo.Commit("two", map[string]string{"y.go": "package worker\n"})

		_, stdout, _ := run(t, "stale", tourPath)
		test.Eq(t, "0 stale tours\n", stdout)

		_, stdout, _ = run(t, "stale", "--max-commits", "1", tourPath)
		test.StrContains(t, stdout, "worker is 2 commits behind main")
	})
}

func TestWorkingTreeFlag(T *testing.T) {
	T.Parallel()

	T.Run("validate and resolve read the ref by default and the disk on request", func(t *testing.T) {
		t.Parallel()

		repo, tourPath := stampedWorkspace(t)
		repo.Write(map[string]string{"worker.go": "package worker\n\n// uncommitted\n\nfunc pollWorker() {\n\tpoll()\n}\n\nfunc handle() {}\n"})

		code, _, _ := run(t, "validate", "--strict", tourPath)
		test.Eq(t, ExitClean, code)

		code, stdout, _ := run(t, "validate", "--strict", "--working-tree", tourPath)
		test.Eq(t, ExitWarnings, code)
		test.StrContains(t, stdout, "anchor-moved")

		_, stdout, _ = run(t, "resolve", tourPath, "--step", "poll", "--working-tree")
		test.StrContains(t, stdout, "status: moved  lines 5-7")
	})
}
