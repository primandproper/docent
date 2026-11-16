#!/usr/bin/env bash
set -euo pipefail

# Render the Homebrew cask for a release.
# Usage: cask.sh <version> <zip>
#
# Prints Casks/docent.rb for the tap, with the version and the zip's checksum
# filled in to packaging/docent.rb.tmpl.

VERSION="${1:?usage: cask.sh <version> <zip>}"
ZIP="${2:?usage: cask.sh <version> <zip>}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

SHA="$(shasum -a 256 "${ZIP}" | cut -d ' ' -f 1)"

sed -e "s/@VERSION@/${VERSION#v}/g" -e "s/@SHA256@/${SHA}/g" "${ROOT}/packaging/docent.rb.tmpl"
