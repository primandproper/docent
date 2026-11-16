#!/usr/bin/env bash
set -euo pipefail

# Run the browser tests: the production frontend bundle in WebKit, against
# answers recorded from the real Go service.
# Usage: test_e2e.sh [playwright args...]

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

go run ./cmd/tools/fixture frontend/e2e/testdata/workspace frontend/e2e/public/fixture.json

cd frontend
pnpm exec vite build --mode mock --logLevel warn
pnpm exec playwright test "$@"
