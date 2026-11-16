package app

import (
	"context"
	"strings"
	"testing"

	"github.com/primandproper/docent/internal/anchor"
	"github.com/primandproper/docent/internal/core"
	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/editor"
	"github.com/primandproper/docent/internal/gitsrc"
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
		Config:    &workspace.Config{},
		Vars:      vars,
		Store:     store,
		Source:    source,
		Repos:     repos,
		Walker:    walker,
		Validator: validate.NewValidator(store, walker, nil, nil),
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
			Path: mainTour, DisplayPath: ".tours/main.tour", Title: "Main", Description: "The big one",
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

func TestService_Workspace(T *testing.T) {
	T.Parallel()

	T.Run("names the opened folder", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, Workspace{Root: "/r", Name: "r"}, newService().Workspace())
	})
}

func TestService_Narrative(T *testing.T) {
	T.Parallel()

	T.Run("a step carries its body and its expanded link cards", func(t *testing.T) {
		t.Parallel()

		s := newService()
		view, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)
		test.Eq(t, []VarView{
			{Name: "env", Value: "staging", Source: links.SourceTour, Choices: []string{"staging", "prod"}},
			{Name: "team", Choices: []string{}},
		}, view.Vars)

		step, err := s.Step(t.Context(), draftTour, 0)
		must.NoError(t, err)
		test.Eq(t, "# One\n\nThe long version.\n", step.Body)
		test.Eq(t, "", step.BodyProblem)
		must.Len(t, 2, step.Links)
		test.Eq(t, "https://grafana.example.com/d/abc?env=staging", step.Links[0].URL)
		test.Eq(t, "grafana.example.com", step.Links[0].Host)
		test.Eq(t, []string{"logs"}, step.Links[1].Missing)
	})

	T.Run("a missing body says so and the step still loads", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), draftTour, 2)
		must.NoError(t, err)
		test.Eq(t, "", step.Body)
		test.StrContains(t, step.BodyProblem, `body "draft/gone.md" does not exist`)
	})
}

func TestService_SetVar(T *testing.T) {
	T.Parallel()

	T.Run("a picked value changes the links", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		vars, err := s.SetVar(t.Context(), draftTour, "env", "prod")
		must.NoError(t, err)
		test.Eq(t, "prod", vars[0].Value)
		test.Eq(t, links.SourcePicker, vars[0].Source)

		step, err := s.Step(t.Context(), draftTour, 0)
		must.NoError(t, err)
		test.Eq(t, "https://grafana.example.com/d/abc?env=prod", step.Links[0].URL)

		vars, err = s.SetVar(t.Context(), draftTour, "env", "")
		must.NoError(t, err)
		test.Eq(t, "staging", vars[0].Value)
	})

	T.Run("a free-form variable takes any value", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		vars, err := s.SetVar(t.Context(), draftTour, "team", "payments")
		must.NoError(t, err)
		test.Eq(t, "payments", vars[1].Value)
	})

	T.Run("refuses a value outside the choices, an undeclared variable, and a closed tour", func(t *testing.T) {
		t.Parallel()

		s := newService()

		_, err := s.SetVar(t.Context(), draftTour, "env", "prod")
		test.ErrorIs(t, err, ErrUnknownTour)

		_, err = s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		_, err = s.SetVar(t.Context(), draftTour, "env", "qa")
		test.ErrorIs(t, err, ErrBadVariable)

		_, err = s.SetVar(t.Context(), draftTour, "grafana", "https://evil.example.com")
		test.ErrorIs(t, err, ErrBadVariable)
	})
}

func TestService_Repos(T *testing.T) {
	T.Parallel()

	T.Run("a step in a named repo reads from that checkout", func(t *testing.T) {
		t.Parallel()

		s, _ := newServiceWith(fixedRepos{root: "/checkouts/api"})
		view, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusOK, view.Steps[1].Status)

		step, err := s.Step(t.Context(), draftTour, 1)
		must.NoError(t, err)
		test.Eq(t, "api", step.Repo)
		test.Eq(t, "package items\n", step.Content)
		test.Eq(t, "", step.CloneCommand)
	})

	T.Run("a repo that isn't checked out comes with a command to paste", func(t *testing.T) {
		t.Parallel()

		s, _ := newServiceWith(fixedRepos{})
		_, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		step, err := s.Step(t.Context(), draftTour, 1)
		must.NoError(t, err)
		test.Eq(t, anchor.StatusMissing, step.Location.Status)
		test.Eq(t, diag.CodeRepoMissing, step.Location.Problem)
		test.Eq(t, "git clone git@github.com:acme/api.git '/w/my work/api'", step.CloneCommand)
	})
}

func TestService_OpenInEditor(T *testing.T) {
	T.Parallel()

	T.Run("refuses steps with nothing to open, and tours that aren't open", func(t *testing.T) {
		t.Parallel()

		s, launcher := newServiceWith(nil)

		_, err := s.OpenInEditor(t.Context(), mainTour, 0, -1, "")
		test.ErrorIs(t, err, ErrUnknownTour)

		_, err = s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		for _, index := range []int{2, 3, 6} {
			_, err = s.OpenInEditor(t.Context(), mainTour, index, -1, "")
			test.ErrorIs(t, err, ErrNotAFile, test.Sprintf("step %d", index))
		}

		_, err = s.OpenInEditor(t.Context(), mainTour, 99, -1, "")
		test.ErrorIs(t, err, walk.ErrNoSuchStep)
		test.Len(t, 0, launcher.opened)
	})

	T.Run("lists the installed editors", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, []editor.Editor{{ID: "zed", Name: "Zed"}}, newService().Editors(t.Context()))
	})
}

func TestShellJoin(T *testing.T) {
	T.Parallel()

	T.Run("quotes only what needs it", func(t *testing.T) {
		t.Parallel()

		test.Eq(t, `git clone git@github.com:acme/api.git '/w/it'\''s here/api' ''`, shellJoin([]string{"git", "clone", "git@github.com:acme/api.git", "/w/it's here/api", ""}))
		test.Eq(t, "", shellJoin(nil))
	})
}

func TestCloneable(T *testing.T) {
	T.Parallel()

	T.Run("only plain https and ssh remotes", func(t *testing.T) {
		t.Parallel()

		for _, ok := range []string{"https://github.com/acme/api.git", "ssh://git@github.com/acme/api", "git@github.com:acme/api.git", "deploy_1@git.example.com:team/repo"} {
			test.True(t, cloneable(ok), test.Sprint(ok))
		}

		for _, bad := range []string{
			"", "/srv/git/api", "../api", "file:///srv/git/api", "ext::sh -c touch%20/tmp/pwned", "ext::sh",
			"-oProxyCommand=evil@host:x", "git@-oProxyCommand=evil:x", "git@host:-x", "http://github.com/acme/api",
			"git@github.com:acme/api.git --upload-pack=evil", "git@github.com", "fd::7",
		} {
			test.False(t, cloneable(bad), test.Sprint(bad))
		}
	})
}

func TestService_RepoActions(T *testing.T) {
	T.Parallel()

	T.Run("refuse tours that aren't open and repos the tour doesn't have", func(t *testing.T) {
		t.Parallel()

		s, _ := newServiceWith(fixedRepos{})

		test.ErrorIs(t, s.Fetch(t.Context(), draftTour, "api"), ErrUnknownTour)
		test.ErrorIs(t, s.Clone(t.Context(), draftTour, "api"), ErrUnknownTour)

		_, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		test.ErrorIs(t, s.Fetch(t.Context(), draftTour, "nope"), walk.ErrNoSuchRepo)
		test.ErrorIs(t, s.Clone(t.Context(), draftTour, "nope"), walk.ErrNoSuchRepo)
		// Not checked out, so there is nothing to fetch.
		test.ErrorIs(t, s.Fetch(t.Context(), draftTour, "api"), walk.ErrNoSuchRepo)
	})

	T.Run("a repo that is already here can't be cloned again", func(t *testing.T) {
		t.Parallel()

		s, _ := newServiceWith(fixedRepos{root: "/checkouts/api"})
		_, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)

		test.ErrorIs(t, s.Clone(t.Context(), draftTour, "api"), ErrNotCloneable)
	})

	T.Run("an opened tour reports its repos", func(t *testing.T) {
		t.Parallel()

		s, _ := newServiceWith(fixedRepos{})
		view, err := s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)
		must.Len(t, 1, view.Repos)
		test.Eq(t, "api", view.Repos[0].Name)
		test.False(t, view.Repos[0].Found)
		test.Eq(t, []string{"git", "clone", "git@github.com:acme/api.git", "/w/my work/api"}, view.Repos[0].CloneCommand)
		test.False(t, view.WorkingTree)

		s.SetWorkingTree(true)

		view, err = s.OpenTour(t.Context(), draftTour)
		must.NoError(t, err)
		test.True(t, view.WorkingTree)
	})
}

func TestService_Browse(T *testing.T) {
	T.Parallel()

	T.Run("lists and reads files of the step's repo, and nothing outside it", func(t *testing.T) {
		t.Parallel()

		s := newService()
		_, err := s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		entries, err := s.Browse(t.Context(), mainTour, 0, -1, "")
		must.NoError(t, err)
		test.SliceContains(t, entries, gitsrc.Entry{Name: "docs", IsDir: true})
		test.SliceContains(t, entries, gitsrc.Entry{Name: "a.go"})

		file, err := s.BrowseFile(t.Context(), mainTour, 0, -1, "docs/x.md")
		must.NoError(t, err)
		test.Eq(t, &FileView{Path: "docs/x.md", Content: "x\n"}, file)

		file, err = s.BrowseFile(t.Context(), mainTour, 0, -1, "bin/blob")
		must.NoError(t, err)
		test.Eq(t, "This is a binary file.", file.Unavailable)

		_, err = s.BrowseFile(t.Context(), mainTour, 0, -1, "../outside")
		test.ErrorIs(t, err, gitsrc.ErrEscapesRoot)

		_, err = s.Browse(t.Context(), mainTour, 0, -1, "../..")
		test.ErrorIs(t, err, gitsrc.ErrEscapesRoot)
	})

	T.Run("only steps with code can be browsed", func(t *testing.T) {
		t.Parallel()

		s := newService()

		_, err := s.Browse(t.Context(), mainTour, 0, -1, "")
		test.ErrorIs(t, err, ErrUnknownTour)

		_, err = s.OpenTour(t.Context(), mainTour)
		must.NoError(t, err)

		_, err = s.Browse(t.Context(), mainTour, 6, -1, "")
		test.ErrorIs(t, err, ErrNotAFile)

		_, err = s.Browse(t.Context(), mainTour, 0, 3, "")
		test.ErrorIs(t, err, ErrNotAFile)
	})
}
