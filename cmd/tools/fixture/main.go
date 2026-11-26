// Command fixture builds a workspace of real git repositories from the files
// under frontend/e2e/testdata, runs the app's real service against it, and
// writes everything the frontend would have asked as one JSON file. The
// browser tests load that file in place of the Wails bindings, so they
// exercise the real frontend against real answers without a webview.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/primandproper/docent/internal/app"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/flags"
	"github.com/primandproper/docent/internal/flags/flagstest"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/links"
	"github.com/primandproper/docent/internal/orderedjson"
	"github.com/primandproper/docent/internal/secrets"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/visual"
	"github.com/primandproper/docent/internal/workspace"
)

type (
	fixture struct {
		_     struct{}                   `json:"-"`
		Views map[string]*app.TourView   `json:"views"`
		Steps map[string][]*app.StepView `json:"steps"`
		// Picked holds each tour again for every choice of every variable that
		// has choices, keyed "path|name=value".
		Picked map[string]picked `json:"picked"`
		// Listings and Files are what the file drawer would be told about
		// each tour's first step with code: every directory, and every file
		// small enough to be worth recording.
		Listings map[string]map[string][]gitsrc.Entry `json:"listings"`
		Files    map[string]map[string]*app.FileView  `json:"files"`
		// LinkFences holds what the service made of each docent-links fence,
		// keyed by tour path and then by the fence's body.
		LinkFences map[string]map[string][]links.Card `json:"linkFences"`
		// ReviewProblems holds what stops each tour being reviewed once every
		// step has been read, by tour path: "" when nothing does.
		ReviewProblems map[string]string `json:"reviewProblems"`
		// Flags holds each flag card's status, keyed by tour path (or
		// "path|name=value" for a picked variable) and then "flag|project".
		Flags   map[string]map[string]flags.Status `json:"flags"`
		Folders []app.Folder                       `json:"folders"`
		Tours   []app.TourSummary                  `json:"tours"`
	}

	picked struct {
		_     struct{}        `json:"-"`
		View  *app.TourView   `json:"view"`
		Steps []*app.StepView `json:"steps"`
	}
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: fixture <testdata dir> <output.json>")
		os.Exit(2)
	}

	if err := run(context.Background(), os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintf(os.Stderr, "fixture: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, testdata, output string) error {
	root, err := build(ctx, testdata)
	if err != nil {
		return fmt.Errorf("building the workspace: %w", err)
	}

	// A second folder of tours, beside the first, for the app to have more
	// than one open: a handbook whose tour links into the workspace's.
	handbook := filepath.Join(filepath.Dir(root), "handbook")
	if err = os.CopyFS(handbook, os.DirFS(filepath.Join(testdata, "handbook"))); err != nil {
		return fmt.Errorf("copying the handbook: %w", err)
	}

	out := fixture{
		Folders:        []app.Folder{},
		Tours:          []app.TourSummary{},
		Views:          map[string]*app.TourView{},
		Steps:          map[string][]*app.StepView{},
		Picked:         map[string]picked{},
		LinkFences:     map[string]map[string][]links.Card{},
		Flags:          map[string]map[string]flags.Status{},
		ReviewProblems: map[string]string{},
		Listings:       map[string]map[string][]gitsrc.Entry{},
		Files:          map[string]map[string]*app.FileView{},
	}

	// Each folder is recorded by a service of its own that knows the other
	// folder's tours, which is how the app checks links between folders.
	if err = recordFolder(ctx, root, handbook, &out); err != nil {
		return err
	}

	if err = recordFolder(ctx, handbook, root, &out); err != nil {
		return err
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding fixture: %w", err)
	}

	if err = os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err = os.WriteFile(output, encoded, 0o600); err != nil {
		return fmt.Errorf("writing fixture: %w", err)
	}

	return nil
}

// recordFolder records everything the service says about one folder's tours,
// with the tours of the folder beside it there to be linked to.
func recordFolder(ctx context.Context, root, beside string, out *fixture) error {
	// Flag cards are answered by a stand-in for LaunchDarkly, through the
	// real provider, so the fixture holds what the app would really be sent.
	launchDarkly := flagstest.NewServer()
	defer launchDarkly.Close()

	services, err := core.New(&core.Options{
		Root: root,
		// The user's own config must not leak into a recorded fixture, nor
		// their credential store or environment.
		UserConfigPath: filepath.Join(root, "no-user-config.json"),
		Secrets:        &secrets.Memory{},
		Getenv: func(name string) string {
			return map[string]string{workspace.ExperimentalEnvVar: workspace.ExperimentIntegrations}[name]
		},
		Flags: flags.NewLaunchDarkly(&flags.LaunchDarklyConfig{
			Host:      launchDarkly.URL,
			Transport: launchDarkly.Client().Transport,
			Token:     func(context.Context) (string, error) { return flagstest.Token, nil },
		}),
		Elsewhere: func(context.Context) ([]string, error) { return tour.Discover(beside) },
	})
	if err != nil {
		return fmt.Errorf("setting up %s: %w", root, err)
	}
	defer services.Close()

	service := app.NewService(root, services, nil, nil, nil)

	out.Folders = append(out.Folders, service.Folders()...)

	tours, err := service.ListTours(ctx)
	if err != nil {
		return fmt.Errorf("listing tours: %w", err)
	}

	out.Tours = append(out.Tours, tours...)

	for i := range tours {
		path := tours[i].Path
		if tours[i].Unreadable {
			continue
		}

		view, steps, recErr := record(ctx, service, path)
		if recErr != nil {
			return recErr
		}

		out.Views[path], out.Steps[path] = view, steps

		// The name on offer is this machine's git user, which isn't the
		// fixture's business. The recorded backend supplies its own.
		view.Review.Name = ""

		for index := range steps {
			state, readErr := service.MarkRead(ctx, path, index)
			if readErr != nil {
				return fmt.Errorf("marking step %d of %q read: %w", index, path, readErr)
			}

			out.ReviewProblems[path] = state.Blocker
		}
		out.LinkFences[path] = map[string][]links.Card{}
		out.Listings[path] = map[string][]gitsrc.Entry{}
		out.Files[path] = map[string]*app.FileView{}

		for index, step := range steps {
			if step.Kind == "file" && step.Content != "" && len(step.Content) < 100_000 {
				if err = recordTree(ctx, service, path, index, "", out.Listings[path], out.Files[path]); err != nil {
					return err
				}

				break
			}
		}

		for _, step := range steps {
			fences := visual.Fences(step.Body)
			for i := range fences {
				if fences[i].Lang != visual.LinksFence {
					continue
				}

				// The frontend reads a fence's text from the rendered page,
				// where it ends with a newline.
				body := fences[i].Body + "\n"

				if out.LinkFences[path][body], err = service.ExpandLinks(ctx, path, body); err != nil {
					return fmt.Errorf("expanding a links fence of %q: %w", path, err)
				}
			}
		}

		if out.Flags[path], err = recordFlags(ctx, service, path, steps, out.LinkFences[path], launchDarkly.URL); err != nil {
			return err
		}

		for v := range view.Vars {
			name := view.Vars[v].Name

			for _, choice := range view.Vars[v].Choices {
				if _, err = service.SetVar(ctx, path, name, choice); err != nil {
					return fmt.Errorf("picking %s=%s: %w", name, choice, err)
				}

				pickedView, pickedSteps, pickErr := record(ctx, service, path)
				if pickErr != nil {
					return pickErr
				}

				out.Picked[path+"|"+name+"="+choice] = picked{View: pickedView, Steps: pickedSteps}

				if out.Flags[path+"|"+name+"="+choice], err = recordFlags(ctx, service, path, pickedSteps, out.LinkFences[path], launchDarkly.URL); err != nil {
					return err
				}

				if _, err = service.SetVar(ctx, path, name, ""); err != nil {
					return fmt.Errorf("clearing %s: %w", name, err)
				}
			}
		}
	}

	return nil
}

// build copies testdata/workspace somewhere stable outside this repository
// and gives it history: api and worker become git repositories, the tours are
// stamped at their first commits, and worker then moves on, so the tours have
// drift to show.
func build(ctx context.Context, testdata string) (string, error) {
	// A fixed location keeps the paths in the fixture the same from run to run.
	root := filepath.Join(os.TempDir(), "docent-e2e", "workspace")

	if err := os.RemoveAll(filepath.Dir(root)); err != nil {
		return "", fmt.Errorf("clearing %s: %w", root, err)
	}

	if err := os.CopyFS(root, os.DirFS(filepath.Join(testdata, "workspace"))); err != nil {
		return "", fmt.Errorf("copying the workspace: %w", err)
	}

	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", root, err)
	}

	// A file long enough to show whether the code pane really only draws
	// what is on screen. Generated here: nobody needs to review 20,000 lines.
	var huge strings.Builder

	huge.WriteString("package big\n\n")

	for line := 3; line <= 20000; line++ {
		if line%40 == 0 {
			fmt.Fprintf(&huge, "func Generated%d() int { return %d }\n", line, line)
		} else {
			fmt.Fprintf(&huge, "// line %d of a very long file, with enough text on it to be worth highlighting\n", line)
		}
	}

	if err = os.MkdirAll(filepath.Join(root, "big"), 0o750); err != nil {
		return "", fmt.Errorf("creating the long file: %w", err)
	}

	if err = os.WriteFile(filepath.Join(root, "big", "huge.go"), []byte(huge.String()), 0o600); err != nil {
		return "", fmt.Errorf("writing the long file: %w", err)
	}

	stamps := map[string]string{}

	for _, repo := range []string{"api", "worker"} {
		dir := filepath.Join(root, repo)

		for _, args := range [][]string{
			{"init", "-q", "-b", "main"},
			{"remote", "add", "origin", "git@github.com:acme/" + repo + ".git"},
			{"add", "-A"},
			{"commit", "-q", "-m", "initial " + repo},
		} {
			if _, err = git(ctx, dir, args...); err != nil {
				return "", err
			}
		}

		if stamps[repo], err = git(ctx, dir, "rev-parse", "HEAD"); err != nil {
			return "", err
		}
	}

	if err = stampTours(root, stamps); err != nil {
		return "", err
	}

	worker := filepath.Join(root, "worker")

	if err = copyOver(filepath.Join(testdata, "worker-v2"), worker); err != nil {
		return "", fmt.Errorf("applying worker-v2: %w", err)
	}

	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "use timers/promises for sleep, and log failed purges"}} {
		if _, err = git(ctx, worker, args...); err != nil {
			return "", err
		}
	}

	return root, nil
}

// copyOver copies the files under src into dst, replacing any that exist.
// (os.CopyFS refuses to overwrite.)
func copyOver(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)
		if err = os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
			return err
		}

		return os.WriteFile(target, data, 0o600)
	})
}

// stampTours records the first commits as the stamps of every tour that names
// these repos.
func stampTours(root string, stamps map[string]string) error {
	paths, err := tour.Discover(root)
	if err != nil {
		return fmt.Errorf("finding tours: %w", err)
	}

	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return fmt.Errorf("reading %s: %w", path, readErr)
		}

		raw, parseErr := orderedjson.Parse(data)
		if parseErr != nil || raw.Get("repos") == nil {
			continue
		}

		for name, sha := range stamps {
			if raw.Get("repos").Get(name) != nil {
				tour.SetStamp(raw, name, sha)
			}
		}

		if err = tour.Write(path, raw); err != nil {
			return err
		}
	}

	return nil
}

// git runs a git command with a fixed identity and clock, so the commits it
// makes have the same SHAs every time.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
		"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
		"GIT_AUTHOR_DATE=2026-03-01T12:00:00Z", "GIT_COMMITTER_DATE=2026-03-01T12:00:00Z",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s in %s: %w: %s", strings.Join(args, " "), dir, err, out)
	}

	return strings.TrimSpace(string(out)), nil
}

// record opens a tour and resolves every step of it.
func record(ctx context.Context, service *app.Service, path string) (*app.TourView, []*app.StepView, error) {
	view, err := service.OpenTour(ctx, path)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %q: %w", path, err)
	}

	steps := make([]*app.StepView, 0, len(view.Tour.Steps))

	for index := range view.Tour.Steps {
		step, stepErr := service.Step(ctx, path, index)
		if stepErr != nil {
			return nil, nil, fmt.Errorf("resolving step %d of %q: %w", index, path, stepErr)
		}

		steps = append(steps, step)
	}

	return view, steps, nil
}

// recordFlags records the status of every flag card a tour has, on its steps
// and in its links fences, as the variables stand now. The stand-in's address
// changes from run to run, so LaunchDarkly's own is written in its place.
func recordFlags(ctx context.Context, service *app.Service, path string, steps []*app.StepView, fences map[string][]links.Card, serverURL string) (map[string]flags.Status, error) {
	out := map[string]flags.Status{}

	ask := func(cards []links.Card) error {
		for i := range cards {
			if cards[i].Flag == "" {
				continue
			}

			status, err := service.FlagStatus(ctx, path, cards[i].Flag, cards[i].Project, false)
			if err != nil {
				return fmt.Errorf("reading flag %q of %q: %w", cards[i].Flag, path, err)
			}

			status.URL = strings.Replace(status.URL, serverURL, flags.LaunchDarklyDefaultHost, 1)
			out[cards[i].Flag+"|"+cards[i].Project] = status
		}

		return nil
	}

	for _, step := range steps {
		if err := ask(step.Links); err != nil {
			return nil, err
		}
	}

	for _, cards := range fences {
		if err := ask(cards); err != nil {
			return nil, err
		}
	}

	return out, nil
}

// recordTree walks the repo a step's code is in, the way the file drawer
// would, recording every listing and file.
func recordTree(ctx context.Context, service *app.Service, path string, index int, dir string, listings map[string][]gitsrc.Entry, files map[string]*app.FileView) error {
	entries, err := service.Browse(ctx, path, index, -1, dir)
	if err != nil {
		return fmt.Errorf("browsing %q of %q: %w", dir, path, err)
	}

	listings[dir] = entries

	for _, entry := range entries {
		child := entry.Name
		if dir != "" {
			child = dir + "/" + entry.Name
		}

		if entry.IsDir {
			if err = recordTree(ctx, service, path, index, child, listings, files); err != nil {
				return err
			}

			continue
		}

		if files[child], err = service.BrowseFile(ctx, path, index, -1, child); err != nil {
			return fmt.Errorf("reading %q of %q: %w", child, path, err)
		}
	}

	return nil
}
