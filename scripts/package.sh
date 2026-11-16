#!/usr/bin/env bash
set -euo pipefail

# Build Docent.app as a universal (arm64 + amd64) bundle, unsigned.
# Usage: package.sh <version>
#
# The result is bin/release/Docent.app. Signing and notarizing are separate
# (sign.sh, notarize.sh) so this much can be built and tried on any Mac.

VERSION="${1:?usage: package.sh <version>}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/bin/release"
APP="${OUT}/Docent.app"
MODULE="github.com/primandproper/docent"
# The oldest macOS the app supports. Keep in step with LSMinimumSystemVersion in build/darwin/Info.plist.
MIN_MACOS="12.0"

cd "${ROOT}"

(cd frontend && pnpm install --frozen-lockfile && pnpm run build)

rm -rf -- "${OUT}"
mkdir -p "${APP}/Contents/MacOS" "${APP}/Contents/Resources"

for arch in arm64 amd64; do
  GOOS=darwin GOARCH="${arch}" CGO_ENABLED=1 \
    CGO_CFLAGS="-mmacosx-version-min=${MIN_MACOS}" \
    CGO_LDFLAGS="-mmacosx-version-min=${MIN_MACOS}" \
    MACOSX_DEPLOYMENT_TARGET="${MIN_MACOS}" \
    go build -tags production -trimpath -buildvcs=false \
    -ldflags "-w -s -X ${MODULE}/internal/version.Version=${VERSION}" \
    -o "${OUT}/docent-${arch}" .
done

lipo -create -output "${APP}/Contents/MacOS/docent" "${OUT}/docent-arm64" "${OUT}/docent-amd64"
rm -- "${OUT}/docent-arm64" "${OUT}/docent-amd64"

cp build/darwin/icons.icns "${APP}/Contents/Resources/"
if [ -f build/darwin/Assets.car ]; then
  cp build/darwin/Assets.car "${APP}/Contents/Resources/"
fi

# The bundle's version is the release's, without the leading "v".
cp build/darwin/Info.plist "${APP}/Contents/Info.plist"
plutil -replace CFBundleVersion -string "${VERSION#v}" "${APP}/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "${VERSION#v}" "${APP}/Contents/Info.plist"

# Finder metadata would invalidate a signature made later.
xattr -cr "${APP}"

echo "${APP}"
