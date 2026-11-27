# Tech debt and findings

The running list of what can be improved in the code base. Add an item when a review finds one that isn't fixed in
the same change; move it to **Done** (with the PR) when it lands. Size: S under 50 lines, M a day, L a design change.

## Open

Items 35 to 50 come from the whole-repo review of 2026-11-25, newest first. **Confirm** marks a behaviour or UX
trade-off to decide before building it.

| # | Item | Where | Why it matters | Size |
|---|------|-------|----------------|------|
| 35 | Auto-archive reads the whole active log again after every key or pull | `app/logs.go` (`Tidy` → `learn(false)`), `app/app.go` `learn` | With `ArchiveGameLogsDays > 0` the active log's size has always changed, so it is read from the start into a new tracker and the keys reloaded: O(n²) over a raid night, at Learn's peak memory. Skip `Active` logs in that learn (they can't be archived anyway). The biggest cheap win | S |
| 36 | Fights that end in this session stay whole in memory | `service/mplus.go` `upsert`, `raid.go`, `saveFights` | Nothing slims them until a reload or restart, Learn's own tracker included (likely much of its 327 MB peak). After `Save` and `SaveTrack` succeed, replace the list entry with `Slim()` under the lock; check the golden files and a heap profile | S |
| 37 | Disk reads under the tracker's lock | `service/track.go` `Track`, `mplus_view.go` `SavedKey` and `DeleteSaved`, `review.go` `finished`, `raid_view.go` `compared`, `fights.go` `keepLocal` | Gunzipping a track of several MB under `k.mu.RLock` makes the follower's `Add` wait. Copy what's needed, unlock, read (as `RaidTrack` does). Deletes under the write lock need care with save races | S-M |
| 38 | The live-delay log grows on raid nights and parses every timestamp twice | `clients/combatlog/delay.go`, `app/app.go` `Delays.Add` | `Delays.ms` resets only when a key is saved, so a raid night grows one int64 per line. Log at pull ends too (changes `live-delay.log`), and pass the parsed lines to `Add` | S |
| 39 | Pet owners are never forgotten | `service/mplus.go` `owners` | One entry per pet or guardian GUID for the life of the process (now copied out of the line, so small). Drop creature GUIDs at a zone change or a fight's end; keep players' permanent pets | S-M |
| 40 | The dungeon map decodes every saved track of the dungeon on each request | `service/track.go` `DungeonMap`, `walked` | `Recent` keeps two tracks. Cache the walked cells per map, cleared by `Version` or the history's length | S-M |
| 41 | **Confirm:** one unreadable saved file stops the app starting, or breaks the Sim history | `clients/cachefile` `ReadAll`, `NewLogTracker`, `EnableRaid`, `Sims.saved` | The first decode error fails the whole list, and `Sims.prune` then can't delete the bad file. Skip and log it as `combatlog.Archived` does; decide whether keys, pulls and routes skip too | S |
| 42 | The Sim tab reads every saved result once a second | `service/sim.go` `List`, `saved`, `finish`; `Sim.tsx` poll | `List` gunzips every saved sim, results included, under `s.mu` (which blocks progress updates), and the page re-renders even when hidden. Keep a summary index updated on finish, remove and prune, read outside the lock, and poll only running sims | M |
| 43 | No body size cap outside the sim endpoints | `controller/mplus.go` routes, settings `Put`, imports | Only same-origin pages get through, so low risk. One router-wide `MaxBytesReader` once the largest MDT route with drawings is measured | S |
| 44 | Third-party Actions pinned by tag | `release.yml` (`microsoft-store-apppublisher`, beside the Store secrets), `ci.yml` (`golangci-lint-action`) | A moved tag runs new code with the secrets. Pin to commit SHAs and let Dependabot bump them | S |
| 45 | **Confirm:** a key page and the overlay re-render 4 times a second during a key | `KeyDetails.tsx` `tabs` memo (on `clockMS`, for `BuffGaps`), `Overlay.tsx`, `useLiveKey.ts` in the hidden `RoutesPage` and Home's `Status` | Every mounted panel re-renders per tick, and the hidden Routes page stringifies the route twice per tick. Split a `useLiveReading()` (no clock) from `useLiveKey`; move the tick into small clock components. Trade-off: open buff gaps would grow per answer, not per tick | S-M |
| 46 | An older answer can overwrite a newer one | `useLiveKey.ts`, `usePushed`, `replay.ts` `useTrack`, `useSaved.ts`, `WaitingCard.tsx` | Each change event starts a read and nothing drops the older ones. One helper in `api.ts` that resolves only the newest call | S |
| 47 | An open saved fight is read again on every change event | `useSaved.ts` | During a live key each log batch refetches the open saved key. Tag events as live or saved changes in the stream | S-M |
| 48 | About ten fetch effects hand-roll abort and `!signal.aborted` | `ComparePlayer`, `StoragePanel`, `CombatLogs`, `TalentTree`, `KeyRoute`, `Recap`, … | `useFetched` covers most; some keep their last answer on purpose (see Done), which may need a variant | M |
| 49 | **Confirm:** the Sim breakdown's bars are a second bar component | `SimBreakdown.tsx` `BarRow`, `AbilityBars`; `MeterCards.tsx`, `FightMeters.tsx` | Same job and names; merging changes how one of them looks | S-M |
| 50 | **Confirm:** outside sites' URLs live outside `api.ts` | `KeyMap.tsx`, `RouteMap.tsx` (keystone.guru), `RaidReplay.tsx`, `ReviewPage.tsx`, `WCLKeys.tsx` | If "api.ts is the only file with URLs" covers other sites, move them next to `reportPages` | S |
| 3 | `Tabs` mounts every panel up front | `Tabs.tsx`, `KeyRoute.tsx`, `ReviewPage.tsx` | A key page reads routes, the map, mobs and the route comparison even if the Route tab never opens. Mount on first open (the "as tall as the tallest" height can jump once), or pass `active` so `KeyRoute` waits | M |
| 8 | Gin could be `net/http` `ServeMux` | `controller/` | Gin pulls in `validator` and `protobuf`. Smaller binary, fewer dependencies; check that swaggo still works | L |
| 9 | An import reloads everything once per fight | `service/wclimporter.go` `fetchAll` → `Site.readImported` | Each fight decodes every saved key and pull twice (`newKeys`, `Reload`) and builds a tracker, plus `im.Saved()` at the start. One import tracker per run and one `Reload` at the end (or every N fights for progress) | S-M |
| 16 | Nothing caps the saved keys, pulls and tracks | `service/logs.go` (rules cover only game logs and imports), `models.Auto` | The cache grows for good (795 MB on the owner's machine). Saved key and pull JSONs are still plain (7 MB for 79 keys, 16 MB for 87 pulls): gzip them first (`ReadAll` already reads `.json.gz`), then an age rule for tracks | M |
| 17 | No size or speed budget is checked in CI | `ci.yml`, `Makefile` | The JS bundle grew from 405 to 475 kB (128 to 150 kB gzipped) unnoticed. Check the bundle in the frontend job and the exe after `build.sh` in the Windows job, failing past a margin | S-M |
| 18 | The app's idle memory and start-up time are unmeasured | `cmd/app` | Record private bytes at idle, after a night's log and after a replay, and the time to first paint. Items 36, 38 and 39 are the likely growth | S |
| 19 | Repeated fetches are shared one by one | `api.ts` callers | Shared: the keys list, dungeon map, spells. Still repeated: `/api/mplus/live` twice once Routes is open, `/api/sim/items` on every consumable hover (keep answers per item ID like `spellAnswers`), `fetchSavedKeys` from `SavedKeys` and `MPlusLive` | S-M |
| 34 | Controller tests repeat their request and router setup | `controller/controller_test.go` | 16 `httptest.NewRecorder()` and 17 `Router(...)` setups; only GET has a helper. `serve()` and `newRouter()` (about -60 lines) | S |
| 51 | A healer's shown HPS reads low against the top logs | `service/healstats.go` (`baseIntellect`, `gearStats`) | The character's intellect is gear plus base, a top log's comes from combatant info with flask, food and Arcane Intellect, so the head's HPS is about 20-30% under a like-geared log (gains against the character's own gear are fair). Add the default consumables and raid buff to the sheet | S |
| 52 | A trinket's healing scales on its tooltip's first number | `service/healmodel.go` `firstNumber`, `simheal.go` `effectScale` | The first number can be a duration or cooldown, so the scale is 1 and a higher-level copy is valued at the wearer's level. Scale on the number after "for"/"by", or the item's stat budget | S |
| 53 | The heal model's per-spec mastery and haste numbers aren't calibrated | `service/healstats.go` `healSpecs` | QE's `defaultStatWeights` are a copied placeholder, so the numbers come from its mastery multipliers only. Compare the weights with QE's site for a real export of each spec and adjust | M |

## To check in the running app (PR #77)

- WebView2 reports `document.hidden` when Wails hides a window to the tray (the `subscribe` gate depends on it).
- The overlay shows no stale data when it appears (it keeps its last content until the first read returns).
- Replay, the Route tab opening and closing, and `/swagger` under `go run ./cmd/site`.

## Tried and dropped

- Interning log-line strings: +8% time for little memory.
- React Compiler, code-splitting, gzip and UPX: no gain worth the cost.
- Item 10, `icon.ico` twice: the repo keeps one (generated by `cmd/appicon`, gitignored); the exe still carries it as the tray icon and the resource (about 70 KB), not worth the work.

## Done

- Whole-repo review, safe fixes (`chore/review-safe-fixes`, `CHANGELOG.md`): stale sim work files, owner and spell names copied out of log lines, pull numbering by index, `Recent.put` in one lock, meter graph and overlay drill memoized, `bindSim`, the Store secrets through `env:`, `docs/sim.md` out of `CLAUDE.md`.
- Saved fights slim in memory (`perf/trim-saved-fights`, items 1 and 7): `Slim`/`whole`/`Store.Load`, `Folder.Save` refusing a slim fight, and golden files for a saved key, pull and tracks. 134 MB to 65 MB on 209 keys and 623 pulls; what is left is the other fields (casts, lanes, big hits, talents), which the lists and recap read.
- The overlay window (`perf/lazy-history-overlay`) is created on first show (`newOverlay` in `cmd/app/main.go`), item 2.
- Tracks (`perf/compact-tracks`): saved tracks are `.json.gz` (`cachefile` gzips by the `.gz` name; `keystore.CompressTracks` converts the old ones at start) and `keystore.Recent` keeps the last two decoded (items 13 and 15). Item 14 is dropped: columnar `t[]`/`x[]`/`y[]` points measured 11% off a track before gzip and 4% after, since the points are a small part of it.
- Shared state (`chore/frontend-shared-state`): one browser event stream (`subscribe`), one clock per tick (`useNow`), the keys list shared while in flight (`sharedRead`), `KeyRoute`'s `live` flag; the page keeps its width while a tab loads; one button look.
- Whole-repo review bugs, fixed in `fix/review-bugs`: a stale cache stands in when the download fails (an over-time key was saved as Timed); a saved key's or pull's error clears (`useSaved`); open buffs, spike stretches and mass kills come out in a fixed order; `Reload` keeps keys and pulls not yet saved; a cancelled hover no longer blocks a spell; `LinkReports` and `RemoveImported` write outside the tracker's lock; the Recap's key length is the key's clock; an empty overlay column list is kept and duplicates are refused; a second launch during start-up can't panic; the waiting card follows the log; the missing-credentials message is neutral.
- Frontend dry-up (`chore/frontend-dry`): `useFetched` for the fetches that don't keep their last answer, `<FixNext>`, `<Tiles>`, `<SavedBefore>`, `disc()`, tab ids and the ARIA tabs pattern, replay hot spots, CSS tokens, ES2024 target, `FightKind`, wrapper tests cut (`CHANGELOG.md`). Left: `MPlusLive`, `RaidPage`, `WaitingCard` (it re-reads on `subscribe`) and `useMobs` keep their last answer while the next one loads (a list that never flashes empty), `ComparePlayer` sets its answer from the run button too; the lane labels (10 px on fixed 14 px lanes) and the tile numbers (1.6rem) keep their own sizes.
- Lint and CI hold the rules (item 12, and 30 and 31 of the review): `max-len` 120, `golangci-lint` (`lll`, `errcheck`, `staticcheck`, `unused`), the `swag` diff check and a Windows job; stale docs fixed.
- PR #77: Learn peak memory 1,019 MB → 327 MB, exe 41 MB → 21.7 MB, replay seek and idle work (`CHANGELOG.md`).
- Backend DRY (items 20 to 25, `chore/backend-dry`): `Key` / `RaidPull` twin code (upsert, save, link, remove, `keystore.Folder[T]`), `cachefile.Fresh` bytes and `ReadJSON[T]`, one `config.Load`, one combat-log constants table, fewer track decodes in `Comparer`, `Importer` files behind `importstore` and `service.SaveSettings`.
