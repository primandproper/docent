# Docent

A native macOS app for walking guided tours of code. Tours are
[CodeTour](https://github.com/microsoft/codetour)-compatible `.tour` files; Docent reads them unchanged and adds
optional extension fields on top. The full design is in [docs/design.md](docs/design.md).

```sh
docent .                          # open the tours under a folder
docent new cache-invalidation     # scaffold a tour
docent resolve <tour> --step <id> # print the code a step's anchor lands on
docent validate --strict --json   # check tours against the schema and the code
docent guide                      # the authoring guide, written for an agent to follow
```

## Status

Built so far:

- Walking tours: tour picker, step rail, highlighted code pane, narrative with Markdown body files, link cards with
  variables, keyboard navigation, live reload, open in editor.
- Tours across repos: a tour names repos by git remote, and Docent finds each checkout through `docent.json`, a
  sibling folder, or its origin URL.
- Authoring: `new`, `resolve`, `validate`, `schema` and `guide`.

Not built yet: reading at a git ref, tracking drift from a stamped commit, the system map, Mermaid, animated visuals,
and the signed Homebrew release.

## Configuration

`docent.json` in the opened folder (or any ancestor), and `~/.config/docent/config.json`, both JSON with comments:

```jsonc
{
  "repoRoots": ["~/work"],                    // folders that hold checkouts side by side
  "repos": { "api": "~/work/api-service" },   // or say exactly where one is
  "tourPaths": ["~/work/team-tours"],         // extra places to look for tours
  "vars": { "grafana": "https://grafana.example.com" },
  "editors": {
    "default": "zed",                         // zed, cursor, code, idea, goland, subl, or a custom one
    "custom": { "nvim": { "argv": ["ghostty", "-e", "nvim", "+{line}", "{path}"] } }
  }
}
```

Custom editors are only read from the user config.

## Development

Needs Go, pnpm, golangci-lint and Docker (for shellcheck).

```sh
make setup    # dependencies, plus the WebKit build the browser tests use
make dev      # run the app with hot reload
make build    # build bin/docent
make format lint test
```

This repo has a tour of its own: `make build && bin/docent .`
