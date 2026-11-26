# Docent: Product Design Document

Nov 16, 2026 · @Jeffrey

## Summary

Docent is a native desktop app (macOS only in v1) for walking guided tours of code that span several repos and systems. You run `docent .` in a folder, pick a tour, and step through narrative, highlighted code, diagrams, animated visuals and links to live systems. Tours are CodeTour-compatible `.tour` JSON files with additive extensions, one tour per subject. Agents draft tours; people walk them, fix them and share them through git.

**Problem.** AI assistance makes it easy to change a system without building a mental model of it. Understanding a feature that crosses repos means jumping between editors, diagrams and dashboards with nothing holding the thread. CodeTour covers only the single-repo, in-editor case: no cross-repo anchors, no diagrams, and external links only as a bare URI step.

**Goals (v1)**

- Walk one tour across several local repos, each step anchored to real code at a known git ref.
- Make a system's shape visible: Mermaid diagrams, plus a persistent system map that highlights where the current step sits.
- Offer animated visuals (sequence playback, data flowing along edges, state stepping) from a built-in component library, declared as data, not code.
- Show typed link cards to external systems (Grafana, logs, traces, runbooks, PRs) with templated variables.
- Open existing `.tour` files unchanged. The Docent format is a superset of CodeTour's: every CodeTour file is a valid Docent tour, and Docent adds flat fields on top.
- Make agent authoring first-class: a JSON Schema, `docent validate --json`, and an authoring guide an agent can follow.
- Install from Homebrew; the cask also puts the `docent` CLI on the PATH.

**Non-goals (v1)**

- Learning mechanics such as quizzes, checkpoints or spaced repetition. Docent is a viewer.
- Editing code, an LSP, or go-to-definition.
- Embedded dashboards or live queries. The one exception is a feature flag's status, read through an integration the reader set up (see Integrations). Credentials never live in a tour or a config file.
- Author-supplied JavaScript or Svelte inside tours.
- A hosted web version or a sharing service. Sharing is git.
- Linux and Windows builds, and forges other than GitHub. Nothing in the design should preclude them later.
- Editing tours or config. The app never writes config (the one file of its own it keeps is the library, the list of folders the reader added; see Launch and discovery), and the only edit it makes to a tour is recording a review (**Mark reviewed**, below); where any other edit would help, it offers a command to copy. Its other writes are a `git clone` or `git fetch`. Each of these is offered, and happens only once the reader confirms.

## Users and use cases

The primary user is an engineer who needs a working mental model of a feature spread across many services. Secondary users are teammates who consume shared tours. Agents are authors, not readers.

| Use case | Who | Flow |
| --- | --- | --- |
| Understand a feature end to end | Engineer + agent | Ask an agent for a tour of a feature across repos A, B and C. The agent writes the `.tour`, loops on `docent validate --json` until it's clean, and the engineer walks it in Docent, fixes wrong steps, marks it reviewed and commits it. |
| Onboard a teammate | Teammate | Clone the tours repo and run `docent`. Repos resolve from local checkouts; Docent offers to clone any that are missing, and the teammate can decline. |
| Incident or RCA walkthrough | On-call engineer | Go from alert to dashboard link to the code that emits the metric to the handler to the downstream call, with the system map showing the path. |
| PR review tour | Reviewer | A tour pinned to a branch walks the change in narrative order rather than file order. |
| Read an existing CodeTour | Anyone | Open any repo with `.tours/` and walk its tours unchanged. |

## Product experience

The walk view is the product: a step rail, a read-only code pane, a narrative pane, and a system map that always shows where you are.

&#91;embedded content: walk view wireframe · step rail, code pane, system map, narrative\]

The highlighted map node and the highlighted code band always describe the same step; moving to the next step moves both.

**Launch and discovery.** `docent [path]` opens a window on the folder `path` (default `.`). Docent finds tours in `.tours/`, `.vscode/tours/` and `.github/tours/` (the CodeTour locations), plus any `tourPaths` in the workspace config. The tour picker lists each tour's title, description, the repos it touches, and its health: all anchors resolve, N drifted, or a repo is missing. A tour with `generatedBy` and no `reviewedBy` carries an **Unreviewed draft** badge.

**The library: several folders at once.** A company's tours rarely live in one place: repo A has its own, repo B has its own, and an org-wide tours repo explains how the two talk to each other. The picker therefore shows any number of folders, each as a group under its own name.

- **Add a folder…** (`⌘O`) asks for a folder in the system's dialog, opens it beside the others and saves it. Saved folders are there every time the app opens.
- A folder opened from outside (`docent <path>`, `docent open`, a `docent://` link) is shown beside the saved ones and marked **Open for now**. It is gone when the app quits, unless the reader presses **Keep**.
- **Remove** closes a folder and forgets it. Nothing on disk is deleted.
- Each folder keeps a context of its own: its `docent.json`, its variables, its editor rules, and the repos cloned beside it. A tour is always read with the configuration of the folder it was found in, so opening the org-wide tours next to repo A changes nothing about how repo A's tours resolve. A tour two folders can both see belongs to the one listed first.
- Links between tours (`docent:tour/…`, `nextTour`) reach across folders. A tour in the linking tour's own folder is meant before a namesake in another, so two folders can each have an `overview.tour`.
- The list is saved in `~/.config/docent/library.json`, a file of its own beside the user config. It is the one file the app writes that isn't a tour: a list of folder paths, each of which the reader picked in a dialog or was already looking at. `config.json` is still never written, because that is where editor commands come from. A saved folder that has since been deleted is listed with a note, so it can be removed.
- The CLI never reads the library. `docent validate`, `stale`, `stamp` and `review` see one folder, so their answers don't depend on what a machine happens to have saved. A link into a tour that lives in another folder is therefore a warning there (`tour-link-unknown`), not an error; the app, with both folders open, checks it fully.

**Walk view**

- **Step rail (left):** numbered step titles with a repo badge and a drift indicator per step.
- **Code pane (center):** read-only and syntax highlighted, scrolled to the anchor, with the anchored lines highlighted. A breadcrumb reads `repo › path › line` with the ref. Extra anchors on a step show as tabs (for example, "handler", "caller", "test"). A file-tree drawer browses the step's repo. Buttons: **Open in editor** and **Copy permalink** (a forge URL at the pinned ref).
- **Narrative pane (right):** rendered Markdown with inline Mermaid diagrams, animated visuals and link cards.
- **System map (collapsible, docked above the narrative):** one diagram per tour. The current step's nodes and edges are highlighted and visited ones are dimmed. Clicking a node jumps to the first step that focuses it. It can zoom in on what the current step focuses and follow the tour from there, take the centre pane, or be popped out into a window of its own that floats above the main one and still follows, and drives, the tour.
- **Content-only steps** hide the code pane and give the narrative full width, which works like a slide.

**Keyboard:** `j`/`k` or arrow keys move between steps, `g` goes to a step, `o` opens in editor, `m` toggles the map, `z` zooms it in on the current step, `⌃Z` pops it out into its own window, `/` searches tours, `⌘R` reloads, and `⌘[`/`⌘]` (or `⌘←`/`⌘→`, and the arrows in the header) move back and forward through navigation history. Stepping through a tour moves the current history entry along instead of adding one, so going back from a tour reached by a link returns to the step the link was on, then to wherever came before, and in the end to the tour picker.

**Live reload.** Docent watches tour files and their referenced Markdown. On change, it re-parses and keeps your position by step `id` (by index if there's no id). This makes the app the preview loop for both agents and people editing by hand.

**Choosing an editor.** The reader picks the editor, never the tour. A shared tour can't know who has IntelliJ installed, and an editor command inside a tour would be code execution from untrusted input.

- **Detection.** At launch, and again whenever the window regains focus, Docent detects installed editors two ways: CLI shims on the `PATH` (`code`, `cursor`, `zed`, `idea`, `goland`, `subl`, `nvim`), and app bundles found by bundle ID through Launch Services. That way, a JetBrains IDE without its shell script still counts. The results fill an **Open in…** menu.
- **Rules give per-step behavior for free.** User config maps file globs, languages or repos to an editor, and the first match wins. A Java step opens in IDEA and a TypeScript step in VS Code without the tour saying anything. Workspace-specific rules can live in `docent.json`. You edit rules by hand, since the app never writes config.
- **One-off override** is a UI action, not a tour field. `o` uses the rules; `⇧O` opens the **Open in…** menu for just this step. A **Copy rule** item puts a ready-made rule on the clipboard to paste into your config.
- **Editor definitions.** Built-in presets know each editor's line-jump syntax and launch method (its CLI, or `open -b <bundle id> --args`). Custom editors are argv templates. Terminal editors need a terminal wrapper.
- **A missing editor never fails silently.** Before launching, Docent confirms the editor still exists (bundle lookup or `exec.LookPath`). If it's gone, a toast says so (for example, "IntelliJ IDEA isn't installed anymore") with buttons for the next detected editor and the full menu. Without input, Docent falls through: next matching rule, then `default`, then the macOS default app for the file (`open`), then reveal the file in Finder and copy `path:line` to the clipboard.

```json
{
  "editors": {
    "rules": [
      { "match": "**/*.{java,kt}", "use": "idea" },
      { "repo": "infra", "use": "zed" },
      { "match": "**/*.go", "use": "goland" }
    ],
    "default": "code",
    "custom": {
      "helix": { "argv": ["ghostty", "-e", "hx", "{path}:{line}:{col}"] }
    }
  }
}
```

## Tour file format

A tour is a CodeTour `.tour` JSON file with additive, flat fields; one file per subject. Every valid CodeTour file is a valid Docent tour. Long-form prose can live in sibling Markdown files, so authors never write multi-paragraph Markdown inside JSON strings.

**Layout on disk**

```text
.tours/
  cache-invalidation.tour          # the tour (JSON)
  cache-invalidation/
    map.mmd                        # system map (Mermaid)
    01-write-path.md               # step bodies (Markdown)
    02-enqueue.md
```

**CodeTour fields** ([schema](https://git.cynarski.dev/Awesome/awesome-copilot/raw/branch/main/skills/code-tour/references/codetour-schema.json))

- Supported as-is: `title`, `description`, `ref`, `isPrimary`, `nextTour`, `steps[].title`, `description`, `file`, `directory`, `uri`, `line`, `pattern`, `selection`.
- Kept in the file but ignored by Docent: `view`, `commands`, `stepMarker` (VS Code specific) and `when`. `when` is a JavaScript expression and is never evaluated.
- `file` and `directory` resolve relative to the **tour root**, which is the directory containing `.tours/`. That's CodeTour's workspace root.

**Tour-level extensions**

| Field | Type | Purpose |
| --- | --- | --- |
| `docent` | integer | Format version, currently `1`. Required to use any field below. |
| `repos` | map of name → `{remote, ref?, stamp?, subdir?}` | The repos the tour touches. `remote` is the canonical git URL; `ref` is a branch to track or a commit to pin; stamp is the commit the tour was written against. |
| `vars` | map of name → `{default?, choices?, description?}` | Template variables for link URLs (for example, `env`). |
| `map` | `{mermaid}` or `{file}` | The system map. Its node ids, and the ids of edges named with Mermaid's `id@` syntax, are what `mapFocus` refers to. |
| `tags` | string\[\] | Filtering in the tour picker. |
| `generatedBy` | `{agent, model?, at}` | Provenance of an AI-drafted tour. |
| `reviewedBy` | `[{name, at, ref?}]` | Who walked the tour and vouched for it, at which ref. |

**Step-level extensions**

| Field | Type | Purpose |
| --- | --- | --- |
| `id` | slug | Stable identity for deep links and live reload. Unique within the tour, and required when `docent` is set. |
| `repo` | string | A key in `repos`. When set, `file` and `directory` are relative to that repo's root. |
| `body` | path | A Markdown file, relative to the tour file, that Docent renders instead of `description`. `description` stays required and acts as the one-line summary CodeTour shows. |
| `anchors` | Anchor\[\] | Extra code references shown as tabs: `{label, repo?, file, line?, selection?, pattern?}`. |
| `mapFocus` | string\[\] | Map node and edge ids to highlight on this step. Edges are named in the map with Mermaid's own edge-id syntax (`api e1@--> queue`), so `e1` is the edge id. |
| `links` | Link\[\] | Link cards: `{kind, title, url, description?}`. `kind` is one of `grafana`, `logs`, `traces`, `runbook`, `pr`, `issue`, `doc`, `url` or `flag`. A `flag` card names a feature flag instead of a URL: `{kind, title, flag, project?, url?, description?}` (see Integrations). |
| `visual` | Visual | One structured visual (same schema as the `docent-*` fences) rendered above the narrative. |
| `layout` | `split` \| `narrative` \| `code` | Pane-layout override. |
| `transition` | `call` | Opt-in animation on arriving from the step before: that step's highlighted code calls the function this step highlights, and the function opens out of its call site. It only plays when the call is on screen. |
| `ref` | string | A per-step ref override, for example to show code before a change. |

**Example**

```json
{
  "$schema": "https://raw.githubusercontent.com/<owner>/docent/main/schema/tour.v1.json",
  "docent": 1,
  "title": "Cache invalidation, end to end",
  "description": "How a write in the API evicts entries in the edge cache.",
  "repos": {
    "api":    { "remote": "git@github.com:acme/api.git",    "ref": "main", "stamp": "3f9c2e1" },
    "worker": { "remote": "git@github.com:acme/worker.git", "ref": "main", "stamp": "a81d04b" }
  },
  "vars": { "env": { "choices": ["staging", "prod"], "default": "staging" } },
  "map": { "file": "cache-invalidation/map.mmd" },
  "generatedBy": { "agent": "claude-code", "at": "2026-11-16T10:00:00Z" },
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
      ]
    }
  ]
}
```

**Compatibility rules**

- **Decided:** the format is a superset of CodeTour's, with Docent's extensions as flat top-level fields rather than an `x-docent` object. Every CodeTour file is a valid Docent tour. Nobody on the team uses VS Code CodeTour, so Docent tours don't need to work in VS Code, and `repo` is used freely.
- If M0 shows a superset can't hold (a Docent field collides with what a CodeTour field means, or real-world `.tour` files don't fit one model), Docent defines its own format and keeps reading `.tour` files as input.
- The parser still preserves unknown fields and key order, so `docent fmt` round-trips files without data loss.

## Multi-repo workspace model

A tour names repos by logical name and git remote; each machine maps those names to local checkouts. The default, zero-config case is the common one: every repo cloned side by side in one folder.

**Repo resolution order** (first match wins, and each result is cached per session):

1. An explicit mapping in the workspace file, `docent.json`, found in the opened folder or its nearest ancestor: `{"repos": {"api": "./api"}}`.
2. An explicit mapping in user config, `~/.config/docent/config.json`.
3. Convention: a directory named after the remote's repo name (`api` for `acme/api.git`) under the opened folder or any configured `repoRoots`.
4. Remote match: scan one level deep in the opened folder and `repoRoots` for git repos whose `origin` URL matches `remote`. SSH and HTTPS forms are normalized, as is a trailing `.git`.
5. Not found: the step shows **Repo missing** and offers to clone it (`git clone <remote> <first repo root>/<name>`). Docent clones only when the reader confirms; declining leaves the step missing, with the clone command available to copy.

**Workspace file** (`docent.json`, JSONC allowed)

```json
{
  "repoRoots": ["~/work"],
  "repos": { "api": "~/work/api-service" },
  "tourPaths": ["~/work/team-tours"],
  "vars": { "grafana": "https://grafana.example.com" },
  "github": { "hosts": ["github.com"] }
}
```

**Refs: track by default, pin on purpose**

- A `ref` that names a branch (`main`) is **tracking**. Docent resolves it at the branch tip as of your last fetch, so the tour follows the code as it changes. A commit SHA or tag is **pinned**. Use pinning on purpose, for PR tours or "how it worked in v2" tours.
- Each repo entry also records a `stamp`: the commit the tour was written or last reviewed against. Agents set it when they write a tour; `docent stamp` and `docent review` refresh it. The stamp is what lets Docent follow anchors forward through later changes (see drift).
- Reads go through git at the resolved commit and never touch your checkout. A **working tree** toggle reads uncommitted files instead, which is useful while authoring.
- Docent fetches only when you agree to, so a tracked branch is as fresh as the last fetch. When that fetch is old, the banner shows its age and offers to fetch now; declining keeps the current refs, with the fetch command available to copy.
- If a tracked branch no longer exists (deleted after a squash-merge, say), Docent falls back to the repo's default branch and says so. A missing pinned commit shows **Ref missing** and offers to fetch it.
- A freshness banner sums up age and damage, for example: "Written against `3f9c2e1`, 412 commits / 9 months behind `main` · 3 anchors moved · 1 missing".

**Where tours live.** Cross-repo tours live in one dedicated tours repo (for example, `team-tours`), listed in `tourPaths` so they show up wherever Docent opens. Scattering cross-repo tours among service repos leaves files that show half a story with the context missing, so `docent validate` warns when a tour inside a service repo anchors into other repos. Single-repo tours may still live in that repo's `.tours/`.

## Rich content

Step bodies are Markdown with three kinds of embeds: Mermaid diagrams, built-in animated visuals, and link cards. All of them are declared as data. Tours can't ship code, so a tour from a stranger is as safe to open as a Markdown file.

**Markdown.** CommonMark plus GFM (tables, task lists, footnotes). Raw HTML is stripped by default and sanitized with DOMPurify if it's ever enabled. Two link schemes are added: `docent:step/<id>` (jump within the tour) and `docent:tour/<path>#<id>` (another tour). In CodeTour these degrade to dead links, which is acceptable.

**Mermaid.** A ```` ```mermaid ```` fence renders with a pinned Mermaid version, themed for light and dark, at `securityLevel: strict`. Mermaid `click` directives with a `docent:` target become step jumps; any other target is dropped.

**System map.** The tour's `map` is a Mermaid flowchart. Docent renders it once and toggles CSS classes on nodes and edges as `mapFocus` changes: `current`, `visited` and `upcoming`. Transitions are animated with Svelte, so moving between steps visibly travels across the system. An edge a step focuses must be named with Mermaid's edge-id syntax (`api e1@--> queue`); unnamed edges can't be focused. The parser validates every `mapFocus` id against the parsed map.

**Animated visuals.** These are built-in Svelte components selected by fence language. Each body is YAML, which also accepts JSON, and is validated by the same JSON Schema `docent validate` uses. All of them have play, pause, step and scrub controls, honor `prefers-reduced-motion`, and can link frames to steps.

| Fence | Shows | Key fields |
| --- | --- | --- |
| `docent-sequence` | Messages between participants, played in order | `participants`, `messages[{from, to, label, async?, step?}]` |
| `docent-flow` | "Packets" moving along edges of a graph (auto-layout via ELK) | `nodes`, `edges`, `paths[{label, through[]}]` |
| `docent-state` | A state machine stepping through transitions | `states`, `transitions[{from, to, on}]`, `run[]` |
| `docent-timeline` | Ordered events across lanes (for example, retries with backoff) | `lanes`, `events[{lane, at, label}]` |

```yaml
# inside a step body, fenced as docent-sequence
participants: [client, api, queue, worker, cache]
messages:
  - { from: client, to: api, label: "PUT /items/42" }
  - { from: api, to: queue, label: "enqueue invalidate(42)", async: true, step: enqueue }
  - { from: queue, to: worker, label: "deliver" }
  - { from: worker, to: cache, label: "PURGE /items/42", step: purge }
```

**Link cards.** Cards come from `links` on the step, or from a ```` ```docent-links ```` fence holding the same array. Each card shows a kind icon, the title, the resolved host and an optional description. Clicking opens the system browser; Docent never fetches the URL itself.

- Template syntax: `{{name}}`. Values come from, in priority order: a CLI flag (`--var env=prod`), the in-app variable picker (a dropdown when `choices` is set), the workspace `vars`, user config `vars`, then the tour default.
- Only `http`, `https` and `docent` schemes are allowed. A card with an unresolved variable renders disabled, with the missing name shown.
- A `flag` card is the one card that can show more than a link: the flag's live status, when the reader has set up an integration (next section). Without one it is an ordinary card.

## Integrations

An integration lets a card show a live fact about the thing it links to. There is one, LaunchDarkly, which shows what a feature flag is doing. It is off until the reader sets it up, and nothing in a tour can turn it on or point it somewhere else.

**Experimental for now.** The whole of it sits behind a switch: `"experimental": { "integrations": true }` in the user config, or `DOCENT_EXPERIMENTAL=integrations` for one run. A workspace file can't flip it. While it is off, a flag card is an ordinary link card, no provider is built, and nothing reaches the network. `docent integrations` and `docent doctor` say which way it is set.

**Flag cards.** Tours of real code keep mentioning flags: "this path only runs when `new-checkout-flow` is on". A flag card names the flag, and Docent says what it is doing right now.

```json
{ "kind": "flag", "title": "New checkout flow", "flag": "new-checkout-flow" }
```

- "On" is not an answer by itself: a flag can be on and still serve the old path to nearly everyone. So the card's first line always says what the default is: "Production · On · 1 prerequisite, 3 named targets, 2 rules, then v1".
- **How it decides** is a list in the order LaunchDarkly evaluates: prerequisite flags, contexts named one by one, each rule with its conditions in words, then everyone else. Each line ends in what it serves: a variation, or a percentage rollout drawn as a bar.
- A variation is called by its name, and its value is shown as the result of the rule that serves it. A JSON value is laid out in full when the list is short, and is a one-line preview to click when it is long. Every variation side by side is a section that starts folded.
- A segment a rule names is described under that rule: how many contexts it lists, how many it excludes, and its own conditions.
- The card is about one environment, the one the reader's variables pick. The others are one line underneath: the environments the configuration names, then "+12 more (4 on)" to unfold. A flag with fourteen environments costs the same space as one with three.
- The list starts unfolded when three or fewer things are decided before the default, and folded otherwise, so a six-rule flag doesn't push the narrative off the screen.
- An off flag says who gets what, and shows its rules dimmed: "1 rule set up, not in effect".
- **Archived** and **Not found** are states of their own. Not found is drift of its own kind: the tour describes a flag that has since been removed.
- `project` on the card overrides the configured project, for a tour that crosses LaunchDarkly projects.
- `url` is optional. With none, the card links to the flag's page at the configured host.
- Whatever LaunchDarkly returns (a name, a description) is shown as plain text, never rendered as Markdown or HTML.

**What a tour may say.** Identifiers, and nothing else.

- `flag` and `project` must match `[A-Za-z0-9._-]+`. Docent builds the request from them in Go; a tour never supplies any part of a URL that a token is sent to.
- The host and the token never come from a tour or from `docent.json`. Both arrive with a cloned repo, and either one choosing the host would let a stranger's file collect the reader's token.
- The request goes to the configured host over `https`, and redirects are not followed.
- What is left for a tour to do is cause a lookup of a flag key in a project the reader's token can already read, and the answer goes only to the reader's screen. LaunchDarkly learns which keys were looked up.

**Configuration.** Under `integrations.launchdarkly`, in the same two files as everything else.

```json
{
  "integrations": {
    "launchdarkly": {
      "host": "https://app.launchdarkly.com",
      "project": "default",
      "environment": "{{env}}",
      "environments": { "prod": "production", "staging": "test" }
    }
  }
}
```

| Setting | May be set in | Purpose |
| --- | --- | --- |
| `host` | User config, or `DOCENT_LAUNCHDARKLY_HOST` | Where requests go. Defaults to `https://app.launchdarkly.com`; set it for the EU and federal instances. Ignored in `docent.json`, and `docent doctor` says so when it finds one there. |
| `project` | `docent.json`, user config | The LaunchDarkly project a flag is looked up in, unless the card names one. |
| `environment` | `docent.json`, user config | Which environment's status to show. It is a link template, so `{{env}}` makes the badge follow the variable picker. |
| `environments` | `docent.json`, user config | Maps a value of that template to a LaunchDarkly environment key, where the names differ. |

A team shares `project`, `environment` and `environments` through `docent.json`. They only choose which flag is read at the host the reader chose, so they are safe to take from a repo.

**The token.** A LaunchDarkly access token, which should have the read-only Reader role. Docent looks for it in two places, in order:

1. `DOCENT_LAUNCHDARKLY_TOKEN` in the environment. This is the override, and the answer on a machine with no credential store, such as a headless Linux box.
2. The operating system's credential store: the Keychain on macOS, the Secret Service on Linux, the Credential Manager on Windows. `docent integrations set launchdarkly` puts it there, reading it from a prompt or stdin and never from an argument, so it stays out of shell history. `docent integrations remove launchdarkly` deletes it.

- The token is never written to a config file, never logged, and never sent to the webview. The app only reads it; storing and removing it are CLI commands, and `app.Service` has no method that touches the store.
- A stored token is found however the app was started. An environment variable is only seen when the app was started from a shell that has it (`docent .`), not from Finder, the Dock or a `docent://` link.
- The store is an interface (`secrets.Store`) over `zalando/go-keyring`, which already speaks to all three platforms' stores, so Linux and Windows need a build, not a redesign. Only macOS is exercised today. On macOS it hands the token to `/usr/bin/security` on standard input, never as an argument.
- A stored token is readable by anything running as the reader, as an environment variable is. The gain is that it isn't sitting in a dotfile or a shell profile.

**When Docent asks.** Setting up the integration is the consent; there is no prompt per tour. With a token present, Docent reads a flag's status when a step with a flag card is shown, with a short timeout, and remembers the answer for a minute. `⌘R` asks again. One flag is two requests (the picked environment in full, then every environment in brief) plus one for each segment its rules name, six at most. A segment's key comes from LaunchDarkly's answer and is checked like a tour's before it goes into a URL. With no token, opening a tour reaches no network, exactly as before.

**When it can't.** A flag card never fails a step. With no token it is a plain card with a note naming what would turn the badge on. A refused token, a rate limit, a missing `project` or `environment`, or no network gives **Status unavailable** with the reason. The CLI never asks at all: `docent validate` checks the shape of `flag` and `project` and stays offline, so its answers don't depend on a token or on LaunchDarkly being up.

**More integrations later.** LaunchDarkly sits behind a small interface (`flags.Provider`: a project, a flag key and an environment in, a status out). A second flag service is a second implementation chosen by which block is configured. There is deliberately no generic "fetch this URL and read this field" integration: every integration is code in Docent that knows which host it talks to and what it sends.

## Architecture

One Go binary is both the CLI and the desktop app. It's built on [Wails v3](https://newreleases.io/project/github/wailsapp/wails/release/v3.0.0), which is now a stable 3.0.0 release and renders the UI in each platform's native webview. Go owns all filesystem, git and parsing work. Svelte owns rendering.

&#91;embedded content: architecture · frontend, bindings, six Go services, three externals\]

The bindings layer is the trust boundary. The CLI skips it and calls the same services directly, which keeps `docent validate` byte-for-byte consistent with what the app shows.

**Go packages** (interface-first, each with an in-memory fake for tests)

| Package | Interface | Responsibility |
| --- | --- | --- |
| `workspace` | `RepoResolver` | Load `docent.json` and user config, then resolve repo names to roots in the order above. |
| `library` | `Store` | The saved list of folders the app opens, in `~/.config/docent/library.json`. |
| `tour` | `TourStore` | Discover, parse, validate (JSON Schema via `santhosh-tekuri/jsonschema`) and normalize tours into the internal model. Watch files with `fsnotify`. |
| `gitsrc` | `FileSource` | Read file bytes at a ref or from the working tree. Keep one long-lived `git cat-file --batch` process per repo rather than using go-git, for fidelity with partial clones and worktrees. |
| `anchor` | `AnchorResolver` | Turn `line`, `selection` and `pattern` into concrete ranges plus a drift status. |
| `links` | `Expander` | Expand `{{var}}` templates and enforce the allowed schemes. |
| `flags` | `Provider` | Read a feature flag's status from a configured integration. The only package that makes an HTTP request. |
| `secrets` | `Store` | Get, set and delete an integration's token in the operating system's credential store. |
| `editor` | `Launcher` | Run the open-in-editor template with arguments as an argv list, never through a shell. |
| `app` | (Wails services) | A thin binding layer. It validates every argument coming from the frontend, even though the frontend validated it too. |
| `cli` | (none) | Headless commands; never starts the webview. |

**Frontend.** Svelte 5 + TypeScript + Vite. Libraries: Shiki for highlighting (VS Code grammars, lazy-loaded per language), Mermaid, markdown-it, DOMPurify, and elkjs for `docent-flow` layout. TypeScript types are generated from the Go model through Wails bindings, so Go stays the source of truth. The code pane is virtualized.

**Observability.** Logging, tracing and metrics go through primitives-go's `observability` package, with logs written to `~/Library/Logs/docent/`. `docent doctor` prints config sources, resolved repos, git version and webview version. Hot paths emit timings at debug level.

**Performance budgets** (Apple-silicon laptop; each is a Go benchmark or a Playwright check in CI)

- Under 1 s from launch to the first step rendered, for a 30-step tour across 5 repos.
- Under 50 ms for step navigation when the file is cached; under 150 ms uncached.
- Smooth 60 fps scrolling for a 20,000-line file.
- Under 2 s for `docent validate` on a 50-step tour.

## CLI and agent authoring

Agents author through the CLI and files; people review in the app. The app never writes config (only its own list of folders), and its one edit to a tour is the review a reader signs with **Mark reviewed**, so a tour otherwise changes only through an editor, an agent, or an explicit CLI command.

| Command | Does |
| --- | --- |
| `docent [path]` | Open the app rooted at `path`. |
| `docent open <tour> [--step <id>] [--var k=v]` | Open a specific tour and step. The `docent://open?tour=…&step=…` URL scheme does the same thing. |
| `docent validate [paths…] [--json] [--strict]` | Check the schema, that repos resolve, that files exist at the ref, that lines are in range, that each `pattern` matches exactly once, that `mapFocus` ids exist in the map, and that visual bodies validate. Exit code 0 means clean, 1 means errors, 2 means warnings under `--strict`. |
| `docent resolve <tour> --step <id>` | Print the resolved code for a step, with line numbers and drift status. This is how an agent checks that an anchor points where it thinks. |
| `docent new <slug>` | Scaffold `<slug>.tour`, a `<slug>/` folder and a map stub. |
| `docent fmt` | Canonicalize formatting. Changes no values. |
| `docent stamp <tour> [--hashes]` | Move each repo's `stamp` to the commit its `ref` resolves to now. `--hashes` also records `contentHash` on each anchor at that commit (see drift). Moving a stamp without `--hashes` clears the old hashes, because a hash describes the code at its stamp. |
| `docent review <tour> --name <n>` | Append a `reviewedBy` entry at the current refs, and refresh stamps and hashes as `docent stamp --hashes` does. |
| `docent schema` / `docent guide` | Print the JSON Schema or the agent authoring guide. |
| `docent doctor` | Diagnose config, repos, git, webview and integrations. For a token it says where it was found (environment, credential store) or that it is not set, and never prints it. |
| `docent integrations [set\|remove] <name>` | List the integrations and whether each has a token; store one in the credential store, read from a prompt or stdin; or remove it. |
| docent stale \[--json\] \[--max-commits N\] \[--max-days N\] | List tours that have fallen behind their tracked refs or have drifted anchors, with the stamp-to-tip diff for each anchored file. This is the input for an agent refresh. |

**`--json` diagnostics** are a stable contract: `{file, pointer, step, severity, code, message, hint}`, where `pointer` is an RFC 6901 JSON Pointer such as `/steps/3/pattern`. Error codes are documented and never reused.

**Authoring guide.** `docent guide` prints a Markdown guide designed to be pasted into an agent's context. The repo also ships it as `SKILL.md` and an `AGENTS.md` snippet. It covers:

- Explore every repo before writing; choose a reader and a depth (5–8 steps for a quick look, 9–15 for an end-to-end feature).
- Tell a path, not an inventory: entry point, then hops across systems, then an edge case, then where to go next.
- Write the system map first, naming every edge a step will focus (`api e1@--> queue`); give every step a `mapFocus`.
- Anchor with `pattern` plus `line`; never guess a line number; run `docent resolve` on every step.
- Add a link card wherever a metric, log or alert is mentioned, using `vars` instead of hard-coded environments.
- Add a `flag` card wherever the code checks a feature flag, with the flag's key exactly as the code spells it.
- Use a `docent-*` visual when order or timing matters (async hops, retries, state changes) and Mermaid for structure.
- Set `generatedBy`; never set `reviewedBy`.
- Loop on `docent validate --json` until clean.

**The review loop.** The agent writes the tour and Docent live-reloads it. The person walks it and fixes steps in their editor (or asks the agent), then clicks **Mark reviewed** in the app or runs `docent review`. The button does what the command does, but only becomes available once every step has stayed on screen in one run of the app: passing through a step doesn't count, an edit to the tour starts the read again, and nothing about the read is saved. It asks for the reviewer's name (offering git's `user.name`) and confirmation before it writes. The **Unreviewed draft** badge exists because an AI tour that reads well can still be wrong, and walking it critically is the point of the tool.

## Anchor resolution and drift

Every anchor resolves to a concrete range plus one of six statuses, and Docent shows the best guess with a warning rather than failing the step.

**Resolution rules**

- `pattern` (Go RE2 syntax) is matched per line. If `line` is also set, the match nearest to `line` wins. Otherwise the first match wins.
- `selection` with a `pattern` is treated as offsets relative to the matched line. Without one, it's absolute.
- `line` alone is absolute.
- `directory` steps show a tree and its README if one exists. `uri` steps show a link card plus the narrative.

| Status | Meaning | UI |
| --- | --- | --- |
| `ok` | It resolved where the author said | No badge |
| `moved` | The pattern matched, but not at `line` | Amber badge showing the offset (for example, "+12 lines") |
| `ambiguous` | The pattern matched more than once and there was no `line` to break the tie | Amber badge; highlights the first match |
| `changed` | It resolved, but the anchored text's `contentHash` differs from the stamp | Amber badge: "Code changed since this step was written" |
| `missing` | File, pattern or ref not found | Red badge; shows the file at `line` if the file exists |
| `unverified` | The stamp is unreachable, so Docent fell back to `pattern` and can't confirm the code is unchanged | Amber badge: "Can't verify" |

`contentHash` is optional: SHA-256 of the anchored lines with whitespace normalized, written by `docent stamp --hashes`. Docent never rewrites anchors automatically. A later `docent fix` (post-v1) proposes updated `line` values for `moved` anchors.

**Following code forward from an old stamp.** When a tracked ref has moved past the stamp, Docent relocates each anchor in four stages and stops at the first that succeeds. It doesn't need a stored snippet, because git can always produce the file as it was at the stamp.

1. **Follow renames.** `git diff -M --name-status <stamp>..<tip> -- <file>` finds where a moved or renamed file went.
2. **Map lines through the diff.** Diff the file at the stamp against the tip and carry the anchored range through the hunks, the way forges keep review comments attached across pushes. If the range itself was untouched, the result is `moved` with an exact new location.
3. **Re-find edited code.** Try `pattern` in the new file, then fuzzy-match the original lines (similarity of at least 0.6 by default) against the new file. A match is `changed`, and the step offers a side-by-side **Then / Now** view.
4. **Explain removals.** If nothing matches, the step is `missing`. It still shows the code as it was at the stamp, plus the commit that removed it (`git log -S` / `-L`) with its subject line, for example "Removed in `d41f9a0`: replace polling worker with event stream". That's the answer to "what replaced this?": the step points at the change that replaced it.

If the stamp is unreachable (rebased away, or a shallow clone), stages 1–3 are skipped, Docent falls back to `pattern`, and the status is `unverified` (or `missing` if the pattern doesn't match either).

**Refreshing stale tours.** `docent stale [--json]` lists tours whose stamp is more than N commits or days behind (configurable), or that have any anchor that isn't `ok`. For each, it outputs the `stamp..tip` diff of every anchored file. That output is the brief for an agent to update the tour. The agent bumps `stamp`, clears `reviewedBy` and sets `generatedBy`, so the tour shows as **Unreviewed draft** again until a person walks it.

All paths are validated to stay inside their repo root after cleaning and symlink evaluation. A step whose `file` escapes its root is a validation error and is never read.

## Security, distribution and packaging

Tours are untrusted input from teammates, agents and strangers, so opening one must never run code, and a tour must never choose where a request goes or what it carries.

**Security rules**

- No execution from tours: CodeTour `commands` and `when` are ignored, there are no script embeds, and the editor template comes only from user config.
- Markdown is sanitized and Mermaid runs at `securityLevel: strict`.
- Links open only `http`/`https` in the system browser. A card always shows its resolved host before you click it.
- Network access is two things only. One is a `git fetch` or `git clone` that Docent offers and the reader confirms. The other is a read from an integration the reader set up, at the host the reader configured, carrying only identifiers Docent validated (see Integrations). With no integration set up, opening a tour reaches no network.
- A credential is read from the environment or the operating system's credential store, never from a tour, `docent.json` or `config.json`, and it never reaches the webview or a log. Only a CLI command stores one.
- File reads are confined to resolved repo roots (see drift).
- The webview runs a strict CSP: no remote scripts, no iframes. The [Wails 3.0.0 notes](https://newreleases.io/project/github/wailsapp/wails/release/v3.0.0) say one advisory about cross-origin child frames on Linux is only partly fixed, and Docent uses no iframes.

**Distribution**

- **macOS only in v1.** Ship a universal binary (arm64 and amd64) inside `Docent.app`, Developer ID signed and notarized. Distribute it as a Homebrew **cask** in your own tap: `brew install --cask <owner>/tap/docent`.
- **One binary, two faces.** The cask's `binary` stanza symlinks the app's executable onto the PATH as `docent`. With no arguments or a path, it opens the window. With a subcommand, it runs headless.
- **The official repo comes later.** A personal tap needs no approval, and the token is yours inside it. Getting into homebrew/cask needs public interest beyond the author: 30 forks, 30 watchers or 75 stars, rising to 90/90/225 for a self-submission ([Homebrew Package Acceptance Policy](https://docs.brew.sh/Package-Acceptance-Policy)). Tokens are first come, first served; fall back to `docent-app` if needed.
- **Updates** come through `brew upgrade`. There's no in-app updater.

**First-time Mac distribution playbook** (an Apple Developer Program membership is in place)

1. **Certificate.** In the Apple Developer portal, create a **Developer ID Application** certificate and install it in your login keychain. For CI, export it as a `.p12` and import it into a temporary keychain on each run.
2. **Notarization credentials.** Create an App Store Connect API key, or store an app-specific password with `xcrun notarytool store-credentials docent`.
3. **Sign.** Build `Docent.app` with Wails, then sign it with the hardened runtime and a secure timestamp: `codesign --options runtime --timestamp --sign "Developer ID Application: <Name> (<TEAMID>)"`. Sign any nested executables first, then the bundle; Apple advises against `--deep`. Add entitlements only if the webview needs them (follow Wails' macOS signing guide).
4. **Notarize and staple.** Zip with `ditto -c -k --keepParent Docent.app Docent.zip`, run `xcrun notarytool submit Docent.zip --keychain-profile docent --wait`, then `xcrun stapler staple Docent.app` and zip again. Confirm with `spctl -a -vvv Docent.app`.
5. **Release.** Upload the zip to a GitHub Release on the Docent repo.
6. **Tap.** Create a `<owner>/homebrew-tap` repo with `Casks/docent.rb`. It holds `version`, `sha256`, the release `url`, `app "Docent.app"`, `binary "#{appdir}/Docent.app/Contents/MacOS/docent"`, a `livecheck` block, and a `zap` stanza for `~/Library/Logs/docent` and `~/.config/docent`. The release workflow bumps `version` and `sha256` with each tag.
7. **Verify.** On a clean macOS user account, run `brew install --cask <owner>/tap/docent`. It should launch without a Gatekeeper warning, and `docent doctor` should work from the terminal. Run `brew audit --cask --new docent` early, so that a later move to homebrew/cask is painless.

Automate steps 3–6 in a GitHub Actions workflow on tag push, with the certificate and API key as repo secrets.

## Milestones and acceptance criteria

Seven milestones, each shippable and ending in something you can use daily. Build them in order; M3 and M4 can run in parallel. M0 to M5 are v1; M6 follows it.

1. **M0: Skeleton.** The Wails app and CLI in one binary; the tour parser with unknown-field preservation; `docent validate`; single-repo walking; the code pane with Shiki; the step rail; keyboard navigation.
   - Done when: a corpus of at least 20 real-world `.tour` files from public repos opens and walks with zero parse errors.
   - Done when: the superset holds (the corpus fits one model and no Docent field collides with a CodeTour field), or the switch to Docent's own format is decided.
2. **M1: Multi-repo.** `repos`, `docent.json`, the five-step resolution order, ref reads through `git cat-file --batch`, drift statuses, stamp-based anchor relocation (renames, diff line-mapping, fuzzy re-find, removal commits), docent stale, open-in-editor with detection and fallback, permalinks.
   - Done when: a 3-repo fixture tour resolves in both sibling-folder and mapped layouts, all six drift statuses have tests, and the perf budgets pass.
3. **M2: Rich narrative.** `body` files, Markdown with sanitization, Mermaid, link cards with `vars` and the picker, the system map with `mapFocus` animation, live reload that keeps position.
   - Done when: editing a body file updates the open app in under 500 ms without losing the current step.
4. **M3: Visuals.** `docent-sequence`, `docent-flow`, `docent-state` and `docent-timeline`, with playback controls, reduced motion, and schema validation in `docent validate`.
   - Done when: each visual has a fixture tour and a visual regression snapshot.
5. **M4: Agent authoring.** `--json` diagnostics, `resolve`, `new`, `fmt`, `stamp`, `review`, `guide`, the shipped `SKILL.md`, provenance badges.
   - Done when: an agent given only `docent guide` and three repos produces a tour that passes `validate --strict` with no human edits.
6. **M5: Ship.** Signing, notarization, the tap and cask, `docent doctor`.
   - Done when: `brew install --cask` on a clean Mac yields a working app and `docent` on the PATH.
7. **M6: Integrations.** `flag` cards, the LaunchDarkly provider, `secrets.Store` on the Keychain, `docent integrations`, and the token and host in `docent doctor`, all behind the `integrations` experiment.
   - Done when: it has been walked against a real LaunchDarkly project and the experiment switch can be removed.
   - Done when: against a fake LaunchDarkly server, a fixture tour's flag cards show on, off, archived, not found and unavailable, and a plain card with no token.
   - Done when: tests prove that neither a tour nor `docent.json` can change the host a token is sent to, and that the token never appears in a log or in anything returned to the frontend.

**After v1** (in rough priority order): symbol anchors via tree-sitter (`symbol: "Handler.UpdateItem"`), which hold up better than regex; diff steps (`diff: {base, head}`) for PR tours; a `docent mcp` server exposing validate and resolve; `docent fix`; sandboxed custom visual components; GitLab and Forgejo permalinks; Linux; Windows.

## Open questions

Decided on Nov 16, 2026:

| Question | Decision |
| --- | --- |
| Name | Docent. `brew search docent` returns no formula or cask. TransluceAI/docent (about 150 GitHub stars, unrelated) could claim the official token first; the fallback is `docent-app`. |
| VS Code CodeTour compatibility | Not needed: no teammates use it. The format is a superset of CodeTour's with flat extension fields, or Docent's own format if M0 shows a superset can't hold. |
| Where cross-repo tours live | A dedicated tours repo, listed in `tourPaths`. |
| Forges | GitHub only for permalinks. |
| Platforms | macOS only for v1. |
| Does the app write files? | Not tours or config: those change through the CLI, an editor or an agent. It offers `git clone` for missing repos and `git fetch` for stale refs, and runs either only when the reader confirms. |
| Map edge ids | Mermaid's own edge-id syntax (`api e1@--> queue`); `mapFocus` names `e1`. |
| `stamp` and `contentHash` | Separate operations: `docent stamp` moves repo stamps, `--hashes` also records anchor hashes, and `docent fmt` only formats. |
| Observability | primitives-go's `observability` package. |
| Signing | Apple Developer Program membership is in place; first Homebrew distribution follows the playbook above. |
| GitHub host | github.com only. Permalinks use https://github.com/{owner}/{repo}/blob/{sha}/{path}#L{start}-L{end}, built from the resolved commit SHA rather than the branch name, so links don't drift. GitHub Enterprise Server hosts are post-v1. |

Decided on Nov 25, 2026:

| Question | Decision |
| --- | --- |
| May Docent reach the network for anything but git? | Yes, for an integration the reader set up. The rule that matters was never "no network" but "a tour never chooses where a request goes": the host and token come only from the reader, and a tour supplies validated identifiers. |
| Where does an integration's token live? | The environment variable first, then the operating system's credential store (the Keychain on macOS), behind `secrets.Store` so Linux and Windows can be added. Never a config file. |
| Who stores the token? | The CLI (`docent integrations set`). The app only reads it, so "the app never writes config" stands unchanged. |
| Is a lookup confirmed each time? | No. Setting up the integration is the consent; a flag is read when its step is shown. |
| How general are integrations? | One provider interface with LaunchDarkly as the only implementation. No generic fetch-a-URL integration. |
| What does a flag card show? | How the flag decides in one environment, in evaluation order, with what each rule serves. Chosen over a status badge (too little: "on" alone misleads) and a grid of every environment (no room for JSON values, segments or rules, and no good with many environments). |
| How does it ship? | Behind the `integrations` experiment, off by default, so it can be tried on real tours before it is promised. |

## Sources

- [CodeTour schema (mirror)](https://git.cynarski.dev/Awesome/awesome-copilot/raw/branch/main/skills/code-tour/references/codetour-schema.json)
- [Wails v3.0.0 release notes](https://newreleases.io/project/github/wailsapp/wails/release/v3.0.0)
- [Homebrew Package Acceptance Policy](https://docs.brew.sh/Package-Acceptance-Policy)
