# CLAUDE.md

Docent is a macOS desktop app and CLI, in one Go binary, for walking guided tours of code. `docs/design.md` is the
product design and the source of truth for what gets built; milestones M0 to M6 are defined at its end.

## Layout

- `main.go`: embeds `frontend/dist` and calls `cli.Execute`. It has to live at the root: Wails' Taskfiles build the
  root package, and `go:embed` can't reach up a directory.
- `internal/cli`: cobra commands. No arguments or a folder opens the app; a subcommand runs headless.
- `internal/app`: the Wails service the frontend calls. It is the trust boundary: validate every argument here.
- `internal/tour`: discovery, parsing, the model, and `Watch` (live reload). `Parse` keeps the document as an ordered tree (`Tour.Raw`) so
  unknown fields and key order survive.
- `internal/orderedjson`: the ordered JSON tree behind that.
- `internal/gitsrc`: `FileSource` (reads confined to a root; `Confine` is the path-escape check) and `Git`, the
  only place git is run. Reads at a ref go through one long-lived `git cat-file --batch` per repo. Answers fixed by
  full commit SHAs (diffs, renames) are memoized for the session. `gittest` builds real repos for tests.
- `internal/anchor`: `Locate` turns `line`/`pattern`/`selection` into a range and a status; `relocate.go` follows
  an anchor forward from a stamp (rename, diff line-mapping, re-find, removal commit).
- `internal/walk`: resolves a step against the code: which repo, which ref (`refs.go`), where in the file, plus
  the system map and each repo's state. The app and every CLI command go through it; keep it that way so they
  can't disagree.
- `internal/mermaid`: a tolerant flowchart reader, only to check `mapFocus` ids.
- `internal/visual`: validates animated visuals and scans Markdown for `docent-*` fences.
- `internal/stamp`: `Apply` moves a tour's stamps and records a review. `docent stamp`, `docent review` and the
  app's Mark reviewed button all go through it.
- `internal/validate`: turns resolved steps into diagnostics.
- `internal/workspace`: `docent.json` and the user config, and `RepoResolver`, which maps a tour's repos to local
  checkouts (mapping, then sibling folder, then origin URL).
- `internal/library`: `library.json`, the folders the reader added in the app. The only config-like file the app
  writes. The CLI never reads it, except `docent doctor` to print it.
- `internal/links`: expands `{{var}}` link templates and refuses anything that isn't http(s). A `flag` card's keys
  are checked here.
- `internal/flags`: `Provider` reads a feature flag's status for a `flag` link card; `launchdarkly.go` is the one
  implementation, and the only place an HTTP request is made. `flagstest` is a stand-in LaunchDarkly server for
  tests and for the browser-test fixture.
- `internal/secrets`: `Store`, the operating system's credential store, where an integration's token lives.
- `internal/editor`: `Launcher`, open-in-editor with detection and fallback. Commands come from built-in presets or
  the user config only, never from a tour or a workspace file.
- `internal/core`: wires the services for one opened folder. Every CLI command builds a `core.Core`; the app builds
  one per open folder (`internal/app/folders.go`), so each folder keeps its own `docent.json`, vars and repos.
- `internal/guide`: `guide.md`, the authoring guide `docent guide` prints. It documents only what is built: update
  it in the same change as any tour-format or CLI change.
- `internal/diag`: the diagnostic type and codes. The JSON shape and the codes are a stable contract: add, never
  rename or reuse.
- `internal/logs`: the file logger (`~/Library/Logs/docent/docent.log`). Stdout belongs to command output.
- `internal/perf`: the design's performance budgets as tests.
- `schema/`: `tour.v1.json` and `visual.v1.json`, embedded by the `schema` package.
- `cmd/tools/fixture`: builds a workspace of real git repos from `frontend/e2e/testdata`, with drift, and records
  the real service's answers for the browser tests.
- `cmd/tools/skill`: writes `skill/` from the guide (`make skill`).
- `scripts/release.sh` (`make release VERSION=vX.Y.Z`): the release, run from a Mac. It chains `package.sh`,
  `sign.sh`, `notarize.sh` and `cask.sh`. There is deliberately no release workflow: signing in CI means shipping
  the certificate to GitHub as a secret, and signing locally uses the keychain.
- `packaging/icon/`: the icon's SVG sources. `make icons` regenerates `build/appicon.png` and `icons.icns` from
  them. `Assets.car` is compiled on every build and isn't checked in.
- `frontend/`: Svelte 5 + TypeScript + Vite. `src/lib/api.ts` is the only door to Go; `src/lib/session.svelte.ts`
  holds all app state. `bindings/` is generated and committed.
- `build/`: Wails build assets (macOS only).

## Commands

```sh
make setup              # go modules, pnpm install, Playwright's WebKit
make format lint test   # the baseline; run before pushing
make bindings           # regenerate frontend/bindings after changing internal/app's API or any type it returns
make build              # bin/docent
make dev                # hot reload
make test_e2e ARGS=--ui # browser tests only
```

`go vet ./...` before `make format`: the field-alignment step retries until it succeeds, so a compile error makes
`make format` spin forever instead of failing.

## Conventions

- Every exported method of `app.Service` is callable from the webview. Anything the frontend must not be able to
  do (open a folder without a dialog, queue a tour) stays unexported. `AddFolder` takes no path for that reason;
  `KeepFolder` and `RemoveFolder` take a root but only act on a folder that is already open.
- The app never writes config, and makes one edit to a tour: `Service.Review` appends a `reviewedBy` entry and
  stamps it. Its other writes are a `git fetch` or `git clone`, and `library.json`, its own list of folders. All
  happen only after the reader confirmed in the UI; a remote from a tour is checked by `cloneable` before git
  sees it.
- A tour belongs to one folder, and every call about it goes to that folder's services (`Service.svc(path)`).
  Links between tours prefer the linking tour's folder, in the validator (`Discoverer` tiers) and in the frontend
  (`nearest` in `src/lib/folders.ts`): keep the two agreeing.
- A review is only accepted once every step was read in this run of the app. The frontend reports a step read after
  it has stayed on screen (`READ_AFTER_MS`), but `Service` keeps the count and is what refuses: it resets when the
  tour file changes or the working-tree toggle flips, and it is never saved.
- A step's code is read at its ref's commit, never from the checkout, unless the working-tree toggle is on.
- The system map can be popped out into a second window (`/#map`, `MapWindow.svelte`). That window holds no state:
  the main window tells it what to draw and it sends keys and clicks back (`MapMessage` in `api.ts`), so the tour is
  only ever in one `Session`. `MapDiagram.svelte` is the map wherever it is shown.
- The call transition (`"transition": "call"`, `src/lib/dive.ts`) is opt-in per step and only plays when
  `findCall` finds the call in the highlight and it is on screen. Its styles are in `app.css` because its elements
  are made outside any component.
- Mermaid and ELK are loaded on first use. Arrowheads in the visuals use explicit markers, not `context-stroke`,
  which WebKit doesn't support.
- Visual snapshots live in `frontend/e2e/visuals.spec.ts-snapshots`. Playwright only rewrites a snapshot that
  differs beyond tolerance, so delete the file before `--update-snapshots` when a small change must be captured.

- Extension fields are only interpreted when a tour sets `"docent": 1`. A plain CodeTour file is never reinterpreted.
- A broken anchor is an answer, not an error: resolution returns a status and a `diag.Code`, and the UI shows its
  best guess. Go errors are for failures that aren't about the tour.
- Tours are untrusted input. Nothing from a tour is executed, fetched or allowed to read outside its root, and the
  production bundle ships a strict CSP (`frontend/vite.config.ts`).
- An integration is the one thing that reaches the network besides git, and a tour never chooses where it goes. The
  host and the token come only from the user config, the environment or the credential store: never from a tour or
  `docent.json`. A tour supplies keys, and `flags.ValidKey` checks every key before it goes into a URL. The token is
  never logged, written to a file, or returned to the frontend. Only the CLI stores it (`docent integrations set`).
- Integrations are experimental: `core.Core.Flags` is nil unless the user config or `DOCENT_EXPERIMENTAL` switches
  them on. While it is nil a `flag` card is a plain link card (`TourView.FlagStatus` tells the frontend which).
- Tests never touch the real Keychain or the real LaunchDarkly: pass `secrets.Memory`, `Getenv` and `flags.Memory`
  (or a `flagstest` server) through `core.Options` or `cli.Options`.
- Each package's concrete type takes a logger and tracer provider and names itself with an `o11yName` const. Nil is
  fine for both (tests pass nil).
- Go tests: `shoenig/test`, subtests always, `T`/`t` naming, parallel everywhere. In-memory fakes:
  `gitsrc.Memory`, `tour.MemoryStore`.
- `internal/tour/testdata/corpus` is real-world tours, byte for byte, with `manifest.json` recording their source
  and `licenses/` their license texts. Don't edit them; `TestCorpus` is the proof that the format is a superset.
- CLI tests pass `UserConfigPath` and `LogDir` so they never touch the real `~/.config/docent` or log folder. Keep
  new tests hermetic the same way.
- Browser tests run the production bundle in WebKit with `--mode mock`, which swaps `src/lib/backend.ts` for
  `backend.mock.ts`. The fixture folders are `frontend/e2e/testdata/workspace` and `handbook`; the mock starts with
  only the first open, and its Add a folder "picks" the next.
