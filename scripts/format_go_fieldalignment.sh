#!/usr/bin/env bash
set -euo pipefail

# Format Go field alignment
# Usage: format_go_fieldalignment.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

packages=()
while IFS= read -r pkg; do
  packages+=("${pkg}")
done < <("${SCRIPT_DIR}/go_packages.sh")

until go tool fieldalignment -fix "${packages[@]}"; do
  true
done > /dev/null
