#!/usr/bin/env bash
set -euo pipefail

# Release Docent from this Mac: build, sign, notarize, publish, update the tap.
# Usage: release.sh <version>        e.g. release.sh v0.1.0
#        release.sh <version> --dry-run   build and sign only; publish nothing
#
# Nothing here needs a file kept around or a secret in GitHub:
#   - signing uses the Developer ID Application certificate in your keychain
#   - notarizing uses the keychain profile named by NOTARY_PROFILE (default
#     "docent"), made once with:
#       xcrun notarytool store-credentials docent --apple-id <you> --team-id <TEAMID>
#   - the GitHub release and the cask are pushed as whoever `gh` is logged in as

VERSION="${1:?usage: release.sh <version> [--dry-run]}"
DRY_RUN="${2:-}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SCRIPTS="${ROOT}/scripts"
TAP="primandproper/homebrew-tap"
export NOTARY_PROFILE="${NOTARY_PROFILE:-docent}"

cd "${ROOT}"

fail() {
  echo "release.sh: $*" >&2
  exit 1
}

[[ "${VERSION}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "${VERSION} is not a version like v0.1.0"

# Everything that could stop a release is checked before anything is built.
IDENTITY="$(security find-identity -v -p codesigning | sed -n 's/.*"\(Developer ID Application: [^"]*\)".*/\1/p' | head -n 1)"
[ -n "${IDENTITY}" ] || fail "no Developer ID Application certificate in the keychain"

if [ "${DRY_RUN}" != "--dry-run" ]; then
  [ -z "$(git status --porcelain)" ] || fail "the working tree has uncommitted changes"
  [ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || fail "releases are cut from main"

  git fetch --quiet origin main --tags
  [ "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)" ] || fail "main is not the same as origin/main; push or pull first"
  git rev-parse --quiet --verify "refs/tags/${VERSION}" >/dev/null && fail "the tag ${VERSION} already exists"

  gh auth status >/dev/null 2>&1 || fail "gh is not logged in"
  xcrun notarytool history --keychain-profile "${NOTARY_PROFILE}" >/dev/null 2>&1 ||
    fail "no notarization credentials under the keychain profile \"${NOTARY_PROFILE}\"; see the top of this script"
fi

"${SCRIPTS}/package.sh" "${VERSION}"
"${SCRIPTS}/sign.sh" "${IDENTITY}"

if [ "${DRY_RUN}" = "--dry-run" ]; then
  echo "Dry run: built and signed bin/release/Docent.app as ${IDENTITY}. Nothing was notarized or published."
  exit 0
fi

"${SCRIPTS}/notarize.sh" "${VERSION}"
ZIP="${ROOT}/bin/release/Docent-${VERSION}.zip"

# From here on things leave this machine, so ask once.
read -r -p "Publish ${VERSION} to GitHub and Homebrew? [y/N] " answer
[[ "${answer}" =~ ^[Yy]$ ]] || fail "stopped before publishing; the notarized zip is at ${ZIP}"

git tag "${VERSION}"
git push origin "${VERSION}"
gh release create "${VERSION}" "${ZIP}" --generate-notes --verify-tag

WORK="$(mktemp -d)"
trap 'rm -rf -- "${WORK}"' EXIT

gh repo clone "${TAP}" "${WORK}/tap" -- --quiet
mkdir -p "${WORK}/tap/Casks"
"${SCRIPTS}/cask.sh" "${VERSION}" "${ZIP}" > "${WORK}/tap/Casks/docent.rb"

git -C "${WORK}/tap" add Casks/docent.rb
git -C "${WORK}/tap" commit --quiet -m "chore: docent ${VERSION}"
git -C "${WORK}/tap" push --quiet

echo "Released ${VERSION}. Install with: brew install --cask primandproper/tap/docent"
