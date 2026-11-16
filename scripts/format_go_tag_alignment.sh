#!/usr/bin/env bash
set -euo pipefail

# Format Go tag alignment
# Usage: format_go_tag_alignment.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

packages=()
while IFS= read -r pkg; do
  packages+=("${pkg}")
done < <("${SCRIPT_DIR}/go_packages.sh")

until go tool tagalign -fix -sort -order "env,envDefault,envPrefix,json,mapstructure,toml,yaml" "${packages[@]}"; do
  true
done > /dev/null
