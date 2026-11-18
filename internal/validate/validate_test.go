package validate

import (
	"context"
	"testing"

	"github.com/primandproper/docent/internal/diag"
	"github.com/primandproper/docent/internal/gitsrc"
	"github.com/primandproper/docent/internal/tour"
	"github.com/primandproper/docent/internal/walk"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const tourPath = "/r/.tours/t.tour"

func newValidator(tourJSON string) Validator {
	source := gitsrc.Memory{
		gitsrc.Key("/r", "", "a.go"):        []byte("package a\n\nfunc A() {}\n\nfunc B() {}\n"),
		gitsrc.Key("/r", "", "docs/x.md"):   []byte("x\n"),
		gitsrc.Key("/r", "", ".tours/keep"): []byte(""),

		gitsrc.Key("/r/.tours", "", "t/map.mmd"):   []byte("flowchart LR\n  api e1@--> queue\n  queue --> worker\n"),
		gitsrc.Key("/r/.tours", "", "t/notmap.md"): []byte("sequenceDiagram\n  a->>b: hi\n"),
	}
	return newValidatorFor(source, map[string]string{tourPath: tourJSON})
}

// newValidatorFor validates among several tours, so links between them can be checked.
func newValidatorFor(source gitsrc.Memory, tours map[string]string) Validator {
	store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{}}
	for path, content := range tours {
		store.Files[path] = []byte(content)
	}

	discover := func(ctx context.Context) ([][]string, error) {
		paths, err := store.Discover(ctx, "/r")
		return [][]string{paths}, err
	}

	return NewValidator(store, walk.NewWalker(&walk.Dependencies{Source: source}), discover, nil, nil)
}

func TestValidator_Map(T *testing.T) {
	T.Parallel()

	T.Run("mapFocus may name nodes and named edges, inline or from a file", func(t *testing.T) {
		t.Parallel()

		for _, m := range []string{`{ "file": "t/map.mmd" }`, `{ "mermaid": "flowchart LR\n  api e1@--> queue --> worker" }`} {
			diags, err := newValidator(`{"docent": 1, "title": "t", "map": `+m+`, "steps": [
				{ "id": "a", "description": "d", "mapFocus": ["api", "e1", "worker"] }
			]}`).Tour(t.Context(), tourPath)
			must.NoError(t, err)
			test.Len(t, 0, diags, test.Sprintf("%s: %+v", m, diags))
		}
	})

	T.Run("an id the map doesn't define is an error, where it is", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "map": { "file": "t/map.mmd" }, "steps": [
			{ "id": "a", "description": "d", "mapFocus": ["api", "cache"] }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)
		test.Eq(t, diag.CodeMapFocusUnknown, diags[0].Code)
		test.Eq(t, "/steps/0/mapFocus/1", diags[0].Pointer)
		test.Eq(t, "a", diags[0].Step)
	})

	T.Run("a map that is missing or not a flowchart is reported once", func(t *testing.T) {
		t.Parallel()

		for _, file := range []string{"t/gone.mmd", "t/notmap.md", "../../etc/passwd"} {
			diags, err := newValidator(`{"docent": 1, "title": "t", "map": { "file": "`+file+`" }, "steps": [
				{ "id": "a", "description": "d", "mapFocus": ["api"] }
			]}`).Tour(t.Context(), tourPath)
			must.NoError(t, err)
			must.Len(t, 1, diags, must.Sprint(file))
			test.Eq(t, "/map", diags[0].Pointer)
		}
	})

	T.Run("mapFocus without a map is an error", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d", "mapFocus": ["api"] }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)
		test.StrContains(t, diags[0].Message, "the tour has no `map`")
	})
}

func TestValidator_Tour(T *testing.T) {
	T.Parallel()

	T.Run("a tour whose anchors all resolve is clean", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"title": "t", "steps": [
			{ "description": "d", "file": "a.go", "line": 3, "pattern": "^func A" },
			{ "description": "d", "file": "a.go", "line": 5 },
			{ "description": "d", "file": "a.go", "selection": { "start": { "line": 3, "character": 1 }, "end": { "line": 5, "character": 4 } } },
			{ "description": "d", "directory": "docs" },
			{ "description": "d", "uri": "https://example.com" },
			{ "description": "d" }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 0, diags, test.Sprintf("%+v", diags))
	})

	T.Run("reports each kind of broken anchor where it is", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "gone", "description": "d", "file": "gone.go" },
			{ "id": "escape", "description": "d", "file": "../../etc/passwd" },
			{ "id": "far", "description": "d", "file": "a.go", "line": 99 },
			{ "id": "regex", "description": "d", "file": "a.go", "pattern": "(?<=x)y" },
			{ "id": "nomatch", "description": "d", "file": "a.go", "pattern": "^func C" },
			{ "id": "twice", "description": "d", "file": "a.go", "pattern": "^func " },
			{ "id": "moved", "description": "d", "file": "a.go", "pattern": "^func B", "line": 3 },
			{ "id": "nodir", "description": "d", "directory": "nope" },
			{ "id": "sel", "description": "d", "file": "a.go", "selection": { "start": { "line": 40, "character": 1 }, "end": { "line": 41, "character": 1 } } }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)

		type finding struct {
			step     string
			code     diag.Code
			pointer  string
			severity diag.Severity
		}

		got := make([]finding, len(diags))
		for i, d := range diags {
			got[i] = finding{d.Step, d.Code, d.Pointer, d.Severity}

			test.Eq(t, tourPath, d.File)
			test.NotEq(t, "", d.Message)
		}

		test.Eq(t, []finding{
			{"gone", diag.CodeFileMissing, "/steps/0/file", diag.SeverityError},
			{"escape", diag.CodePathEscapesRoot, "/steps/1/file", diag.SeverityError},
			{"far", diag.CodeLineOutOfRange, "/steps/2/line", diag.SeverityError},
			{"regex", diag.CodePatternInvalid, "/steps/3/pattern", diag.SeverityError},
			{"nomatch", diag.CodePatternNoMatch, "/steps/4/pattern", diag.SeverityError},
			{"twice", diag.CodePatternAmbiguous, "/steps/5/pattern", diag.SeverityError},
			{"moved", diag.CodeAnchorMoved, "/steps/6/line", diag.SeverityWarning},
			{"nodir", diag.CodeFileMissing, "/steps/7/directory", diag.SeverityError},
			{"sel", diag.CodeLineOutOfRange, "/steps/8/selection", diag.SeverityError},
		}, got)
	})

	T.Run("parse diagnostics come first and steps are still checked", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"title": "t", "tags": ["x"], "steps": [
			{ "description": "d", "file": "gone.go" }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 2, diags)
		test.Eq(t, diag.CodeExtensionIgnored, diags[0].Code)
		test.Eq(t, diag.CodeFileMissing, diags[1].Code)
		test.Eq(t, "#1", diags[1].Step)
	})

	T.Run("an unparseable tour reports only that", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{nope`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)
		test.Eq(t, diag.CodeJSONSyntax, diags[0].Code)
	})
}

func TestValidator_Embeds(T *testing.T) {
	T.Parallel()

	find := func(diags []diag.Diagnostic, code diag.Code) []string {
		var out []string

		for i := range diags {
			if diags[i].Code == code {
				out = append(out, diags[i].Pointer+" "+diags[i].Message)
			}
		}

		return out
	}

	T.Run("valid visuals, fences and step links are clean", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "See [b](docent:step/b).\n\n`+"```"+`docent-sequence\nparticipants: [x, y]\nmessages:\n  - { from: x, to: y, label: hi, step: b }\n`+"```"+`\n\n`+"```"+`docent-links\n- { kind: doc, title: Docs, url: \"https://example.com\" }\n`+"```"+`",
			  "visual": { "type": "timeline", "lanes": ["w"], "events": [{ "lane": "w", "at": 0, "label": "go" }] } },
			{ "id": "b", "description": "d" }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 0, diags, test.Sprintf("%+v", diags))
	})

	T.Run("a broken `visual` is pointed at inside the JSON", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d", "visual": { "type": "sequence", "participants": ["x", "y"], "messages": [{ "from": "x", "to": "z", "label": "hi", "step": "gone" }] } }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Eq(t, []string{`/steps/0/visual/messages/0/to "z" is not one of the participants (x, y)`}, find(diags, diag.CodeVisualInvalid))
		test.Len(t, 1, find(diags, diag.CodeStepLinkUnknown))
	})

	T.Run("a broken fence says which fence, by line", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "intro\n\n`+"```"+`docent-state\nstates: [idle, busy]\ntransitions:\n  - { from: idle, to: busy, on: go }\nrun: [stop]\n`+"```"+`\n\n`+"```"+`docent-pie\nx: 1\n`+"```"+`\n\n`+"```"+`docent-links\n- { kind: slack, title: Chat, url: \"https://x\" }\n- { kind: doc }\n`+"```"+`\n\n[gone](docent:step/nowhere)" }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)

		invalid := find(diags, diag.CodeVisualInvalid)
		must.Len(t, 4, invalid, must.Sprintf("%+v", invalid))
		test.StrContains(t, invalid[0], `/steps/0/description the docent-state fence on line 3: run/0: state "idle" has no transition on "stop"`)
		test.StrContains(t, invalid[1], `the docent-pie fence on line 10: type: there is no "pie" visual`)
		test.StrContains(t, invalid[2], `link "Chat" has kind "slack"`)
		test.StrContains(t, invalid[3], "link 2 needs a `title` and a `url`")
		test.Eq(t, []string{`/steps/0/description links to step "nowhere", which this tour doesn't have`}, find(diags, diag.CodeStepLinkUnknown))
	})
}

func TestValidator_ExtraAnchors(T *testing.T) {
	T.Parallel()

	T.Run("extra anchors are checked like the step's own, and pointed at inside `anchors`", func(t *testing.T) {
		t.Parallel()

		diags, err := newValidator(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d", "file": "a.go", "line": 3, "anchors": [
				{ "label": "fine", "file": "a.go", "pattern": "^func B", "line": 5 },
				{ "label": "test", "file": "a_test.go", "line": 1 },
				{ "label": "drifted", "file": "a.go", "pattern": "^func B", "line": 3 },
				{ "label": "elsewhere", "repo": "nope", "file": "x.go" }
			] }
		]}`).Tour(t.Context(), tourPath)
		must.NoError(t, err)

		got := make([]string, len(diags))
		for i, d := range diags {
			got[i] = string(d.Code) + " " + d.Pointer + " " + string(d.Severity) + " " + d.Step
		}

		test.Eq(t, []string{
			"file-missing /steps/0/anchors/1/file error a",
			"anchor-moved /steps/0/anchors/2/line warning a",
			"repo-unknown /steps/0/anchors/3/repo error a",
		}, got)
		test.StrContains(t, diags[0].Message, `anchor "test": `)
	})
}

func TestPlacement(T *testing.T) {
	T.Parallel()

	tr := &tour.Tour{Path: "/r/.tours/t.tour", Steps: []tour.Step{
		{Repo: "api"},
		{Repo: "worker", Anchors: []tour.Anchor{{Repo: "billing"}, {Repo: "api"}}},
		{},
	}}

	T.Run("a tour inside one of its repos that reaches into others is warned about, once", func(t *testing.T) {
		t.Parallel()

		d, misplaced := placement(tr, "api")
		must.True(t, misplaced)
		test.Eq(t, diag.CodeTourMisplaced, d.Code)
		test.Eq(t, diag.SeverityWarning, d.Severity)
		test.StrContains(t, d.Message, "lives in the api repo but also anchors into worker, billing")
	})

	T.Run("a tours repo, or a tour that stays in its own repo, is fine", func(t *testing.T) {
		t.Parallel()

		_, misplaced := placement(tr, "")
		test.False(t, misplaced)

		_, misplaced = placement(&tour.Tour{Steps: []tour.Step{{Repo: "api"}, {}}}, "api")
		test.False(t, misplaced)
	})
}

func TestValidator_TourLinks(T *testing.T) {
	T.Parallel()

	const (
		cronPath = "/r/.tours/ops/cron.tour"
		cron     = `{"docent": 1, "title": "Cron", "steps": [{ "id": "scheduling", "description": "d" }]}`
	)

	source := gitsrc.Memory{
		gitsrc.Key("/r", "", ".tours/keep"):      []byte(""),
		gitsrc.Key("/r/.tours", "", "t/body.md"): []byte("See [cron](docent:tour/cron.tour#nope).\n"),
	}

	check := func(t *testing.T, tours map[string]string) []diag.Diagnostic {
		t.Helper()

		diags, err := newValidatorFor(source, tours).Tour(t.Context(), tourPath)
		must.NoError(t, err)

		return diags
	}

	T.Run("links to a tour, to one of its steps, and a nextTour that exist are clean", func(t *testing.T) {
		t.Parallel()

		diags := check(t, map[string]string{cronPath: cron, tourPath: `{"docent": 1, "title": "t", "nextTour": "Cron", "steps": [
			{ "id": "a", "description": "[a](docent:tour/cron.tour) [b](docent:tour/ops/cron.tour#scheduling) [c](docent:tour/t.tour#a)",
			  "links": [{ "kind": "doc", "title": "Cron", "url": "docent:tour/cron.tour#scheduling" }] }
		]}`})
		test.Len(t, 0, diags, test.Sprintf("%+v", diags))
	})

	T.Run("a tour that isn't here is a warning, where the link is", func(t *testing.T) {
		t.Parallel()

		diags := check(t, map[string]string{cronPath: cron, tourPath: `{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d", "links": [{ "kind": "doc", "title": "Gone", "url": "docent:tour/gone.tour#x" }] }
		]}`})
		must.Len(t, 1, diags)
		test.Eq(t, diag.CodeTourLinkUnknown, diags[0].Code)
		test.Eq(t, diag.SeverityWarning, diags[0].Severity)
		test.Eq(t, "/steps/0/links/0/url", diags[0].Pointer)
		test.Eq(t, "a", diags[0].Step)
	})

	T.Run("a step the other tour doesn't have is an error, in a body file too", func(t *testing.T) {
		t.Parallel()

		diags := check(t, map[string]string{cronPath: cron, tourPath: `{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d", "body": "t/body.md" }
		]}`})
		must.Len(t, 1, diags, must.Sprintf("%+v", diags))
		test.Eq(t, diag.CodeTourLinkStepUnknown, diags[0].Code)
		test.Eq(t, diag.SeverityError, diags[0].Severity)
		test.Eq(t, "/steps/0/body", diags[0].Pointer)
	})

	T.Run("a name that fits two tours is a warning", func(t *testing.T) {
		t.Parallel()

		diags := check(t, map[string]string{cronPath: cron, "/r/.tours/dev/cron.tour": cron, tourPath: `{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "[a](docent:tour/cron.tour) and [b](docent:tour/ops/cron.tour)" }
		]}`})
		must.Len(t, 1, diags, must.Sprintf("%+v", diags))
		test.Eq(t, diag.CodeTourLinkAmbiguous, diags[0].Code)
		test.StrContains(t, diags[0].Message, "/r/.tours/dev/cron.tour")
	})

	T.Run("a nextTour that is no other tour's title is a warning", func(t *testing.T) {
		t.Parallel()

		for _, next := range []string{"Crons", "t"} {
			diags := check(t, map[string]string{cronPath: cron, tourPath: `{"title": "t", "nextTour": "` + next + `", "steps": [
				{ "description": "d" }
			]}`})
			must.Len(t, 1, diags, must.Sprint(next))
			test.Eq(t, diag.CodeNextTourUnknown, diags[0].Code)
			test.Eq(t, diag.SeverityWarning, diags[0].Severity)
			test.Eq(t, "/nextTour", diags[0].Pointer)
		}
	})

	T.Run("a tour of the same folder is meant before a namesake elsewhere", func(t *testing.T) {
		t.Parallel()

		const elsewhere = "/other/.tours/cron.tour"

		store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{
			cronPath:                  []byte(cron),
			elsewhere:                 []byte(`{"docent": 1, "title": "Other cron", "steps": [{ "id": "far", "description": "d" }]}`),
			"/other/.tours/only.tour": []byte(`{"docent": 1, "title": "Only", "steps": [{ "id": "there", "description": "d" }]}`),
			tourPath: []byte(`{"docent": 1, "title": "t", "nextTour": "Only", "steps": [
				{ "id": "a", "description": "[a](docent:tour/cron.tour#scheduling) [b](docent:tour/only.tour#there) [c](docent:tour/cron.tour#far)" }
			]}`),
		}}
		discover := func(context.Context) ([][]string, error) {
			return [][]string{{cronPath, tourPath}, {elsewhere, "/other/.tours/only.tour"}}, nil
		}

		diags, err := NewValidator(store, walk.NewWalker(&walk.Dependencies{Source: source}), discover, nil, nil).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags, must.Sprintf("%+v", diags))
		test.Eq(t, diag.CodeTourLinkStepUnknown, diags[0].Code)
		test.StrContains(t, diags[0].Message, `"far"`)
	})

	T.Run("the other tours are read once, until Reset", func(t *testing.T) {
		t.Parallel()

		store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{
			tourPath: []byte(`{"title": "t", "nextTour": "Cron", "steps": [{ "description": "d" }]}`),
		}}
		discover := func(ctx context.Context) ([][]string, error) {
			paths, err := store.Discover(ctx, "/r")
			return [][]string{paths}, err
		}
		v := NewValidator(store, walk.NewWalker(&walk.Dependencies{Source: source}), discover, nil, nil)

		diags, err := v.Tour(t.Context(), tourPath)
		must.NoError(t, err)
		must.Len(t, 1, diags)

		store.Files[cronPath] = []byte(cron)

		diags, err = v.Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 1, diags)

		v.Reset()

		diags, err = v.Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 0, diags)
	})

	T.Run("with no way to list the tours, links between them are not checked", func(t *testing.T) {
		t.Parallel()

		store := &tour.MemoryStore{Root: "/r", Files: map[string][]byte{
			tourPath: []byte(`{"title": "t", "nextTour": "Cron", "steps": [{ "description": "[a](docent:tour/gone.tour)" }]}`),
		}}

		diags, err := NewValidator(store, walk.NewWalker(&walk.Dependencies{Source: source}), nil, nil, nil).Tour(t.Context(), tourPath)
		must.NoError(t, err)
		test.Len(t, 0, diags)
	})
}
