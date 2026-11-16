#!/usr/bin/env bash
set -euo pipefail

# Run the wails3 CLI at the version go.mod pins as a tool.
# Usage: wails.sh <wails3 args...>
#
# Not `go tool wails3`: Wails' Taskfiles under build/ call `wails3` by name, so
# it has to be on PATH. Installing it into .tools/ keeps the pinned version out
# of the global GOBIN.

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="${ROOT}/.tools"

cd "${ROOT}"
GOBIN="${TOOLS_DIR}" go install github.com/wailsapp/wails/v3/cmd/wails3
PATH="${TOOLS_DIR}:${PATH}" exec wails3 "$@"
