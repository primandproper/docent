#!/usr/bin/env bash
set -euo pipefail

# Run prettier over the frontend, writing by default or checking with --check.
# Usage: format_frontend.sh [--check]

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/frontend"

MODE="--write"
if [ "${1:-}" = "--check" ]; then
  MODE="--check"
fi

pnpm exec prettier "${MODE}" --log-level warn 'src/**/*.{ts,svelte,css}' 'e2e/*.ts' '*.{json,js,ts,html}'
