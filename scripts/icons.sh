#!/usr/bin/env bash
set -euo pipefail

# Rebuild the app icon from its sources in packaging/icon.
# Usage: icons.sh
#
# Two SVGs are the source of truth:
#   mark.svg     the white mark alone. macOS 26 composes the icon from this
#                layer and the fill in build/appicon.icon/icon.json.
#   appicon.svg  the whole icon on its rounded square, for everything older.
#
# Needs rsvg-convert (brew install librsvg) and Xcode.

command -v rsvg-convert >/dev/null || { echo "icons.sh: rsvg-convert is not installed (brew install librsvg)" >&2; exit 1; }

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

rsvg-convert --width 1024 --height 1024 packaging/icon/appicon.svg --output build/appicon.png
cp packaging/icon/mark.svg build/appicon.icon/Assets/docent_mark.svg

# Wails turns the PNG into icons.icns and the .icon bundle into Assets.car.
scripts/wails.sh task common:generate:icons
