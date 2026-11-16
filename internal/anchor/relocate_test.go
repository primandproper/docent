package anchor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/gitsrc/gittest"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const stamped = `package worker

import "time"

// pollWorker asks the queue for work on a timer.
func pollWorker(q Queue) {
	for {
		job := q.Next()
		handle(job)
		time.Sleep(time.Second)
	}
}

func handle(job Job) {
	job.Run()
}
`

// driftRepo commits the stamped file, then whatever change the test makes,
// and resolves an anchor on pollWorker's body (lines 6 to 12) across the two.
func resolveAcross(t *testing.T, spec Spec, change func(repo *gittest.Repo)) *Resolution {
	t.Helper()

	repo := gittest.New(t)
	stamp := repo.Commit("add worker", map[string]string{"worker.go": stamped})
	change(repo)
	tip := repo.Git("rev-parse", "HEAD")

	git := gitsrc.NewGit(nil, nil)
	t.Cleanup(git.Close)

	res, err := NewResolver(git, nil, nil).Resolve(t.Context(), &Request{
		Root:  repo.Dir,
		Ref:   tip,
		File:  "worker.go",
		Spec:  spec,
		Drift: &Drift{Stamp: stamp, Tip: tip},
	})
	must.NoError(t, err)

	return res
}

var bodySpec = Spec{Pattern: `^func pollWorker`, Line: 6, Selection: sel(6, 1, 12, 2)}

func TestResolver_Drift(T *testing.T) {
	T.Parallel()

	T.Run("code that shifted but was not touched is moved, exactly", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, bodySpec, func(repo *gittest.Repo) {
			edited := strings.Replace(stamped, `import "time"`, "import (\n\t\"log\"\n\t\"time\"\n)\n\nvar _ = log.Println", 1)
			edited += "\nfunc extra() {}\n"
			repo.Commit("add logging import and a helper", map[string]string{"worker.go": edited})
		})

		test.Eq(t, StatusMoved, res.Location.Status)
		test.Eq(t, 5, res.Location.Offset)
		test.Eq(t, Range{StartLine: 11, StartCol: 1, EndLine: 17, EndCol: 2}, res.Location.Range)
		test.StrContains(t, res.Location.Detail, "moved +5 lines since")
		test.Nil(t, res.Then)
		test.Eq(t, "worker.go", res.File)
	})

	T.Run("changes elsewhere in the file leave the anchor ok", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, bodySpec, func(repo *gittest.Repo) {
			repo.Commit("change handle", map[string]string{"worker.go": strings.Replace(stamped, "job.Run()", "job.Run()\n\tjob.Done()", 1)})
		})

		test.Eq(t, StatusOK, res.Location.Status)
		test.Eq(t, Range{StartLine: 6, StartCol: 1, EndLine: 12, EndCol: 2}, res.Location.Range)
	})

	T.Run("a renamed file is followed", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, bodySpec, func(repo *gittest.Repo) {
			repo.Git("mv", "worker.go", "poller.go")
			repo.Commit("rename", nil)
		})

		test.Eq(t, StatusMoved, res.Location.Status)
		test.Eq(t, "poller.go", res.File)
		test.Eq(t, 6, res.Location.Range.StartLine)
		test.StrContains(t, res.Location.Detail, `file moved to "poller.go"`)
		test.StrContains(t, string(res.Content), "func pollWorker")
	})

	T.Run("code edited in place is changed, found again by its pattern", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, bodySpec, func(repo *gittest.Repo) {
			edited := "// Copyright\n" + strings.Replace(stamped, "time.Sleep(time.Second)", "time.Sleep(backoff())", 1)
			repo.Commit("back off", map[string]string{"worker.go": edited})
		})

		test.Eq(t, StatusChanged, res.Location.Status)
		test.Eq(t, diag.CodeContentChanged, res.Location.Problem)
		test.Eq(t, Range{StartLine: 7, StartCol: 1, EndLine: 13, EndCol: 2}, res.Location.Range)
		must.NotNil(t, res.Then)
		test.Eq(t, 6, res.Then.StartLine)
		test.Len(t, 7, res.Then.Lines)
		test.StrContains(t, res.Then.Lines[4], "time.Sleep(time.Second)")
	})

	T.Run("edited code with no pattern is found by similarity", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, Spec{Line: 6, Selection: sel(6, 1, 12, 2)}, func(repo *gittest.Repo) {
			edited := strings.Replace(stamped, "func pollWorker(q Queue) {", "func pollWorker(q Queue, every time.Duration) {", 1)
			edited = strings.Replace(edited, "time.Sleep(time.Second)", "time.Sleep(every)", 1)
			repo.Commit("configurable interval", map[string]string{"worker.go": "package worker\n\n// moved down\n\n" + strings.TrimPrefix(edited, "package worker\n")})
		})

		test.Eq(t, StatusChanged, res.Location.Status)
		test.Eq(t, 9, res.Location.Range.StartLine)
		test.Eq(t, 15, res.Location.Range.EndLine)
	})

	T.Run("removed code is missing, with what it was and the commit that removed it", func(t *testing.T) {
		t.Parallel()

		var removal string

		res := resolveAcross(t, bodySpec, func(repo *gittest.Repo) {
			repo.Commit("unrelated", map[string]string{"other.go": "package worker\n"})
			removal = repo.Commit("replace polling worker with event stream", map[string]string{
				"worker.go": "package worker\n\nfunc subscribe(s Stream) {\n\ts.On(handle)\n}\n\nfunc handle(job Job) {\n\tjob.Run()\n}\n",
			})
		})

		test.Eq(t, StatusMissing, res.Location.Status)
		test.Eq(t, diag.CodeCodeRemoved, res.Location.Problem)
		must.NotNil(t, res.Then)
		test.StrContains(t, res.Then.Lines[0], "func pollWorker")
		must.NotNil(t, res.Removed)
		test.Eq(t, removal, res.Removed.SHA)
		test.Eq(t, "replace polling worker with event stream", res.Removed.Subject)
	})

	T.Run("a deleted file is missing, with the commit that deleted it", func(t *testing.T) {
		t.Parallel()

		var removal string

		res := resolveAcross(t, bodySpec, func(repo *gittest.Repo) {
			must.NoError(t, os.Remove(filepath.Join(repo.Dir, "worker.go")))
			removal = repo.Commit("delete the worker", nil)
		})

		test.Eq(t, StatusMissing, res.Location.Status)
		must.NotNil(t, res.Removed)
		test.Eq(t, removal, res.Removed.SHA)
		must.NotNil(t, res.Then)
		test.Nil(t, res.Content)
	})

	T.Run("an anchor that never resolved at the stamp is just located at the tip", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, Spec{Pattern: `^func subscribe`}, func(repo *gittest.Repo) {
			repo.Commit("add subscribe", map[string]string{"worker.go": stamped + "\nfunc subscribe() {}\n"})
		})

		test.Eq(t, StatusOK, res.Location.Status)
		test.Eq(t, 18, res.Location.Range.StartLine)
	})

	T.Run("a whole-file anchor only has to follow the file", func(t *testing.T) {
		t.Parallel()

		res := resolveAcross(t, Spec{}, func(repo *gittest.Repo) {
			repo.Commit("rewrite", map[string]string{"worker.go": "package worker\n"})
		})

		test.Eq(t, StatusOK, res.Location.Status)
		test.Eq(t, Range{}, res.Location.Range)
	})
}

func TestResolver_ContentHash(T *testing.T) {
	T.Parallel()

	lines := SplitLines([]byte(stamped))
	hash := Hash(lines, Range{StartLine: 6, EndLine: 12})

	T.Run("ignores whitespace and notices real edits", func(t *testing.T) {
		t.Parallel()

		test.StrHasPrefix(t, HashPrefix, hash)

		reindented := SplitLines([]byte(strings.ReplaceAll(stamped, "\t", "    ")))
		test.Eq(t, hash, Hash(reindented, Range{StartLine: 6, EndLine: 12}))

		edited := SplitLines([]byte(strings.Replace(stamped, "time.Second", "time.Minute", 1)))
		test.NotEq(t, hash, Hash(edited, Range{StartLine: 6, EndLine: 12}))

		test.Eq(t, "", Hash(lines, Range{StartLine: 90, EndLine: 91}))
	})

	T.Run("an edit that leaves the anchor in place is changed", func(t *testing.T) {
		t.Parallel()

		edited := strings.Replace(stamped, "time.Second", "time.Minute", 1)
		source := gitsrc.Memory{gitsrc.Key("/r", "", "same.go"): []byte(stamped), gitsrc.Key("/r", "", "edited.go"): []byte(edited)}
		r := NewResolver(source, nil, nil)

		res, err := r.Resolve(t.Context(), &Request{Root: "/r", File: "same.go", Spec: bodySpec, ContentHash: hash})
		must.NoError(t, err)
		test.Eq(t, StatusOK, res.Location.Status)

		res, err = r.Resolve(t.Context(), &Request{Root: "/r", File: "edited.go", Spec: bodySpec, ContentHash: hash})
		must.NoError(t, err)
		test.Eq(t, StatusChanged, res.Location.Status)
		test.Eq(t, diag.CodeContentChanged, res.Location.Problem)
	})
}

func TestCarry(T *testing.T) {
	T.Parallel()

	T.Run("sums the hunks above a range and notices ones inside it", func(t *testing.T) {
		t.Parallel()

		r := Range{StartLine: 10, EndLine: 20}

		offset, touched := carry([]gitsrc.Hunk{
			{OldStart: 2, OldCount: 0, NewStart: 3, NewCount: 4},   // insert 4 above
			{OldStart: 5, OldCount: 3, NewStart: 10, NewCount: 1},  // shrink by 2 above
			{OldStart: 30, OldCount: 2, NewStart: 32, NewCount: 9}, // below: irrelevant
		}, r)
		test.Eq(t, 2, offset)
		test.False(t, touched)

		// Inserting directly above the first line, or directly below the last,
		// doesn't touch the range.
		_, touched = carry([]gitsrc.Hunk{{OldStart: 9, OldCount: 0, NewCount: 1}, {OldStart: 20, OldCount: 0, NewCount: 1}}, r)
		test.False(t, touched)

		for _, inside := range []gitsrc.Hunk{
			{OldStart: 10, OldCount: 1, NewCount: 1},
			{OldStart: 20, OldCount: 1, NewCount: 0},
			{OldStart: 8, OldCount: 3, NewCount: 3},
			{OldStart: 15, OldCount: 0, NewCount: 2},
		} {
			_, touched = carry([]gitsrc.Hunk{inside}, r)
			test.True(t, touched, test.Sprintf("%+v", inside))
		}
	})
}
