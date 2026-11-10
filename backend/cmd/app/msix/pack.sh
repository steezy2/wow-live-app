#!/usr/bin/env bash
# Packs bin/WoW Live.exe (from build.sh) into bin/WoW Live.msix for the Microsoft Store, which signs it: the package
# can't be installed before that. The identity is the one Partner Center reserved for the app (Product identity), in
# MSIX_NAME, MSIX_PUBLISHER and MSIX_PUBLISHER_NAME.
#   pack.sh 1.2.3   (Git Bash on Windows, with the Windows SDK)
set -euo pipefail
: "${MSIX_NAME:?}" "${MSIX_PUBLISHER:?}" "${MSIX_PUBLISHER_NAME:?}"
here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/../../../.." && pwd)"
makeappx="$(ls -d "/c/Program Files (x86)/Windows Kits/10/bin/10."*/x64/makeappx.exe | sort -V | tail -1)"
stage=$(mktemp -d)
cp "$repo/bin/WoW Live.exe" "$repo/backend/cmd/appicon/png/msix/"*.png "$stage/"
# The Store wants the version's fourth part 0.
sed -e "s|\$VERSION|$1.0|" -e "s|\$NAME|$MSIX_NAME|" -e "s|\$PUBLISHER_NAME|$MSIX_PUBLISHER_NAME|" \
  -e "s|\$PUBLISHER|$MSIX_PUBLISHER|" "$here/AppxManifest.xml" > "$stage/AppxManifest.xml"
MSYS2_ARG_CONV_EXCL='*' "$makeappx" pack /o /d "$(cygpath -w "$stage")" /p "$(cygpath -w "$repo/bin/WoW Live.msix")"
rm -r "$stage"
