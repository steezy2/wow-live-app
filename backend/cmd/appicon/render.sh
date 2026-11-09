#!/usr/bin/env bash
# Renders icon.svg (48 px and up) and icon-small.svg (under 48) to png/icon-SIZE.png with Edge's headless mode, on a
# see-through background, and the MSIX package's logos to png/msix/. Run it from backend/cmd/appicon after changing
# either SVG, and commit the PNGs: the app build (cmd/appicon) and the package (cmd/app/msix) use them and don't need
# a browser.
set -euo pipefail
edge="/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
tmp=$(mktemp -d)
render() { # render SIZE OUT
  local src=icon.svg
  [ "$1" -lt 48 ] && src=icon-small.svg
  sed -E "0,/width=\"[0-9]+\" height=\"[0-9]+\"/s//width=\"$1\" height=\"$1\"/" "$src" > "$tmp/$1.svg"
  "$edge" --headless=new --disable-gpu --hide-scrollbars --default-background-color=00000000 --window-size="$1,$1" \
    --screenshot="$(cygpath -w "$PWD/$2")" "file:///$(cygpath -m "$tmp/$1.svg")" >/dev/null 2>&1
}
for s in 16 24 32 48 64 128 256; do
  render "$s" "png/icon-$s.png"
done
mkdir -p png/msix
render 44 png/msix/Square44x44Logo.png
render 50 png/msix/StoreLogo.png
render 150 png/msix/Square150x150Logo.png
rm -r "$tmp"
