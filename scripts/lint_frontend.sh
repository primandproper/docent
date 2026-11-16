#!/usr/bin/env bash
set -euo pipefail

# Lint and type-check the frontend.
# Usage: lint_frontend.sh

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/frontend"

pnpm exec eslint --max-warnings 0 .
pnpm run check
"${ROOT}/scripts/format_frontend.sh" --check
