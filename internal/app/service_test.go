package app

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/library"
	"github.com/primandproper/docent/internal/links"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/validate"
	"github.com/primandproper/docent/internal/walk"
	"github.com/primandproper/docent/internal/workspace"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const (
	mainTour  = "/r/.tours/main.tour"
	draftTour = "/r/.tours/draft.tour"
	junkTour  = "/r/.tours/junk.tour"
)

// fakeLauncher records what it was asked to open.
type fakeLauncher struct {
	opened []editor.Target
	ids    []string
}

func (f *fakeLauncher) Detect(context.Context) []editor.Editor {
	return []editor.Editor{{ID: "zed", Name: "Zed"}}
}

func (f *fakeLauncher) Open(_ context.Context, target *editor.Target, id string) (*editor.Result, error) {
	f.opened = append(f.opened, *target)
	f.ids = append(f.ids, id)

	return &editor.Result{Via: editor.ViaEditor, Editor: editor.Editor{ID: "zed", Name: "Zed"}}, nil
}

func newService() *Service {
	service, _ := newServiceWith(nil)
	return service
}

func newServiceWith(repos workspace.RepoResolver) (*Service, *fakeLauncher) {
	source := gitsrc.Memory{
		gitsrc.Key("/r", "", "a.go"):                 []byte("package a\n\nfunc A() {}\n"),
		gitsrc.Key("/r", "", "bin/blob"):             {0x00, 0x01, 0x02},
		gitsrc.Key("/r", "", "docs/x.md"):            []byte("x\n"),
		gitsrc.Key("/r", "", "big/big.go"):           []byte(strings.Repeat("x", maxFileSize+1)),
		gitsrc.Key("/r/.tours", "", "draft/one.md"):  []byte("# One\n\nThe long version.\n"),
		gitsrc.Key("/checkouts/api", "", "items.go"): []byte("package items\n"),
	}

	store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{
		mainTour: []byte(`{"title": "Main", "isPrimary": true, "description": "The big one", "steps": [
			{ "description": "d", "file": "a.go", "line": 3, "pattern": "^func A" },
			{ "description": "d", "file": "a.go", "line": 1, "pattern": "^func A" },
			{ "description": "d", "file": "gone.go" },
			{ "description": "d", "directory": "docs" },
			{ "description": "d", "file": "bin/blob" },
			{ "description": "d", "file": "big/big.go" },
			{ "description": "d" }
		]}`),
		draftTour: []byte(`{"docent": 1, "title": "Draft", "tags": ["cache"],
			"repos": { "api": { "remote": "git@github.com:acme/api.git" } },
			"vars": { "env": { "choices": ["staging", "prod"], "default": "staging" }, "team": {} },
			"generatedBy": { "agent": "claude-code", "at": "2026-11-16T10:00:00Z" },
			"steps": [
				{ "id": "one", "description": "d", "body": "draft/one.md", "links": [
					{ "kind": "grafana", "title": "p99", "url": "https://grafana.example.com/d/abc?env={{env}}" },
					{ "kind": "logs", "title": "Logs", "url": "{{logs}}/search" }
				] },
				{ "id": "two", "description": "d", "repo": "api", "file": "items.go", "line": 1 },
				{ "id": "three", "description": "d", "body": "draft/gone.md" }
			]}`),
		junkTour: []byte(`{nope`),
	}}

	vars := links.NewVars(&workspace.Config{}, nil)
	walker := walk.NewWalker(&walk.Dependencies{Source: source, Repos: repos, Vars: vars})
	services := &core.Core{
		Config: &workspace.Config{},
		Vars:   vars,
		Store:  store,
		Source: source,
		Repos:  repos,
		Walker: walker,
		Validator: validate.NewValidator(store, walker, func(ctx context.Context) ([][]string, error) {
			paths, err := store.Discover(ctx, "/r")
			return [][]string{paths}, err
		}, nil, nil),
	}
	launcher := &fakeLauncher{}

	return NewService("/r", services, launcher, nil, nil), launcher
}

// fixedRepos resolves every repo to one checkout, or to none.
type fixedRepos struct{ root string }

func (f fixedRepos) Resolve(_ context.Context, repo *tour.Repo) (*workspace.Checkout, error) {
	if f.root == "" {
		return &workspace.Checkout{CloneCommand: []string{"git", "clone", repo.Remote, "/w/my work/" + repo.Name}}, nil
	}

	return &workspace.Checkout{Found: true, Root: f.root, How: workspace.ByConvention}, nil
}

func TestService_ListTours(T *testing.T) {
	T.Parallel()

	T.Run("summarizes every tour, primary first", func(t *testing.T) {
		t.Parallel()

		tours, err := newService().ListTours(t.Context())
		must.NoError(t, err)
		must.Len(t, 3, tours)

		test.Eq(t, TourSummary{
			Path: mainTour, Folder: "/r", DisplayPath: ".tours/main.tour", Title: "Main", Description: "The big one",
			Tags: []string{}, Repos: []string{}, Steps: 7, Errors: 1, Warnings: 1, IsPrimary: true,
		}, tours[0])

		test.Eq(t, "Draft", tours[1].Title)
		test.True(t, tours[1].Unreviewed)
		test.Eq(t, []string{"cache"}, tours[1].Tags)
		test.Eq(t, []string{"api"}, tours[1].Repos)
		// The repo isn't checked out and a body is missing: two errors. One
		// link variable has no value: a warning.
		test.Eq(t, 2, tours[1].Errors)
		test.Eq(t, 1, tours[1].Warnings)

		test.True(t, tours[2].Unreadable)
		test.Eq(t, "junk.tour", tours[2].Title)
		test.Eq(t, 1, tours[2].Errors)
	})
}

func TestService_OpenTour(T *testing.T) {
	T.Parallel()

	T.Run("returns the tour with a status per step", func(t *testing.T) {
		t.Parallel()

		view, err := newService().OpenTour(t.Context(), mainTour)
		must.NoError(t, err)
		test.Eq(t, "Main", view.Tour.Title)
		test.Len(t, 0, view.Diagnostics)
		test.NotNil(t, view.Diagnostics)

		statuses := make([]anchor.Status, len(view.Steps))
		for i, s := range view.Steps {
			statuses[i] = s.Status
		}

		test.Eq(t, []anchor.Status{
			anchor.StatusOK, anchor.StatusMoved, anchor.StatusMissing, anchor.StatusOK,
			anchor.StatusOK, anchor.StatusOK, anchor.StatusOK,
		}, statuses)
		test.Eq(t, walk.KindDirectory, view.Steps[3].Kind)
		test.Eq(t, walk.KindContent, view.Steps[6].Kind)
	})

	T.Run("refuses a path the workspace didn't discover", func(t *testing.T) {
		t.Parallel()

		_, err := newService().OpenTour(t.Context(), "/etc/passwd")
		test.ErrorIs(t, err, ErrUnknownTour)
	})

	T.Run("an unparseable tour is an error that says why", func(t *testing.T) {
		t.Parallel()

		_, err := newService().OpenTour(t.Context(), junkTour)
		test.ErrorIs(t, err, ErrTourUnreadable)
	})
}

func TestService_Step(T *testing.T) {
	T.Parallel()

	T.Run("returns the file and where the anchor landed", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 0)
		must.NoError(t, err)
		test.Eq(t, walk.KindFile, step.Kind)
		test.Eq(t, "a.go", step.Path)
		test.Eq(t, "package a\n\nfunc A() {}\n", step.Content)
		test.Eq(t, anchor.Range{StartLine: 3, EndLine: 3}, step.Location.Range)
		test.Eq(t, 0, step.Index)
		test.NotNil(t, step.Entries)
	})

	T.Run("a missing file carries its reason", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 2)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusMissing, step.Location.Status)
		test.Eq(t, diag.CodeFileMissing, step.Location.Problem)
		test.Eq(t, "", step.Content)
	})

	T.Run("a directory step lists its entries", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 3)
		must.NoError(t, err)
		test.Eq(t, []gitsrc.Entry{{Name: "x.md"}}, step.Entries)
		test.Eq(t, anchor.StatusOK, step.Location.Status)
	})

	T.Run("binary and oversized files are withheld, with a reason", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), mainTour, 4)
		must.NoError(t, err)
		test.Eq(t, "", step.Content)
		test.Eq(t, "This is a binary file.", step.Unavailable)

		step, err = s.Step(t.Context(), mainTour, 5)
		must.NoError(t, err)
		test.Eq(t, "", step.Content)
		test.StrContains(t, step.Unavailable, "too large")
	})

	T.Run("refuses a tour that isn't open", func(t *testing.T) {
		t.Parallel()

		_, err := newService().Step(t.Context(), mainTour, 0)
		test.ErrorIs(t, err, ErrUnknownTour)
	})

	T.Run("refuses an index outside the tour", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		_, err = s.Step(t.Context(), mainTour, 99)
		test.ErrorIs(t, err, walk.ErrNoSuchStep)

		_, err = s.Step(t.Context(), mainTour, -1)
		test.ErrorIs(t, err, walk.ErrNoSuchStep)
	})
}

func TestService_Folders(T *testing.T) {
	T.Parallel()

	T.Run("names the folder it was given", func(t *testing.T) {
		t.Parallel()

		folders := newService().Folders()
		must.Len(t, 1, folders)
		test.Eq(t, "/r", folders[0].Root)
		test.Eq(t, "r", folders[0].Name)
		test.False(t, folders[0].Saved)
	})
}

func TestService_Launch(T *testing.T) {
	T.Parallel()

	// Two real folders of tours, and a service that starts with neither. The
	// feature tour links into the cron tour, which lives in the other folder.
	type world struct {
		svc     *Service
		library *library.Memory
		opened  *[][]string
		choose  *string
		feature string
		cron    string
		a, b    string
	}

	setup := func(t *testing.T) *world {
		t.Helper()

		dir, err := filepath.EvalSymlinks(t.TempDir())
		must.NoError(t, err)

		w := &world{a: filepath.Join(dir, "a"), b: filepath.Join(dir, "b"), library: &library.Memory{}}
		w.feature = filepath.Join(w.a, ".tours", "feature.tour")
		w.cron = filepath.Join(w.b, ".tours", "cron.tour")

		for path, content := range map[string]string{
			w.feature: `{"docent": 1, "title": "Feature", "nextTour": "Cron",
				"vars": { "grafana": { "description": "Grafana" } },
				"steps": [{"id": "s", "description": "It runs [on a schedule](docent:tour/cron.tour#scheduling)."}]}`,
			w.cron:                            `{"docent": 1, "title": "Cron", "steps": [{"id": "scheduling", "description": "d"}]}`,
			filepath.Join(w.a, "docent.json"): `{"vars": {"grafana": "https://grafana.a.example"}}`,
		} {
			must.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
			must.NoError(t, os.WriteFile(path, []byte(content), 0o600))
		}

		rebuild := func(root string) (*core.Core, error) {
			return core.New(&core.Options{Root: root, Elsewhere: w.svc.elsewhere(root), UserConfigPath: filepath.Join(dir, "no-config.json")})
		}

		empty, err := core.New(&core.Options{UserConfigPath: filepath.Join(dir, "no-config.json")})
		must.NoError(t, err)

		w.svc = NewService("", empty, &fakeLauncher{}, nil, nil)
		w.svc.setLibrary(w.library)
		t.Cleanup(w.svc.close)

		var (
			seen   [][]string
			chosen = w.a
		)

		w.opened, w.choose = &seen, &chosen

		w.svc.setHost(&Host{
			Rebuild:      rebuild,
			ChooseFolder: func() (string, error) { return chosen, nil },
			Opened: func(folders []*core.Core) {
				roots := make([]string, 0, len(folders))
				for _, f := range folders {
					roots = append(roots, f.Config.Root)
				}

				seen = append(seen, roots)
			},
		})

		return w
	}

	roots := func(folders []Folder) []string {
		out := make([]string, 0, len(folders))
		for i := range folders {
			out = append(out, folders[i].Root)
		}

		return out
	}

	T.Run("with no folder there are no folders and no tours", func(t *testing.T) {
		t.Parallel()

		w := setup(t)
		test.Len(t, 0, w.svc.Folders())

		tours, err := w.svc.ListTours(t.Context())
		must.NoError(t, err)
		test.Len(t, 0, tours)
	})

	T.Run("adding a folder opens it and saves it", func(t *testing.T) {
		t.Parallel()

		w := setup(t)

		folders, err := w.svc.AddFolder(t.Context())
		must.NoError(t, err)
		must.Len(t, 1, folders)
		test.Eq(t, Folder{Root: w.a, Name: "a", Saved: true}, folders[0])
		test.Eq(t, []string{w.a}, w.library.Saved)
		test.Eq(t, [][]string{{w.a}}, *w.opened)

		tours, err := w.svc.ListTours(t.Context())
		must.NoError(t, err)
		must.Len(t, 1, tours)
		test.Eq(t, w.feature, tours[0].Path)
		test.Eq(t, w.a, tours[0].Folder)
		test.Eq(t, filepath.Join(".tours", "feature.tour"), tours[0].DisplayPath)

		// Adding it again, or cancelling the dialog, changes nothing.
		folders, err = w.svc.AddFolder(t.Context())
		must.NoError(t, err)
		test.Len(t, 1, folders)

		*w.choose = ""
		folders, err = w.svc.AddFolder(t.Context())
		must.NoError(t, err)
		test.Len(t, 1, folders)
		test.Eq(t, []string{w.a}, w.library.Saved)
	})

	T.Run("each folder's tours are listed under it, and links reach across", func(t *testing.T) {
		t.Parallel()

		w := setup(t)

		_, err := w.svc.AddFolder(t.Context())
		must.NoError(t, err)

		// Alone, the feature tour's link and its nextTour lead nowhere.
		tours, err := w.svc.ListTours(t.Context())
		must.NoError(t, err)
		must.Len(t, 1, tours)
		test.Eq(t, 2, tours[0].Warnings)

		*w.choose = w.b
		folders, err := w.svc.AddFolder(t.Context())
		must.NoError(t, err)
		test.Eq(t, []string{w.a, w.b}, roots(folders))

		tours, err = w.svc.ListTours(t.Context())
		must.NoError(t, err)
		must.Len(t, 2, tours)
		test.Eq(t, w.a, tours[0].Folder)
		test.Eq(t, w.b, tours[1].Folder)
		test.Eq(t, 0, tours[0].Warnings+tours[0].Errors)

		// Each tour is read with its own folder's configuration.
		view, err := w.svc.OpenTour(t.Context(), w.feature)
		must.NoError(t, err)
		must.Len(t, 1, view.Vars)
		test.Eq(t, "https://grafana.a.example", view.Vars[0].Value)

		_, err = w.svc.OpenTour(t.Context(), w.cron)
		must.NoError(t, err)
		test.True(t, w.svc.svc(w.feature) != w.svc.svc(w.cron))
	})

	T.Run("removing a folder closes it and forgets it", func(t *testing.T) {
		t.Parallel()

		w := setup(t)

		_, err := w.svc.AddFolder(t.Context())
		must.NoError(t, err)
		_, err = w.svc.OpenTour(t.Context(), w.feature)
		must.NoError(t, err)

		_, err = w.svc.RemoveFolder(t.Context(), w.b)
		test.ErrorIs(t, err, ErrUnknownFolder)

		folders, err := w.svc.RemoveFolder(t.Context(), w.a)
		must.NoError(t, err)
		test.Len(t, 0, folders)
		test.Len(t, 0, w.library.Saved)

		_, err = w.svc.OpenTour(t.Context(), w.feature)
		test.ErrorIs(t, err, ErrUnknownTour)
		_, err = w.svc.Step(t.Context(), w.feature, 0)
		test.ErrorIs(t, err, ErrUnknownTour)
	})

	T.Run("a docent link opens the tour's folder without saving it, and queues the tour once", func(t *testing.T) {
		t.Parallel()

		w := setup(t)

		must.NoError(t, w.svc.openLink(t.Context(), "docent://open?tour="+url.QueryEscape(w.feature)+"&step=intro"))
		test.Len(t, 1, *w.opened)
		test.Eq(t, LaunchRequest{Tour: w.feature, Step: "intro"}, w.svc.Launch())
		test.Eq(t, LaunchRequest{}, w.svc.Launch())

		folders := w.svc.Folders()
		must.Len(t, 1, folders)
		test.False(t, folders[0].Saved)
		test.Len(t, 0, w.library.Saved)

		// Already open: nothing more is opened.
		must.NoError(t, w.svc.openLink(t.Context(), "docent://open?tour="+url.QueryEscape(w.feature)))
		test.Len(t, 1, w.svc.Folders())
		test.Eq(t, LaunchRequest{Tour: w.feature}, w.svc.Launch())

		// A second link opens its folder beside the first.
		must.NoError(t, w.svc.openLink(t.Context(), "docent://open?tour="+url.QueryEscape(w.cron)))
		test.Eq(t, []string{w.a, w.b}, roots(w.svc.Folders()))
	})

	T.Run("only a folder that is open can be kept", func(t *testing.T) {
		t.Parallel()

		w := setup(t)

		_, err := w.svc.KeepFolder(t.Context(), w.a)
		test.ErrorIs(t, err, ErrUnknownFolder)
		test.Len(t, 0, w.library.Saved)

		must.NoError(t, w.svc.openFolder(t.Context(), w.a, false))

		folders, err := w.svc.KeepFolder(t.Context(), w.a)
		must.NoError(t, err)
		must.Len(t, 1, folders)
		test.True(t, folders[0].Saved)
		test.Eq(t, []string{w.a}, w.library.Saved)
	})

	T.Run("the saved folders are opened at the start, after the one asked for", func(t *testing.T) {
		t.Parallel()

		w := setup(t)
		gone := filepath.Join(filepath.Dir(w.a), "gone")
		w.library.Saved = []string{w.a, gone, w.b}

		must.NoError(t, w.svc.openFolder(t.Context(), w.b, false))
		must.NoError(t, w.svc.restore(t.Context()))

		folders := w.svc.Folders()
		test.Eq(t, []string{w.b, w.a, gone}, roots(folders))
		test.True(t, folders[0].Saved)
		test.Eq(t, "", folders[1].Problem)
		test.NotEq(t, "", folders[2].Problem)

		tours, err := w.svc.ListTours(t.Context())
		must.NoError(t, err)
		test.Len(t, 2, tours)

		// What is gone can still be removed.
		folders, err = w.svc.RemoveFolder(t.Context(), gone)
		must.NoError(t, err)
		test.Len(t, 2, folders)
		test.Eq(t, []string{w.a, w.b}, w.library.Saved)
	})

	T.Run("links that aren't a tour on disk are refused", func(t *testing.T) {
		t.Parallel()

		w := setup(t)

		for _, bad := range []string{
			"docent://open",
			"docent://open?tour=relative.tour",
			"docent://open?tour=/etc/passwd",
			"docent://open?tour=" + url.QueryEscape(w.feature+".gone.tour"),
			"docent://run?tour=" + url.QueryEscape(w.feature),
			"https://open?tour=" + url.QueryEscape(w.feature),
			"docent://open?tour=" + url.QueryEscape(filepath.Dir(w.feature)),
		} {
			test.ErrorIs(t, w.svc.openLink(t.Context(), bad), ErrBadLink, test.Sprint(bad))
		}

		test.Len(t, 0, *w.opened)
		test.Eq(t, LaunchRequest{}, w.svc.Launch())
	})

	T.Run("without a host no folder can be opened", func(t *testing.T) {
		t.Parallel()

		s := newService()

		_, err := s.AddFolder(t.Context())
		test.ErrorIs(t, err, ErrNoHost)
		test.ErrorIs(t, s.requestTour(t.Context(), "/elsewhere/.tours/x.tour", ""), ErrNoHost)
	})

	T.Run("the map window is the host's to open and close", func(t *testing.T) {
		t.Parallel()

		s := newService()
		test.ErrorIs(t, s.ToggleMapWindow(t.Context()), ErrNoMapWindow)

		toggled := 0
		s.setHost(&Host{ToggleMapWindow: func() { toggled++ }})

		test.NoError(t, s.ToggleMapWindow(t.Context()))
		test.NoError(t, s.ToggleMapWindow(t.Context()))
		test.Eq(t, 2, toggled)
	})
}
