# Tech debt and findings

The running list of what can be improved in the code base. Add an item when a review finds one that isn't fixed in
the same change; move it to **Done** (with the PR) when it lands. Size: S under 50 lines, M a day, L a design change.

## Open

| # | Item | Where | Why it matters | Size |
|---|------|-------|----------------|------|
| 1 | Every saved key and pull is kept fully in memory | `LogTracker.history` / `pulls` in `service/mplus.go` (about 40 call sites) | Memory grows with history; load on demand instead | L |
| 2 | The overlay window is always created, even hidden | `cmd/app/main.go` | A second WebView2 renderer costs tens of MB when the overlay is off | M |
| 3 | `Tabs` mounts every panel up front | `Tabs.tsx` | A key page fetches routes, comparison, dungeon map and mobs even if the Route tab never opens. Lazy mount conflicts with the "as tall as the tallest" layout | M |
| 4 | `KeyRoute` uses `track` only as a trigger to fetch again | `KeyRoute.tsx`, `service/route_progress.go` (`Compare` reads the saved track from disk per call) | Give it a `live` flag, not an overloaded prop; cache the track read | S |
| 5 | Each `subscribe` opens its own `EventSource` in a browser | `api.ts` | About 5 streams approach the 6-connection limit per host (`cmd/site` only; the app is unaffected). Share one | S–M |
| 6 | Seven separate 60 s `useNow` intervals | `CombatLogs`, `Home`, `MPlusLive`, `RaidPage`, `SettingsPage`, `WCLImport`, `WaitingCard` | Duplicated timers that keep ticking while hidden. One shared clock | S |
| 7 | The byte-for-byte Learn check is manual | CLAUDE.md rule, `app.readInto` | A small golden-fixture test would catch changes to saved keys and pulls in CI | M |
| 8 | Gin could be `net/http` `ServeMux` | `controller/` | Smaller binary and fewer dependencies; swaggo ties routes to Gin, so weigh the cost | L |
| 9 | Importing from Warcraft Logs reloads the tracker once per fight | `service/wclimporter.go` | Double reload; read once at the end | S–M |
| 10 | `icon.ico` exists twice | `cmd/app` | Duplicate file in the binary and repo | XS |
| 11 | The keys list is fetched twice on one page | `api.ts` callers | Share the in-flight request as `fetchDungeonMap` does | S |
| 12 | Lines over 120 columns in `api.ts`, `replay.ts`, `KeyDetails.tsx`, `controller/router.go`, `service/logfields_test.go` | | Breaks the repo's rule; no formatter enforces it | XS |

## To check in the running app (PR #77)

- WebView2 reports `document.hidden` when Wails hides a window to the tray (the `subscribe` gate depends on it).
- The overlay shows no stale data when it appears (it keeps its last content until the first read returns).
- Replay, the Route tab opening and closing, and `/swagger` under `go run ./cmd/site`.

## Tried and dropped

- Interning log-line strings: +8% time for little memory.
- React Compiler, code-splitting, gzip and UPX: no gain worth the cost.

## Done

- PR #77: Learn peak memory 1,019 MB → 327 MB, exe 41 MB → 21.7 MB, replay seek and idle work (`CHANGELOG.md`).
