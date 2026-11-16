#!/usr/bin/env bash
set -euo pipefail

# Sign Docent.app with a Developer ID certificate.
# Usage: sign.sh <identity>
#   identity: e.g. "Developer ID Application: Jane Doe (TEAMID1234)"
#
# The hardened runtime and a secure timestamp are both required for
# notarization. Nested code is signed before the bundle; Apple advises against
# --deep. Docent has one executable, so there is nothing nested today.

IDENTITY="${1:?usage: sign.sh <identity>}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP="${ROOT}/bin/release/Docent.app"

[ -d "${APP}" ] || { echo "sign.sh: ${APP} doesn't exist; run package.sh first" >&2; exit 1; }

codesign --force --options runtime --timestamp --sign "${IDENTITY}" "${APP}/Contents/MacOS/docent"
codesign --force --options runtime --timestamp --sign "${IDENTITY}" "${APP}"
codesign --verify --strict --verbose=2 "${APP}"
