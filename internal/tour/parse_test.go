package tour

import (
	"testing"

	"github.com/primandproper/docent/internal/diag"

	"github.com/shoenig/test"
	"github.com/shoenig/test/must"
)

const docentTour = `{
  "$schema": "https://raw.githubusercontent.com/primandproper/docent/main/schema/tour.v1.json",
  "docent": 1,
  "title": "Cache invalidation, end to end",
  "description": "How a write in the API evicts entries in the edge cache.",
  "repos": {
    "api":    { "remote": "git@github.com:acme/api.git",    "ref": "main", "stamp": "3f9c2e1" },
    "worker": { "remote": "git@github.com:acme/worker.git", "ref": "main", "stamp": "a81d04b" }
  },
  "vars": { "env": { "choices": ["staging", "prod"], "default": "staging" } },
  "map": { "file": "cache-invalidation/map.mmd" },
  "tags": ["cache"],
  "generatedBy": { "agent": "claude-code", "at": "2026-11-16T10:00:00Z" },
  "reviewedBy": [{ "name": "Jeffrey", "at": "2026-11-17T09:00:00Z", "ref": "3f9c2e1" }],
  "futureField": { "kept": true },
  "steps": [
    {
      "id": "write-path",
      "title": "The write handler",
      "repo": "api",
      "file": "internal/http/items.go",
      "pattern": "^func \\(h \\*Handler\\) UpdateItem",
      "line": 88,
      "description": "Every item update enters here.",
      "body": "cache-invalidation/01-write-path.md",
      "mapFocus": ["api"],
      "anchors": [
        { "label": "test", "repo": "api", "file": "internal/http/items_test.go", "pattern": "^func TestUpdateItem" }
      ],
      "links": [
        { "kind": "grafana", "title": "Update latency p99", "url": "{{grafana}}/d/abc123/items?var-env={{env}}" }
      ],
      "visual": { "type": "sequence", "participants": ["api", "queue"], "messages": [{ "from": "api", "to": "queue", "label": "enqueue" }] },
      "layout": "split",
      "transition": "call",
      "ref": "v2.0.0"
    }
  ]
}`

func codes(ds []diag.Diagnostic) []diag.Code {
	out := make([]diag.Code, len(ds))
	for i := range ds {
		out[i] = ds[i].Code
	}

	return out
}

func TestParse(T *testing.T) {
	T.Parallel()

	T.Run("decodes the design doc's example", func(t *testing.T) {
		t.Parallel()

		parsed, diags, err := Parse("x.tour", []byte(docentTour))
		must.NoError(t, err)
		must.Len(t, 0, diags, must.Sprintf("%+v", diags))

		test.True(t, parsed.IsDocent())
		test.Eq(t, "Cache invalidation, end to end", parsed.Title)
		test.Eq(t, []Repo{
			{Name: "api", Remote: "git@github.com:acme/api.git", Ref: "main", Stamp: "3f9c2e1"},
			{Name: "worker", Remote: "git@github.com:acme/worker.git", Ref: "main", Stamp: "a81d04b"},
		}, parsed.Repos)
		test.Eq(t, []Var{{Name: "env", Default: "staging", Choices: []string{"staging", "prod"}}}, parsed.Vars)
		test.Eq(t, "cache-invalidation/map.mmd", parsed.Map.File)
		test.Eq(t, []string{"cache"}, parsed.Tags)
		test.Eq(t, "claude-code", parsed.GeneratedBy.Agent)
		test.Eq(t, "Jeffrey", parsed.ReviewedBy[0].Name)

		must.Len(t, 1, parsed.Steps)
		step := parsed.Steps[0]
		test.Eq(t, "write-path", step.ID)
		test.Eq(t, "api", step.Repo)
		test.Eq(t, 88, step.Line)
		test.Eq(t, `^func \(h \*Handler\) UpdateItem`, step.Pattern)
		test.Eq(t, "cache-invalidation/01-write-path.md", step.Body)
		test.Eq(t, []string{"api"}, step.MapFocus)
		test.Eq(t, "test", step.Anchors[0].Label)
		test.Eq(t, "grafana", step.Links[0].Kind)
		test.Eq(t, "split", step.Layout)
		test.Eq(t, "call", step.Transition)
		test.Eq(t, "v2.0.0", step.Ref)
		test.StrContains(t, string(step.Visual), `"type":"sequence"`)
		test.Eq(t, "write-path", parsed.StepLabel(0))

		// Unknown fields survive in the raw document.
		test.NotNil(t, parsed.Raw.Get("futureField"))
	})

	T.Run("a plain CodeTour file ignores extension fields, with a warning", func(t *testing.T) {
		t.Parallel()

		parsed, diags, err := Parse("x.tour", []byte(`{
			"title": "t",
			"tags": "not even the right type",
			"steps": [{ "description": "d", "file": "a.go", "line": 3, "id": "Whatever They Like", "repo": 7 }]
		}`))
		must.NoError(t, err)
		test.False(t, parsed.IsDocent())
		test.Eq(t, []diag.Code{diag.CodeExtensionIgnored, diag.CodeExtensionIgnored, diag.CodeExtensionIgnored}, codes(diags))
		test.False(t, diag.HasErrors(diags))
		test.Eq(t, "/steps/0/id", diags[1].Pointer)
		test.Eq(t, "#1", diags[1].Step)
		test.Eq(t, "", parsed.Steps[0].ID)
		test.Nil(t, parsed.Tags)
		test.Eq(t, 3, parsed.Steps[0].Line)
	})

	T.Run("invalid JSON is one diagnostic and no tour", func(t *testing.T) {
		t.Parallel()

		parsed, diags, err := Parse("x.tour", []byte(`{ "title": "t", // comment`))
		must.NoError(t, err)
		test.Nil(t, parsed)
		test.Eq(t, []diag.Code{diag.CodeJSONSyntax}, codes(diags))
	})

	T.Run("a non-object document is a schema error and no tour", func(t *testing.T) {
		t.Parallel()

		parsed, diags, err := Parse("x.tour", []byte(`[]`))
		must.NoError(t, err)
		test.Nil(t, parsed)
		test.Eq(t, []diag.Code{diag.CodeSchema}, codes(diags))
	})

	T.Run("schema errors point at the offending value and still decode the rest", func(t *testing.T) {
		t.Parallel()

		parsed, diags, err := Parse("x.tour", []byte(`{
			"title": "t",
			"steps": [
				{ "description": "fine", "file": "a.go" },
				{ "file": "b.go", "line": "12" }
			]
		}`))
		must.NoError(t, err)
		must.NotNil(t, parsed)
		must.Len(t, 2, diags, must.Sprintf("%+v", diags))

		pointers := []string{diags[0].Pointer, diags[1].Pointer}
		test.SliceContainsAll(t, pointers, []string{"/steps/1", "/steps/1/line"})
		test.Eq(t, "#2", diags[0].Step)
		test.Eq(t, diag.SeverityError, diags[0].Severity)
		test.Eq(t, 0, parsed.Steps[1].Line)
		test.Eq(t, "b.go", parsed.Steps[1].File)
	})

	T.Run("docent tours need a step id", func(t *testing.T) {
		t.Parallel()

		_, diags, err := Parse("x.tour", []byte(`{"docent": 1, "title": "t", "steps": [{ "description": "d" }]}`))
		must.NoError(t, err)
		test.Eq(t, []diag.Code{diag.CodeSchema}, codes(diags))
		test.Eq(t, "/steps/0", diags[0].Pointer)
	})

	T.Run("docent tours need unique step ids", func(t *testing.T) {
		t.Parallel()

		_, diags, err := Parse("x.tour", []byte(`{"docent": 1, "title": "t", "steps": [
			{ "id": "a", "description": "d" },
			{ "id": "a", "description": "d" }
		]}`))
		must.NoError(t, err)
		test.Eq(t, []diag.Code{diag.CodeStepIDDuplicate}, codes(diags))
		test.Eq(t, "/steps/1/id", diags[0].Pointer)
	})

	T.Run("an unknown docent version is rejected", func(t *testing.T) {
		t.Parallel()

		parsed, diags, err := Parse("x.tour", []byte(`{"docent": 2, "title": "t", "steps": []}`))
		must.NoError(t, err)
		test.False(t, parsed.IsDocent())
		test.Eq(t, []diag.Code{diag.CodeSchema}, codes(diags))
		test.Eq(t, "/docent", diags[0].Pointer)
	})

	T.Run("extension fields are type-checked in docent tours", func(t *testing.T) {
		t.Parallel()

		_, diags, err := Parse("x.tour", []byte(`{"docent": 1, "title": "t", "tags": "nope", "steps": [
			{ "id": "a", "description": "d", "links": [{ "kind": "slack", "title": "x", "url": "https://x" }] }
		]}`))
		must.NoError(t, err)

		pointers := make([]string, len(diags))
		for i := range diags {
			pointers[i] = diags[i].Pointer
		}

		test.Eq(t, []string{"/steps/0/links/0/kind", "/tags"}, pointers)
		test.Eq(t, "a", diags[0].Step)
		test.Eq(t, "", diags[1].Step)
	})
}
