# Docent

A native macOS app for walking guided tours of code. Tours are
[CodeTour](https://github.com/microsoft/codetour)-compatible `.tour` files; Docent reads them unchanged and adds
optional extension fields on top. The full design is in [docs/design.md](docs/design.md).

```sh
docent .                 # open the tours under a folder
docent validate --json   # check tours against the schema and the code they point at
```

## Status

Built so far (milestone M0 of the design): the app and CLI in one binary, the tour parser, `docent validate`,
single-repo walking with a syntax-highlighted code pane, the step rail and keyboard navigation.

## Development

Needs Go, pnpm, golangci-lint and Docker (for shellcheck).

```sh
make setup    # dependencies, plus the WebKit build the browser tests use
make dev      # run the app with hot reload
make build    # build bin/docent
make format lint test
```

This repo has a tour of its own: `make build && bin/docent .`
