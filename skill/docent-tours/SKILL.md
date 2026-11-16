---
name: docent-tours
description: Write or refresh a Docent code tour (a .tour file that walks a reader through code across one or more repos). Use when asked for a code tour, a walkthrough of a feature, or to update tours that have fallen behind the code.
---

# Writing a Docent tour

A tour walks a reader through code in narrative order: one step at a time, each anchored to real lines, with prose
beside it. You write the tour as files; a person walks it in the Docent app and fixes what you got wrong. This guide
is everything you need to produce one that validates.

## The loop

1. Explore every repo the tour touches before writing anything.
2. `docent new <slug>` scaffolds `.tours/<slug>.tour` and `.tours/<slug>/` for step bodies.
3. Write the system map first (see below): it forces you to decide what the systems are and how they connect.
4. Write steps. After each one, run `docent resolve .tours/<slug>.tour --step <id>` and read what it prints. That is
   the code the reader will see highlighted. If it isn't what you meant, fix the anchor.
5. Run `docent validate --strict --json .tours/<slug>.tour` and fix every diagnostic. Repeat until it prints an empty
   `diagnostics` array and exits 0.
6. Run `docent stamp --hashes .tours/<slug>.tour`. It records the commit of each repo the tour is correct against,
   which is what lets Docent tell a reader later how far the code has moved.

Never guess a line number. Never finish with warnings.

Docent reads each repo at its `ref` as of the last fetch, not at whatever is checked out, so uncommitted edits don't
count. Pass `--working-tree` to `validate` and `resolve` only if you are deliberately writing against uncommitted
code.

## Shape of a good tour

- Pick a reader and a depth: 5 to 8 steps for a quick look, 9 to 15 for a feature end to end.
- Tell a path, not an inventory: the entry point, then each hop across systems in the order data moves, then one edge
  case, then where to go next.
- One idea per step. If a step needs two highlighted regions, it is two steps.
- Open with a content-only step (no `file`) that says what the reader will understand by the end.
- Say why the code is the way it is. The reader can already see what it does.

## The file

A tour is JSON: strict, no comments. `"docent": 1` turns on everything beyond plain CodeTour, and then every step
needs an `id`.

```json
{
  "$schema": "https://raw.githubusercontent.com/primandproper/docent/main/schema/tour.v1.json",
  "docent": 1,
  "title": "Cache invalidation, end to end",
  "description": "How a write in the API evicts entries in the edge cache.",
  "repos": {
    "api": { "remote": "git@github.com:acme/api.git", "ref": "main" },
    "worker": { "remote": "git@github.com:acme/worker.git", "ref": "main" }
  },
  "vars": { "env": { "choices": ["staging", "prod"], "default": "staging" } },
  "generatedBy": { "agent": "your-name", "model": "your-model", "at": "2026-11-16T10:00:00Z" },
  "steps": [
    {
      "id": "overview",
      "title": "What happens on a write",
      "description": "A write has to reach three systems before a reader sees it.",
      "body": "cache-invalidation/01-overview.md"
    },
    {
      "id": "write-path",
      "title": "The write handler",
      "repo": "api",
      "file": "internal/http/items.go",
      "pattern": "^func \\(h \\*Handler\\) UpdateItem",
      "line": 88,
      "description": "Every item update enters here.",
      "body": "cache-invalidation/02-write-path.md",
      "links": [
        { "kind": "grafana", "title": "Update latency p99", "url": "{{grafana}}/d/abc123/items?var-env={{env}}" }
      ]
    }
  ]
}
```

Run `docent schema` for the full JSON Schema.

### Tour fields

| Field | Meaning |
| --- | --- |
| `title`, `description` | Shown in the tour picker. Required: `title`. |
| `repos` | The repos the tour touches, by a short name you choose. `remote` is the git URL. Docent finds each one on the reader's machine by that URL, so use the real one (`git remote get-url origin`). `ref` is the branch the tour follows (usually `main`); use a tag or commit only for a tour about how something worked at that point. Leave `stamp` to `docent stamp`. |
| `vars` | Variables for link URLs: `{ "default"?, "choices"?, "description"? }`. |
| `map` | The system map: `{ "file": "<slug>/map.mmd" }` (preferred) or `{ "mermaid": "..." }`. |
| `tags` | Strings for filtering in the picker. |
| `generatedBy` | Always set it: `{ "agent", "model"?, "at" }` with an RFC 3339 time. It marks the tour as an unreviewed draft. |
| `reviewedBy` | Never set it. A person adds it after walking the tour. |
| `nextTour` | The `title` of a tour to offer after the last step. |

### Step fields

| Field | Meaning |
| --- | --- |
| `id` | A lowercase slug (`write-path`), unique in the tour. Required. Keep it stable when you reorder steps. |
| `title` | Short, shown in the step rail. |
| `description` | Required. One line saying what the step is about. Shown when there is no `body`. |
| `body` | A Markdown file, relative to the tour file, with the step's full narrative. Use it for anything longer than a sentence or two so you never write paragraphs inside a JSON string. |
| `repo` | A key of `repos`. `file` and `directory` are then relative to that repo's root. Without it they are relative to the folder that contains `.tours/`. |
| `file` | The file to show. |
| `pattern` + `line` | Where in the file. See below. |
| `selection` | `{ "start": { "line", "character" }, "end": { "line", "character" } }` to highlight a range of lines instead of one. With a `pattern`, it moves with the match. |
| `directory` | Show a folder's listing and README instead of a file. |
| `uri` | Show a single web link instead of code. |
| `links` | Link cards: `{ "kind", "title", "url", "description"? }`. |
| `mapFocus` | Ids of the map nodes and edges this step is about. Give every step that touches a system one. |
| `visual` | One animated visual, shown above the narrative. See below. |
| `ref` | Read this one step at a different ref, to show code as it was before a change. |

A step with none of `file`, `directory` or `uri` is content only: the narrative takes the whole window, like a slide.

## Anchoring

Anchor every file step with `pattern` and `line` together.

- `pattern` is a Go RE2 regular expression tested against each line on its own. There is no lookahead, lookbehind or
  backreference. Remember that the JSON string needs its backslashes doubled: `"^func \\(h \\*Handler\\) Update"`.
- `line` is the line you saw the pattern on. It breaks ties when the pattern matches more than once, and it is how
  Docent tells the reader the code has moved since you wrote the tour.
- Make the pattern specific enough to survive edits elsewhere in the file: a function signature, a type declaration,
  a distinctive constant. Not `return err`.
- To highlight a whole function, add a `selection` from its first line to its last.

Then check it:

```sh
docent resolve .tours/cache-invalidation.tour --step write-path
```

It prints the status, the file and the anchored lines. `ok` is the only acceptable status in a tour you hand over.
`moved` means `line` is wrong: set it to the line the output shows. `ambiguous` means the pattern matches several
lines and there is no `line`. `missing` means the file, the repo or the pattern wasn't found, and the output says
which.

## The system map

The map is one Mermaid flowchart per tour, shown beside every step with the current step's part highlighted. Keep it
to the systems the tour visits: 4 to 9 nodes.

```
flowchart LR
    client([Client]) --> api[API]
    api e1@--> queue[(Queue)]
    queue e2@--> worker[Worker]
    worker e3@--> cache[(Edge cache)]
```

- A step's `mapFocus` lists node ids (`api`) and edge ids (`e1`).
- An edge can only be focused if it is named with Mermaid's edge-id syntax: `api e1@--> queue`. Name every edge a
  step is about.
- A step about a hop between systems focuses the edge and both ends: `["api", "e1", "queue"]`.

Clicking a node takes the reader to the first step that focuses it, so the map is also the tour's index.

## Visuals

Use a visual when order or timing is the point: an async hop, retries, a state changing. Use a Mermaid diagram (a fence
named `mermaid` in a body) for structure that doesn't move. Don't decorate: a step that is clear from its
code needs neither.

Write a visual as a fence in a body file. The body of the fence is YAML.

````
```docent-sequence
participants: [client, api, queue, worker, cache]
messages:
  - { from: client, to: api, label: "PUT /items/42" }
  - { from: api, to: queue, label: "enqueue invalidate(42)", async: true, step: enqueue }
  - { from: queue, to: worker, label: "deliver" }
  - { from: worker, to: cache, label: "PURGE /items/42", step: purge }
```
````

| Fence | Shows | Fields |
| --- | --- | --- |
| `docent-sequence` | Messages between participants, in order | `participants`, `messages[{from, to, label, async?, step?}]` |
| `docent-flow` | A packet moving along the edges of a graph | `nodes`, `edges[{from, to, label?}]`, `paths[{label, through[], step?}]` |
| `docent-state` | A state machine stepping through events | `states` (the first is the start), `transitions[{from, to, on}]`, `run[]` (the events to play) |
| `docent-timeline` | Events across lanes on a time axis | `lanes`, `events[{lane, at, label, step?}]`, `unit?` |

All four take an optional `title`. Anywhere a list of nodes is wanted (`participants`, `nodes`, `states`, `lanes`),
an item is an id (`api`) or `{ id: api, label: "The API" }`. `step` is the id of the tour step a frame belongs to;
the reader gets a button to jump there. Every id a visual refers to must be one it declares, each consecutive pair in
a flow path's `through` needs an edge, and a state machine's `run` must be playable from its first state.

The same object, with `"type": "sequence"` (or `flow`, `state`, `timeline`) added, can be a step's `visual` field.

## Links

Whenever the narrative mentions a metric, a log query, an alert, a runbook, a PR or an issue, add a link card so the
reader can go and look. `kind` is one of `grafana`, `logs`, `traces`, `runbook`, `pr`, `issue`, `doc` or `url`.

Never hard-code an environment or an internal hostname. Use `{{name}}` variables: declare ones the reader chooses
between (like `env`) under the tour's `vars` with `choices` and a `default`, and leave hosts (like `grafana`) to the
reader's `docent.json`. A variable with no value anywhere is a validation warning, and its card shows disabled.

Only `http` and `https` URLs are allowed, plus Docent's own:

- `docent:step/<id>` jumps to a step of this tour.
- `docent:tour/<file>#<id>` opens another tour (named by the end of its path, like `cache-invalidation.tour`) at a
  step.

Both work as link cards and as ordinary Markdown links in a body: `[the write handler](docent:step/write-path)`.

Cards can also be written inside a body, where they appear in the flow of the text:

````
```docent-links
- { kind: runbook, title: Cache runbook, url: "{{wiki}}/runbooks/cache" }
```
````

## Body files

A body is plain Markdown (CommonMark with tables). Raw HTML is shown as text, and images that aren't embedded are not
loaded. Write for someone reading beside the highlighted code: refer to what they can see, keep paragraphs short, and
use a list when order matters.

Name bodies so they sort in step order: `01-overview.md`, `02-write-path.md`.

## Refreshing a tour that has fallen behind

`docent stale --json` lists the tours whose code has moved: for each, why, and the diff since the stamp of every file
a step anchors into. For each listed tour:

1. Read the diffs. Decide, step by step, whether the narrative is still true.
2. Fix anchors (`moved`: update `line`), rewrite steps whose code `changed`, and re-anchor or drop steps whose code
   is `missing`. The diagnostic names the commit that removed it, which usually says what replaced it.
3. Remove `reviewedBy` and set `generatedBy` again: the tour is a draft until a person walks it.
4. Validate until clean, then `docent stamp --hashes`.

`docent stamp` refuses while any anchor has drifted. That is deliberate: stamping says "this is right as of now".

## Diagnostics

`docent validate --json` prints `{ "diagnostics": [...], "tours", "errors", "warnings" }`. Each diagnostic is
`{ file, pointer, step, severity, code, message, hint }`, where `pointer` is a JSON Pointer to the offending value
(`/steps/3/pattern`) and `step` is the step's id.

| Code | What to do |
| --- | --- |
| `json-syntax` | The file isn't valid JSON. |
| `schema` | A field has the wrong type or a required one is missing. The message says which. |
| `step-id-duplicate` | Two steps share an `id`. |
| `repo-unknown` | A step's `repo` isn't a key of `repos`. |
| `repo-missing` | The repo isn't checked out on this machine. Tell the person; don't work around it. |
| `file-missing` | The path is wrong. Check what it is relative to. |
| `path-escapes-root` | The path leaves its repo. Paths can't use `..` to climb out. |
| `pattern-invalid` | Not valid RE2. |
| `pattern-no-match` | The pattern matches no line of that file. |
| `pattern-ambiguous` | Several lines match. Add `line` or tighten the pattern. |
| `anchor-moved` | Set `line` to where the pattern matched. |
| `line-out-of-range` | `line` or `selection` is past the end of the file. |
| `body-missing` | The `body` path is wrong. It is relative to the tour file. |
| `link-var-unresolved` | Declare the variable in `vars` with a default, or tell the person which value to configure. |
| `link-scheme` | Not an http(s) URL. |
| `map-missing` | The `map` file path is wrong (it is relative to the tour file), or it isn't a flowchart. |
| `map-focus-unknown` | A `mapFocus` id isn't a node, or an edge named with `id@`, in the map. |
| `visual-invalid` | A visual doesn't match its schema or refers to an id it doesn't declare. The message says where. |
| `step-link-unknown` | A `docent:step/` link or a visual's `step` names a step that doesn't exist. |
| `content-changed` | The anchored code was edited since the stamp. Check the step is still true. |
| `code-removed` | The anchored code is gone. `docent stale` shows what replaced it. |
| `stamp-unreachable` | The stamp commit isn't in the local repo. Tell the person to fetch. |
| `ref-missing` | A pinned commit isn't in the local repo. Tell the person to fetch. |
| `extension-ignored` | The tour uses Docent fields without `"docent": 1`. |
