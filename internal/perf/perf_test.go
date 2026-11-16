package perf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/primandproper/docent/internal/app"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/gitsrc/gittest"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

// The budgets, on an Apple-silicon laptop. `make test` runs with the race
// detector, which costs several times over, so each is allowed that slack;
// what these guard against is a change that makes something ten times slower.
const (
	raceSlack = 5

	launchBudget   = 1 * time.Second        // launch to the first step, 30 steps across 5 repos (Go's share)
	cachedBudget   = 50 * time.Millisecond  // step navigation, file already read
	uncachedBudget = 150 * time.Millisecond // step navigation, first read
	validateBudget = 2 * time.Second        // `docent validate` on a 50-step tour
)

const (
	repoCount    = 5
	linesPerFile = 600
)

// workspace builds repoCount sibling repositories and a tours repo holding
// one tour of the given number of steps spread across them, stamped at each
// repo's first commit, with a second commit on top so drift has to be
// followed.
func workspace(t *testing.T, steps int) (root, tourPath string) {
	t.Helper()

	top, err := filepath.EvalSymlinks(t.TempDir())
	must.NoError(t, err)

	var repos, stepJSON []string

	for r := range repoCount {
		name := fmt.Sprintf("svc%d", r)
		dir := filepath.Join(top, name)
		must.NoError(t, os.MkdirAll(dir, 0o750))

		repo := gittest.At(t, dir)
		files := map[string]string{}

		for f := range 6 {
			var b strings.Builder
			for line := 1; line <= linesPerFile; line++ {
				if line%50 == 0 {
					fmt.Fprintf(&b, "func Handler%d_%d() {}\n", f, line)
				} else {
					fmt.Fprintf(&b, "// %s file %d line %d\n", name, f, line)
				}
			}

			files[fmt.Sprintf("pkg/file%d.go", f)] = b.String()
		}

		stamp := repo.Commit("initial", files)
		repo.Commit("touch", map[string]string{"pkg/file0.go": "// header\n" + files["pkg/file0.go"]})

		repos = append(repos, fmt.Sprintf(`%q: {"remote": "git@github.com:acme/%s.git", "ref": "main", "stamp": %q}`, name, name, stamp))
	}

	for s := range steps {
		line := 50 * (1 + s%10)
		stepJSON = append(stepJSON, fmt.Sprintf(
			`{"id": "s%d", "description": "d", "repo": "svc%d", "file": "pkg/file%d.go", "pattern": "^func Handler%d_%d\\(", "line": %d}`,
			s, s%repoCount, s%6, s%6, line, line))
	}

	root = filepath.Join(top, "tours")
	tourPath = filepath.Join(root, ".tours", "big.tour")
	must.NoError(t, os.MkdirAll(filepath.Dir(tourPath), 0o750))
	must.NoError(t, os.WriteFile(tourPath, fmt.Appendf(nil,
		`{"docent": 1, "title": "Big", "repos": {%s}, "steps": [%s]}`, strings.Join(repos, ","), strings.Join(stepJSON, ",")), 0o600))

	return root, tourPath
}

func services(t *testing.T, root string) *core.Core {
	t.Helper()

	c, err := core.New(&core.Options{Root: root, UserConfigPath: filepath.Join(root, "no-config.json")})
	must.NoError(t, err)
	t.Cleanup(c.Close)

	return c
}

func within(t *testing.T, what string, budget, took time.Duration) {
	t.Helper()

	t.Logf("%s: %s (budget %s)", what, took.Round(time.Millisecond), budget)
	test.Less(t, budget*raceSlack, took, test.Sprintf("%s took %s, over its %s budget", what, took, budget))
}

// These tests share nothing, but they are not parallel: each is a stopwatch,
// and running them at once would have them time each other.

//nolint:paralleltest // see above
func TestBudget_LaunchToFirstStep(t *testing.T) {
	root, tourPath := workspace(t, 30)

	started := time.Now()

	service := app.NewService(root, services(t, root), nil, nil, nil)

	tours, err := service.ListTours(t.Context())
	must.NoError(t, err)
	must.Len(t, 1, tours)

	view, err := service.OpenTour(t.Context(), tourPath)
	must.NoError(t, err)
	must.Len(t, 30, view.Steps)

	step, err := service.Step(t.Context(), tourPath, 0)
	must.NoError(t, err)
	must.NotEq(t, "", step.Content)

	within(t, "launch to first step", launchBudget, time.Since(started))
	test.Eq(t, 0, tours[0].Errors)
}

//nolint:paralleltest // see above
func TestBudget_StepNavigation(t *testing.T) {
	root, tourPath := workspace(t, 30)
	service := app.NewService(root, services(t, root), nil, nil, nil)

	_, err := service.OpenTour(t.Context(), tourPath)
	must.NoError(t, err)

	// OpenTour has resolved every step once, so the refs are known and the
	// git helper processes are running: this is navigating a tour that is open.
	var worst time.Duration

	for index := range 30 {
		started := time.Now()

		step, stepErr := service.Step(t.Context(), tourPath, index)
		must.NoError(t, stepErr)
		must.NotEq(t, "", step.Content)

		worst = max(worst, time.Since(started))
	}

	within(t, "slowest step of an open tour", cachedBudget, worst)
}

//nolint:paralleltest // see above
func TestBudget_FirstStepOfAColdTour(t *testing.T) {
	root, tourPath := workspace(t, 30)
	c := services(t, root)

	parsed, _, err := c.Store.Load(t.Context(), tourPath)
	must.NoError(t, err)

	// Nothing is warm: no refs resolved, no git helper running.
	started := time.Now()

	step, err := c.Walker.Step(t.Context(), parsed, 7)
	must.NoError(t, err)
	must.NotNil(t, step.Anchor.Content)

	within(t, "first step, cold", uncachedBudget, time.Since(started))
}

//nolint:paralleltest // see above
func TestBudget_Validate(t *testing.T) {
	root, tourPath := workspace(t, 50)
	c := services(t, root)

	started := time.Now()

	diags, err := c.Validator.Tour(t.Context(), tourPath)
	must.NoError(t, err)

	within(t, "validate, 50 steps", validateBudget, time.Since(started))

	// Steps in file0 moved by a line when its header was added; nothing is broken.
	for i := range diags {
		test.Eq(t, "anchor-moved", string(diags[i].Code))
	}
}
