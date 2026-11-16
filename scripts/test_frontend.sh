#!/usr/bin/env bash
set -euo pipefail

# Run the frontend unit tests.
# Usage: test_frontend.sh

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/frontend"

pnpm exec vitest run
