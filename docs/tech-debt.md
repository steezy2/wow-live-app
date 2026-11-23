# Tech debt and findings

The running list of what can be improved in the code base. Add an item when a review finds one that isn't fixed in
the same change; move it to **Done** (with the PR) when it lands. Size: S under 50 lines, M a day, L a design change.

## Open

| # | Item | Where | Why it matters | Size |
|---|------|-------|----------------|------|
| 1 | Every saved key and pull is kept fully in memory | `LogTracker.history` / `pulls` in `service/mplus.go` (about 40 call sites) | Memory grows with history; load on demand instead | L |
| 2 | The overlay window is always created, even hidden | `cmd/app/main.go` | A second WebView2 renderer costs tens of MB when the overlay is off | M |
| 3 | `Tabs` mounts every panel up front | `Tabs.tsx` | A key page fetches routes, comparison, dungeon map and mobs even if the Route tab never opens. Lazy mount conflicts with the "as tall as the tallest" layout | M |
| 7 | The byte-for-byte Learn check is manual | CLAUDE.md rule, `app.readInto` | A small golden-fixture test would catch changes to saved keys and pulls in CI | M |
| 8 | Gin could be `net/http` `ServeMux` | `controller/` | The app links 350 packages; Gin's binding pulls in `validator` and `protobuf`. Smaller binary, fewer dependencies; check that swaggo still works | L |
| 9 | Importing from Warcraft Logs reloads the tracker once per fight | `service/wclimporter.go` | Double reload; read once at the end | S–M |
| 10 | `icon.ico` exists twice | `cmd/app` | Duplicate file in the binary and repo | XS |
| 16 | Nothing caps the saved keys, pulls and tracks | `service/logs.go` (auto rules cover only game logs and imports) | The cache grows for good (795 MB on the owner's machine). Offer an auto rule to drop or compress tracks older than N days, as the archive rule does | M |
| 17 | No size or speed budget is checked in CI | `Makefile`, CI | The exe (21.7 MB), the JS bundle (405 kB, 128 kB gzipped) and Learn's peak memory (327 MB on 2.2 GB of logs) were measured by hand once. A CI step that fails past a margin keeps them from creeping back | S-M |
| 18 | The app's idle memory and start-up time are unmeasured | `cmd/app` | Only Learn was measured. Record private bytes at idle, after loading a night's log and after a replay, and the time to first paint, so later changes have a baseline | S |
| 19 | Repeated fetches are shared one by one | `api.ts` callers | Only the dungeon map and spells share an in-flight request. Audit pages that fetch the same URL (keys, routes, mobs) and share them in one place | S-M |
| 34 | `controller_test.go` repeats its request and router setup (16 `httptest.NewRecorder()` calls) | `controller_test.go` | `serve()` and `newRouter()` helpers (about -60 lines) | S |

## To check in the running app (PR #77)

- WebView2 reports `document.hidden` when Wails hides a window to the tray (the `subscribe` gate depends on it).
- The overlay shows no stale data when it appears (it keeps its last content until the first read returns).
- Replay, the Route tab opening and closing, and `/swagger` under `go run ./cmd/site`.

## Tried and dropped

- Interning log-line strings: +8% time for little memory.
- React Compiler, code-splitting, gzip and UPX: no gain worth the cost.

## Done

- Tracks (`perf/compact-tracks`): saved tracks are `.json.gz` (`cachefile` gzips by the `.gz` name; `keystore.CompressTracks` converts the old ones at start) and `keystore.Recent` keeps the last two decoded (items 13 and 15). Item 14 is dropped: columnar `t[]`/`x[]`/`y[]` points measured 11% off a track before gzip and 4% after, since the points are a small part of it.
- Shared state (`chore/frontend-shared-state`): one browser event stream (`subscribe`), one clock per tick (`useNow`), the keys list shared while in flight (`sharedRead`), `KeyRoute`'s `live` flag; the page keeps its width while a tab loads; one button look.
- Whole-repo review bugs, fixed in `fix/review-bugs`: a stale cache stands in when the download fails (an over-time key was saved as Timed); a saved key's or pull's error clears (`useSaved`); open buffs, spike stretches and mass kills come out in a fixed order; `Reload` keeps keys and pulls not yet saved; a cancelled hover no longer blocks a spell; `LinkReports` and `RemoveImported` write outside the tracker's lock; the Recap's key length is the key's clock; an empty overlay column list is kept and duplicates are refused; a second launch during start-up can't panic; the waiting card follows the log; the missing-credentials message is neutral.
- Frontend dry-up (`chore/frontend-dry`): `useFetched` for the fetches that don't keep their last answer, `<FixNext>`, `<Tiles>`, `<SavedBefore>`, `disc()`, tab ids and the ARIA tabs pattern, replay hot spots, CSS tokens, ES2024 target, `FightKind`, wrapper tests cut (`CHANGELOG.md`). Left: `MPlusLive`, `RaidPage`, `WaitingCard` (it re-reads on `subscribe`) and `useMobs` keep their last answer while the next one loads (a list that never flashes empty), `ComparePlayer` sets its answer from the run button too; the lane labels (10 px on fixed 14 px lanes) and the tile numbers (1.6rem) keep their own sizes.
- Lint and CI hold the rules (item 12, and 30 and 31 of the review): `max-len` 120, `golangci-lint` (`lll`, `errcheck`, `staticcheck`, `unused`), the `swag` diff check and a Windows job; stale docs fixed.
- PR #77: Learn peak memory 1,019 MB → 327 MB, exe 41 MB → 21.7 MB, replay seek and idle work (`CHANGELOG.md`).
- Backend DRY (items 20 to 25, `chore/backend-dry`): `Key` / `RaidPull` twin code (upsert, save, link, remove, `keystore.Folder[T]`), `cachefile.Fresh` bytes and `ReadJSON[T]`, one `config.Load`, one combat-log constants table, fewer track decodes in `Comparer`, `Importer` files behind `importstore` and `service.SaveSettings`.
