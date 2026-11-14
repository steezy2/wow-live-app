#!/usr/bin/env bash
# Builds WoW Live (bin/WoW Live.exe): the page, the icon (cmd/appicon), then the app with the page, the M+ and raid
# rules, the season's dungeon facts, the raid's boss mechanics and the icon built in. Run from anywhere, in Git Bash
# on Windows.
set -euo pipefail
repo="$(cd "$(dirname "$0")/../../.." && pwd)"
(cd "$repo/frontend" && npm ci && npm run build)
embed="$repo/backend/cmd/app/embed"
rm -rf "$embed/dist"
cp -r "$repo/frontend/dist" "$embed/dist"
cp "$repo/mplus.toml" "$repo/raid-rules.toml" "$repo/spec-rules.toml" "$repo/reference/mplus-dungeons.json" \
  "$repo/reference/raid-bosses.json" "$embed/"
cd "$repo/backend" && go run ./cmd/appicon
go build -ldflags "-H windowsgui" -o "$repo/bin/WoW Live.exe" ./cmd/app
echo "built $repo/bin/WoW Live.exe"
