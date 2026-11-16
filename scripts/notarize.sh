#!/usr/bin/env bash
set -euo pipefail

# Notarize and staple Docent.app, then zip it for release.
# Usage: notarize.sh <version>
#
# Credentials come from the environment, as an App Store Connect API key:
#   NOTARY_KEY_PATH   path to the .p8 key file
#   NOTARY_KEY_ID     the key's id
#   NOTARY_ISSUER_ID  the issuer id
# or, for a local run, NOTARY_PROFILE: a keychain profile made with
# `xcrun notarytool store-credentials`.
#
# Prints the path of the zip to upload.

VERSION="${1:?usage: notarize.sh <version>}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/bin/release"
APP="${OUT}/Docent.app"
SUBMISSION="${OUT}/Docent-submission.zip"
ZIP="${OUT}/Docent-${VERSION}.zip"

[ -d "${APP}" ] || { echo "notarize.sh: ${APP} doesn't exist; run package.sh and sign.sh first" >&2; exit 1; }

credentials=()
if [ -n "${NOTARY_PROFILE:-}" ]; then
  credentials=(--keychain-profile "${NOTARY_PROFILE}")
else
  credentials=(--key "${NOTARY_KEY_PATH:?set NOTARY_KEY_PATH or NOTARY_PROFILE}" --key-id "${NOTARY_KEY_ID:?}" --issuer "${NOTARY_ISSUER_ID:?}")
fi

ditto -c -k --keepParent "${APP}" "${SUBMISSION}"
xcrun notarytool submit "${SUBMISSION}" "${credentials[@]}" --wait
rm -- "${SUBMISSION}"

# The ticket is stapled to the app, so the zip that ships is made afterwards.
xcrun stapler staple "${APP}"
spctl --assess --type execute --verbose=2 "${APP}"

ditto -c -k --keepParent "${APP}" "${ZIP}"
echo "${ZIP}"
