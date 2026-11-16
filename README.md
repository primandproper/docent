# Docent

A native macOS app for walking guided tours of code. Tours are
[CodeTour](https://github.com/microsoft/codetour)-compatible `.tour` files; Docent reads them unchanged and adds
optional extension fields on top. The full design is in [docs/design.md](docs/design.md).

```sh
docent .                            # open the tours under a folder
docent open <tour> --step <id>      # open one tour at a step
docent new cache-invalidation       # scaffold a tour, a map and a body folder
docent resolve <tour> --step <id>   # print the code a step's anchor lands on
docent validate --strict --json     # check tours against the schema and the code
docent stamp --hashes <tour>        # record the commits a tour is correct against
docent review <tour> --name <you>   # vouch for a tour you have walked
docent stale --json                 # tours the code has moved away from, with diffs
docent fmt                          # canonical formatting
docent guide                        # the authoring guide, written for an agent to follow
docent doctor                       # what Docent sees: config, repos, editors, git
```

## What it does

- **Walks tours across repos.** A tour names repos by git remote; Docent finds each checkout through `docent.json`,
  a sibling folder, or its origin URL, and offers the clone command for any that are missing.
- **Reads code at a ref, not your checkout.** A branch is tracked at its tip as of your last fetch; a tag or commit
  is pinned. A toggle reads the working tree instead.
- **Tracks drift.** Each repo is stamped with the commit the tour was written against. Anchors are followed forward
  through renames and diffs, and each step says whether its code moved, changed or was removed, with the original
  beside the current and the commit that removed it.
- **Shows the system.** One Mermaid map per tour, with the current step highlighted; Mermaid diagrams in step
  bodies; four animated visuals (sequence, flow, state, timeline) declared as data.
- **Links out.** Typed link cards with variables, permalinks at the exact commit, and open-in-editor with per-file
  rules.
- **Is safe to point at a stranger's tour.** Tours can't run code, reads are confined to each repo, and the only
  things Docent ever does to a repo are a `git fetch` or `git clone` you confirm.

Agents write tours with `docent guide`; `skill/` has the same guide as an agent skill and an `AGENTS.md` snippet.

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
    "rules": [                                // first match wins; workspace rules before your own
      { "match": "**/*.{java,kt}", "use": "idea" },
      { "repo": "infra", "use": "zed" }
    ],
    "custom": { "helix": { "argv": ["ghostty", "-e", "hx", "{path}:{line}:{col}"] } }
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
make skill    # regenerate skill/ after changing the guide
```

## Releasing

Pushing a `v*` tag runs `.github/workflows/release.yaml`: it builds a universal `Docent.app`, signs and notarizes
it, publishes the zip as a GitHub release, and updates the cask in `primandproper/homebrew-tap`. The secrets it needs
are listed at the top of the workflow. `scripts/package.sh <version>` builds the unsigned bundle locally.

This repo has a tour of its own: `make build && bin/docent .`
