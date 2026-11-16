#!/usr/bin/env bash
set -euo pipefail

# Run tests
# Usage: test.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

packages=()
while IFS= read -r pkg; do
  packages+=("${pkg}")
done < <("${SCRIPT_DIR}/go_packages.sh")

CGO_ENABLED=1 go test -shuffle=on -race -vet=all -failfast "${packages[@]}"
