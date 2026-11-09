#!/usr/bin/env bash
# Renders icon.svg (48 px and up) and icon-small.svg (under 48) to png/icon-SIZE.png with Edge's headless mode, on a
# see-through background. Run it from backend/cmd/appicon after changing either SVG, and commit the PNGs: the app
# build (cmd/appicon) packs them and doesn't need a browser.
set -euo pipefail
edge="/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
tmp=$(mktemp -d)
for s in 16 24 32 48 64 128 256; do
  src=icon.svg
  [ "$s" -lt 48 ] && src=icon-small.svg
  sed -E "0,/width=\"[0-9]+\" height=\"[0-9]+\"/s//width=\"$s\" height=\"$s\"/" "$src" > "$tmp/$s.svg"
  "$edge" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 --window-size="$s,$s" \
    --screenshot="$(cygpath -w "$PWD/png/icon-$s.png")" "file:///$(cygpath -m "$tmp/$s.svg")" >/dev/null 2>&1
done
rm -r "$tmp"
