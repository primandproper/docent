# CLAUDE.md

Docent is a macOS desktop app and CLI, in one Go binary, for walking guided tours of code. `docs/design.md` is the
product design and the source of truth for what gets built; milestones M0 to M5 are defined at its end.

## Layout

- `main.go`: embeds `frontend/dist` and calls `cli.Execute`. It has to live at the root: Wails' Taskfiles build the
  root package, and `go:embed` can't reach up a directory.
- `internal/cli`: cobra commands. No arguments or a folder opens the app; a subcommand runs headless.
- `internal/app`: the Wails service the frontend calls. It is the trust boundary: validate every argument here.
- `internal/tour`: discovery, parsing, the model, and `Watch` (live reload). `Parse` keeps the document as an ordered tree (`Tour.Raw`) so
  unknown fields and key order survive.
- `internal/orderedjson`: the ordered JSON tree behind that.
- `internal/gitsrc`: `FileSource`, reading files confined to a root. `Confine` is the path-escape check.
- `internal/anchor`: `Locate` turns `line`/`pattern`/`selection` into a range and a status.
- `internal/walk`: resolves a step against the code. The app and `docent validate` both go through it; keep it
  that way so they can't disagree.
- `internal/validate`: turns resolved steps into diagnostics.
- `internal/workspace`: `docent.json` and the user config, and `RepoResolver`, which maps a tour's repos to local
  checkouts (mapping, then sibling folder, then origin URL).
- `internal/links`: expands `{{var}}` link templates and refuses anything that isn't http(s).
- `internal/editor`: `Launcher`, open-in-editor with detection and fallback. Commands come from built-in presets or
  the user config only, never from a tour or a workspace file.
- `internal/core`: wires the services for one opened folder. The app and every CLI command build a `core.Core`.
- `internal/guide`: `guide.md`, the authoring guide `docent guide` prints. It documents only what is built: update
  it in the same change as any tour-format or CLI change.
- `internal/diag`: the diagnostic type and codes. The JSON shape and the codes are a stable contract: add, never
  rename or reuse.
- `internal/logs`: the file logger (`~/Library/Logs/docent/docent.log`). Stdout belongs to command output.
- `schema/tour.v1.json`: the tour JSON Schema, embedded by the `schema` package.
- `cmd/tools/fixture`: records the real service's answers for a workspace, for the browser tests.
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

- Extension fields are only interpreted when a tour sets `"docent": 1`. A plain CodeTour file is never reinterpreted.
- A broken anchor is an answer, not an error: resolution returns a status and a `diag.Code`, and the UI shows its
  best guess. Go errors are for failures that aren't about the tour.
- Tours are untrusted input. Nothing from a tour is executed, fetched or allowed to read outside its root, and the
  production bundle ships a strict CSP (`frontend/vite.config.ts`).
- Each package's concrete type takes a logger and tracer provider and names itself with an `o11yName` const. Nil is
  fine for both (tests pass nil).
- Go tests: `shoenig/test`, subtests always, `T`/`t` naming, parallel everywhere. In-memory fakes:
  `gitsrc.Memory`, `tour.MemoryStore`.
- `internal/tour/testdata/corpus` is real-world tours, byte for byte, with `manifest.json` recording their source
  and `licenses/` their license texts. Don't edit them; `TestCorpus` is the proof that the format is a superset.
- CLI tests pass `UserConfigPath` and `LogDir` so they never touch the real `~/.config/docent` or log folder. Keep
  new tests hermetic the same way.
- Browser tests run the production bundle in WebKit with `--mode mock`, which swaps `src/lib/backend.ts` for
  `backend.mock.ts`. The fixture workspace is `frontend/e2e/testdata/workspace`.
