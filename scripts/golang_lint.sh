#!/usr/bin/env bash
set -euo pipefail

# Lint Go code using golangci-lint
# Usage: golang_lint.sh
#
# Native rather than in a container like template-go: Wails is cgo against the
# host's webview (WebKit on macOS), which a Linux container can't type-check.

command -v golangci-lint >/dev/null || { echo "golangci-lint is not installed (brew install golangci-lint)" >&2; exit 1; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE="$(go list -m)"

# golangci-lint takes directories, not import paths.
directories=()
while IFS= read -r pkg; do
  directories+=("./${pkg#"${MODULE}"}")
done < <("${SCRIPT_DIR}/go_packages.sh")

golangci-lint run --config=.golangci.yml --timeout 30m "${directories[@]}"
