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
| 8 | Gin could be `net/http` `ServeMux` | `controller/` | The app links 350 packages; Gin's binding pulls in `validator` and `protobuf`. Smaller binary, fewer dependencies; check that swaggo still works | L |
| 9 | Importing from Warcraft Logs reloads the tracker once per fight | `service/wclimporter.go` | Double reload; read once at the end | S–M |
| 10 | `icon.ico` exists twice | `cmd/app` | Duplicate file in the binary and repo | XS |
| 11 | The keys list is fetched twice on one page | `api.ts` callers | Share the in-flight request as `fetchDungeonMap` does | S |
| 12 | Lines over 120 columns (see 30) | | Superseded by 30 | XS |
| 13 | Saved tracks are plain JSON, about 2.5 MB each | `clients/cachefile`, `service/track.go` | The dev cache holds 122 MB of tracks of 155 MB total (the Store app's cache is 795 MB). One track gzips 5.4x (2.5 MB to 470 KB). Store `.json.gz` and read through it; keep the byte-for-byte Learn check on the decoded JSON | M |
| 14 | A track's points repeat their keys: `{"t":..,"x":..,"y":..,"map":..}` per sample | `models.KeyTrack` | Columnar arrays (`t[]`, `x[]`, `y[]`) cut the file, the API response and the parse time before gzip. Changes the API and `readerVersion`; do it with 13 | M-L |
| 15 | A track is decoded from disk on every request that needs it | `Routes.Compare`, the track handlers | A 2.5 MB decode per open of a saved key or Route tab. Keep the last one or two decoded (small LRU), after measuring | S |
| 16 | Nothing caps the saved keys, pulls and tracks | `service/logs.go` (auto rules cover only game logs and imports) | The cache grows for good (795 MB on the owner's machine). Offer an auto rule to drop or compress tracks older than N days, as the archive rule does | M |
| 17 | No size or speed budget is checked in CI | `Makefile`, CI | The exe (21.7 MB), the JS bundle (405 kB, 128 kB gzipped) and Learn's peak memory (327 MB on 2.2 GB of logs) were measured by hand once. A CI step that fails past a margin keeps them from creeping back | S-M |
| 18 | The app's idle memory and start-up time are unmeasured | `cmd/app` | Only Learn was measured. Record private bytes at idle, after loading a night's log and after a replay, and the time to first paint, so later changes have a baseline | S |
| 19 | Repeated fetches are shared one by one | `api.ts` callers | Only the dungeon map and spells share an in-flight request. Audit pages that fetch the same URL (keys, routes, mobs) and share them in one place | S-M |
| 30 | Lint and CI don't enforce the rules: 120 columns (about 160 lines over, in about 30 files), `swag` output being regenerated, `cmd/app`'s Windows tests (only run on tags), `staticcheck` | `eslint.config.js`, `.github/workflows` | A max-len rule, `golangci-lint` with `lll`, `unused`, `errcheck`; `swag init` then `git diff --exit-code`; a windows-latest job. Replaces 12 | S |
| 31 | Stale docs: README's review tabs and "Missing buffs" card, `spec-rules.toml` in the layout notes and `build.sh`, `cmd/app/embed/README.md`, the `cmd/site` API description, `store-listing.md` ("matches 0.4.3"), no `[0.4.4]` heading | `README.md`, `CHANGELOG.md`, `docs/store-listing.md`, `cmd/` | Fix in one docs PR | XS |

## To check in the running app (PR #77)

- WebView2 reports `document.hidden` when Wails hides a window to the tray (the `subscribe` gate depends on it).
- The overlay shows no stale data when it appears (it keeps its last content until the first read returns).
- Replay, the Route tab opening and closing, and `/swagger` under `go run ./cmd/site`.

## Tried and dropped

- Interning log-line strings: +8% time for little memory.
- React Compiler, code-splitting, gzip and UPX: no gain worth the cost.

## Done

- Whole-repo review bugs, fixed in `fix/review-bugs`: a stale cache stands in when the download fails (an over-time key was saved as Timed); a saved key's or pull's error clears (`useSaved`); open buffs, spike stretches and mass kills come out in a fixed order; `Reload` keeps keys and pulls not yet saved; a cancelled hover no longer blocks a spell; `LinkReports` and `RemoveImported` write outside the tracker's lock; the Recap's key length is the key's clock; an empty overlay column list is kept and duplicates are refused; a second launch during start-up can't panic; the waiting card follows the log; the missing-credentials message is neutral.
- Frontend dry-up (`chore/frontend-dry`): `useFetched` for the fetches that don't keep their last answer, `<FixNext>`, `<Tiles>`, `<SavedBefore>`, `disc()`, tab ids and the ARIA tabs pattern, replay hot spots, CSS tokens, ES2024 target, `FightKind`, wrapper tests cut (`CHANGELOG.md`). Left: `MPlusLive`, `RaidPage`, `WaitingCard` (it re-reads on `subscribe`) and `useMobs` keep their last answer while the next one loads (a list that never flashes empty), `ComparePlayer` sets its answer from the run button too; the lane labels (10 px on fixed 14 px lanes) and the tile numbers (1.6rem) keep their own sizes.
- PR #77: Learn peak memory 1,019 MB → 327 MB, exe 41 MB → 21.7 MB, replay seek and idle work (`CHANGELOG.md`).
- Backend DRY (items 20 to 25, `chore/backend-dry`): `Key` / `RaidPull` twin code (upsert, save, link, remove, `keystore.Folder[T]`), `cachefile.Fresh` bytes and `ReadJSON[T]`, one `config.Load`, one combat-log constants table, fewer track decodes in `Comparer` and `DungeonMap`, `Importer` files behind `importstore` and `service.SaveSettings`.
