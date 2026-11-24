# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- **Sim tab, more of Raidbots' Top Gear**: Top Gear lists each slot's items with the highest item level first, and each item has an *Upgrade, socket, enchant…* form: try it at another level of its upgrade track, with an added socket and a gem, or with an enchant. Each change is a variant, simmed as a candidate beside the item (it counts toward the 100 combinations; the options panel says how many combinations the ticks make and about how long they take, and the Sim button waits until it is within the limit). Every sim kind has a dropdown each for flask, food, potion, augment rune and weapon oil (*Default* keeps simc's), a new **Consumables** tab ranks several flasks, foods, potions, runes or oils against each other, and the **Omnium Folio**'s runes can be picked instead of the export's. The lists come from Raidbots' public game data through `go run ./cmd/simoptref` into `reference/sim-options.json` (built into the app); `GET /api/sim/options` serves them and `POST /api/sim/plan` counts a Top Gear request's combinations.
- A **Sim** tab in the desktop app runs character sims on your PC with SimulationCraft, laid out like Raidbots (the character on top with its item level as the game counts it, the kinds as a button group, the gear by slot with Wowhead's item icons and quality colours, the fight's options beside it, results as ranked rows with each swap's icons and a bar for the gain): paste the `/simc` export from the SimulationCraft addon, then run a **Quick Sim** (DPS, each ability's share and the buffs that were up), **Top Gear** (the best combination of the gear you carry; tick the candidates, at most 100 combinations) or a **Droptimizer** (the gain of each piece the season's raid bosses, at a difficulty you pick, and Mythic+ dungeons, at a key level you pick, drop for your spec; at most 300 pieces). The first run asks you to download SimulationCraft's `simc.exe` (the official nightly from this repo's `simc-engine` release over HTTPS, checked against its sha256; a weekly workflow, `simc.yml`, refreshes it from simulationcraft/simc-publish; or use a `simc.exe` you already have); the page warns when the build is more than 14 days old. One sim runs at a time, can be cancelled, and the last 20 are saved. The export is read again by the app on every request and simc only gets a file the app writes from the fields it validated. Only the gear's item IDs leave the PC, to Wowhead for names and icons (`GET /api/sim/items?ids=`, `service.Items`). API: `/api/sim` (desktop app only). Code: `service/sim.go`, `simprofile.go`, `simgear.go`, `simdrops.go`, `clients/simc`, `Sim.tsx`.
- `go run ./cmd/simref` writes `reference/sim-drops.json` (each boss's and dungeon's gear, from wago.tools' journal tables) beside the hand-kept `reference/sim-drops.toml` (item levels per difficulty and key level, taken from the Midnight Season 2 wiki page and not yet checked in game). Tier tokens aren't in it.

### Changed
- The Home tab no longer shows the Blizzard trademark notice and data credits; they are in the README and the Store listing.

## [0.5.5] - 2026-11-23

### Changed
- **The key review's player names have their class colour**, and its party lines (deaths added to the clock, must-stops, time between pulls, priority target) are a bulleted *Party* card beside the players' cards.
- **The tabs under a key or pull are buttons:** boxed, the open one filled.
- **A death line on the overlay is a button** (boxed and tinted red) while the overlay takes clicks.
- **Less memory for saved keys and raid pulls:** the app keeps them without their per-ability lists (about a third of a key, two thirds of a pull) and reads the whole fight from its file when a page, a review or a link needs it. On the owner's 209 keys and 623 pulls the heap for them went from 134 MB to 65 MB.
- **The overlay's window is made the first time it is shown**, not at start: with the overlay off (or no key running yet) its second WebView2 renderer, tens of MB, never starts.
- **Saved tracks are gzipped** (about 5x smaller: 2.5 MB to 0.5 MB), and the app compresses the ones already saved once, in the background, when it starts. A saved key's map, route and review read the last two tracks from memory instead of decoding the file each time.
- **One look for buttons that do something** (Settings, Home's data, report links, quick links, the route editor and the Routes sidebar): boxed, the main action filled, a destructive one red.
- **Fewer connections and timers:** every page in a browser shares one event stream, the 60 s clocks share one timer that stops while the page is hidden, and the saved keys list is read once when two parts of a page ask at once.
- **A key's Route tab** reads its comparison again only while the key runs and the tab is open (it took the track just as a trigger).

### Fixed
- **One long phrase can't shrink the overlay:** a line of text (a death's ability, a fix, a boss's name) wraps to the width of the cards and bars instead of setting it, so the overlay's zoom no longer drops to its minimum.
- **A page no longer starts narrow and moves to the middle** when its tab opens: the page keeps its full width while its content loads.
- **Names in a damage spike's list** have their class colour.

## [0.5.4] - 2026-11-17

### Added
- **The overlay can rank by DPS or HPS** (rows and bars), and show a DPS card and an HPS card (healers only) of the key. The raid overlay follows the same rank and has an option for one ranked card in place of its Damage and Healing cards (off, as before).
- **The raid overlay has its own row counts** for the Damage and Healing cards (5 and 3 by default, 0 for everyone) and an option to put the two cards side by side.
- **Ctrl+Alt+H hides the overlay** (or the tray's *Hide the overlay*) until the next key or raid pull.
- **A death's recap shows over the overlay** for 3 to 60 seconds (10 by default, or off) without a click: a new Mythic+ death's. A death line on the raid card opens its recap on a click.
- **Preview the overlay with dummy data** from Settings (*Preview Mythic+*, *Preview raid*), to try every option without a key or a pull.
- **Overlay layouts** (Compact, Standard, Full) fill in the card options in Settings in one click.

### Changed
- **A click on a player in the overlay shows the main window's two cards in one:** damage by ability and into each enemy, then healing by ability and the damage they took. It works on the raid overlay's meters too (the whole pull), and in both previews. The breakdown lists its rows in full, with no scrollbar.
- **The raid overlay no longer opens on a death's recap** after a pull; only a new Mythic+ death shows its recap by itself (Settings has the seconds), and a click on a raid death line opens its recap.
- **Death lines look clickable** on the overlay (a pointer and an arrow) while it takes clicks.
- **The overlay window keeps the shape of its content** while it is resized (drag a corner), so it never has a spare side or a scrollbar.

## [0.5.3] - 2026-11-17

### Changed
- **The M+ Live bottom tabs are as tall as the raid page's** (640 px lists instead of 320 px), so the other tabs match the bigger Route.
- **Clicking a boss shows the party's meter cards on it**, and a click on a player drills into two cards: their damage by spell over their damage into each enemy, and their healing by spell over the damage they took.
- **Home's quick links look like buttons**, bigger and with an arrow, and wrap on a narrow window.

### Fixed
- **Clicking a boss no longer scrolls the page** up to the replay; it moves the replay and stays where it is.

## [0.5.2] - 2026-11-16

### Changed
- **M+ Live opens on All characters** the first time it is opened in a session, then the top bar keeps the last pick (it used to start on the Settings character).
- **Store listing rewritten** shorter, with a Routes section and screenshot, with a new Compare screenshot (sample data) and a death recap showing health falling to 0%, leading with death recaps and the comparison with top runs; `docs/video-script.md` has a 50 s trailer and a 3 min walkthrough script.
- **Credits**: the Home tab now carries the Blizzard disclaimer and credits for Keystone.guru, Mythic Dungeon Tools, Raidbots, wago.tools, Warcraft Logs, Raider.IO and Wowhead (the README said the page credited them); the Store manifest's description mentions raid pulls.

### Fixed
- **Privacy policy** lists `wow.zamimg.com` (spell and talent icons), which it missed, and drops the GitHub hosts: only the build-time `cmd/mplusref` calls them, the app doesn't.

## [0.5.1] - 2026-11-16

### Changed
- The app always opens on **Home** (it was M+ Live, or Settings when the log folder was missing; Home's guide already shows a missing log folder).
- `npm audit fix` in `frontend/` bumps `minimatch` (a dev-only denial-of-service advisory reached through ESLint); `npm audit` now reports 0 vulnerabilities.
- **Less duplicated backend code, same saved output** (a fresh `--learn` writes byte-identical keys and pulls): one `Fight` method set on `Key` and `RaidPull` behind one upsert, save, link and remove-imported path and one `keystore.Folder[T]`; `cachefile.Fresh` returns the bytes, with `ReadJSON[T]` and `Forever`; one `config.Load(fsys)` for `cmd/site` and the app's built-in files (`build.sh` now puts the reference files in `embed/reference/`); the combat log's unit flags and raid difficulties in one table (`clients/combatlog`) that the Warcraft Logs converter and the reader share; the Warcraft Logs import's files behind `clients/importstore`, and the settings save in `service.SaveSettings`.
- A player review reads the player's newest 10 similar runs (not all), each track once, and fails on a track it can't read instead of leaving it out.
- **Frontend dry-up** (about 60 lines fewer, tests included; the pages look alike apart from a few colour and spacing steps that now follow the tokens): `Home`, `KeyRoute`, `Recap` and the raid track read through `useFetched`; one `<FixNext>`, `<Tiles>`, `<SavedBefore>` (the "saved before … were kept" lines now share one wording), `PlayerPicker`, `disc()` and `FightKind` replace their copies. The replay does less per tick: the walked-floor bounds are computed once, `sumStretches` walks its stretches once instead of searching them for each, the route map's backdrop and deaths are stable, and the review page's still tabs are built only when what they show changes.
- Tabs have ids instead of positions and name prefixes, and follow the ARIA tabs pattern (arrow keys, Home and End move between them, only the open one is in the tab order, each tab names its panel).
- CSS spacing and font sizes come from the `--s` and `--t` tokens, and the route map's unpulled and missed colours from `--muted` and `--bad`. The TypeScript target is ES2024 (`Map.groupBy`, `toReversed`, `at`).
- Code and CI only, no behavior change: CI also runs `golangci-lint` (`backend/.golangci.yml`: `lll` at 120 columns, `errcheck`, `staticcheck`, `unused`), checks that `backend/docs` is what `swag init` writes, and runs the desktop app's tests on Windows; ESLint's `max-len` holds the page's 120 columns. The lines over 120 columns were wrapped (about 50 in the page, 145 in the backend).
- Docs: the README's review tabs (and the raid Buffs tab), `spec-rules.toml` in the layout notes, `build.sh`, `cmd/app` and its embed folder, the API description (now covers raid pulls, routes, the recap and the settings), the Store listing's version and the missing `[0.4.4]` heading.

## [0.5.0] - 2026-11-16

### Changed
- **Faster reviews and listings**: a player review reads its top logs five at a time, each log's tables, enemy hits and talents together, and the character's parses beside them, instead of one query after another. Listing a character's Warcraft Logs fights asks the next page of reports while the last is read, and a page's reports' fights in side-by-side queries of 8. Reading every log again takes about 20 s for 1.5 GB (the tracker is the limit, one line at a time), so it is unchanged.

### Added
- **Talent icons, tooltips and both hero trees**: the compare tab's talent tree shows each talent's icon (the app fetches them from Wowhead once and keeps them) and what it does on hover, with who took it below; both hero trees are drawn (Arcane's Sunfury and Spellslinger, and every spec's two). Each node has a thick ring and a corner mark for who took it (! top logs only, + you only, ≠ another rank, ✓ both) and a missed talent shows how many top logs took it; the talents listed under the tree are buttons that scroll to the talent and pulse it for 5 seconds.
- **Compare talents**: a player review draws the spec's talent tree with what the top logs take that you didn't, what only you took and where you both agree, with how many of the top logs took each. Keys and pulls read from now on (or with *Read old logs*) keep their talents; older ones say so.

### Fixed
- **Compare tab**: a stray "0" under *What to work on* (the spec rules count showing when there are none) is gone, and a dungeon review no longer lists a Vantus Rune (raid-only, from `raid-rules.toml`) as a buff you lacked.
- **Compare buffs**: always-on talent buffs (Charged Orb, Orb Barrage...) that read 100% for the top logs and 0% for you are left off the buff list.
- **Compare damage taken**: the top logs' damage taken counted only hits from friendly sources (a mage's own Refraction), so it read about a tenth of yours; it now adds the enemies' hits, matching the key's own numbers within 0.3%. Reviews made before this are read again (run them once more).
- **Compare buffs**: a buff with several spell IDs (Overpowered Missiles read 121% for the top logs) counts its time up once, so no buff passes 100%.
- **Key page**: a running key's Replay map holds its place while the track loads, so the tabs below (Route) no longer jump when the map appears.
- **Forces**: a finished key no longer reads "100% · 546/551" when the log missed a mob's death; the count shows only when it adds up.

## [0.4.9] - 2026-11-16

### Changed
- **Routes tab**: typing a new name offers *Save as new route* (the original stays as it was) and *Rename* (saves over it); *Delete route* is beside them. *From MDT's saves* lists only the picked dungeon's routes.

### Added
- **Record next run as new route** on the Routes tab: arm it for a dungeon and the next key that ends there is recorded as a route, as *Record route* does on a finished key. *Record route* and this button are filled buttons now.

## [0.4.8] - 2026-11-16

### Changed
- **A finished key's Route tab shows what was pulled**: the map draws the key's own pulls (the plan only as faint dashed outlines) and each pull is listed against the route pull it is closest to: *Plan pull N* when it matches, *N extra, M not killed* when it differs, *Not in the route* otherwise. Planned enemies the log never showed dying are listed as *not seen in the log*, not as missed. A running key's tab is as before. `GET /api/mplus/keys/{start}/route` has the new `run`.
- **Record route keeps every kill**: a kill pulled differently from the map (more than 30 yards from an enemy of its NPC) used to be dropped; now it takes the nearest unused enemy of its NPC on its floor, so the recorded route holds what was pulled. `GET /api/mplus/keys/{start}/recorded-route` returns `{pulls, pct, dropped}`, and the page says the forces share recorded and the kills not on the map. The route's name has the key's date.

## [0.4.7] - 2026-11-16

### Added
- **Record route**: a key's Route tab has a *Record route* button that makes a new route (starred when the dungeon has none) from the pulls the key made: its kills matched to map enemies, grouped by the key pull they died in. `GET /api/mplus/keys/{start}/recorded-route` gives those pulls.

### Changed
- **Smaller and faster**: the app's exe is about half its size (`-s -w -trimpath` in `build.sh`, and the Swagger UI, which the app couldn't serve without its spec, now only comes with `go run ./cmd/site`). *Read old logs* peaks at about a third of the memory (1,019 MB to 327 MB on 2.2 GB of logs) with the same saved keys and pulls, byte for byte: finished keys and pulls are saved every 100,000 lines instead of when the log ends, at most 4 goroutines parse, saved tracks are freed, and two per-line string splits no longer allocate. Following a log from its start reads it with one buffer. The replay is about 3 times lighter per move (the graph's numbers by binary search, review tabs built once instead of on every tick, no remount when a tab opens or its count changes, one render per live tick). Idle: the live clock only ticks while a key runs, a window hidden in the tray reads once when it shows, the Route tab re-reads a running key only while open, and a key's page asks for its dungeon map once. The built page's `assets/` are cached for good.
- A running key's route map labels its two live pulls **Current** and **Next** instead of their numbers, and outlines them in colors that run along the line (still, with reduced motion).
- The replay's floor buttons are numbered in the dungeon's own floor order, as the route map's are, not by where the party stood most (Voidscar Arena's route Floor 1 was the replay's Floor 3).

### Fixed
- A key finished while Raider.IO can't be reached and its saved timers are over a week old is no longer saved as timed: the old timers are used. The same for the other sites' cached data.
- A saved key or raid pull that failed to load no longer keeps its error up after the next one loads. The waiting card follows the log as it is written.
- *Read old logs* saves the same files every time: open defensives, spikes and mass kills no longer depend on map order. A key or pull that ends while an import reloads the lists stays in them, and an import's file writes no longer hold up the live log reader. Hovering a spell and moving away no longer blocks it for a minute. The Recap counts an abandoned key's time like every other page. An empty overlay column list can be saved, and a column can't be picked twice. A second copy started while the app opens no longer crashes it.
- A key's kills match the route far more often: a mob counts where it was first seen (a pulled mob runs to the party before it dies), up to 30 yards from its enemy, and the closest kill and enemy pair match first. On five saved keys' logs this took matches from about 40% to 80%.
- **Ruby Life Pools** had its two floors' map IDs swapped against the combat log (every kill stood on the other floor's enemies), so nothing matched a route and its enemies drew on the wrong floor. `reference/mplus-dungeons.json` and the generator now use the log's IDs; a saved route's drawings there sit on the other floor and need drawing again. Every other dungeon's kills fall on their own floors.

## [0.4.6] - 2026-11-15

### Added
- **Live route tracker**: a running key's Route tab shows each pull of the route as done (muted green), next (the two unfinished pulls after the last one reached, in their colors with a "next" tag), later (faded) or missed (red outline: unfinished though a later pull was reached, with its unkilled mobs and forces listed), and a *Forces x% vs y% planned* line when the key trails the plan by 1 point or more. The log shows nothing inside a fight, so the pull in progress is the first unfinished one. The live header adds `· N missed` and `· forces behind plan`, and any route that plans under 100% forces says so. `GET /api/mplus/keys/{start}/route` returns each pull's `state` plus `last`, `missed` and `behind`; `GET /api/mplus/live`'s route adds `missed` and `behind`.
- **Mob data** for the route editor: `reference/mplus-dungeons.json` now keeps each dungeon's mobs (`mobs`: name and spells with Interruptible/Magic from Mythic Dungeon Tools; level, type, class, base health and affix flags from Keystone.guru's `npcs.json`). `GET /api/mplus/dungeons/{mapID}/mobs?level=N` scales them to a key level (2 to 40, by default the top bar character's newest saved key there, else 10) the way Keystone.guru does: 7% a level through +10, 10% a level after, Fortified on trash and Tyrannical on bosses from +10, trash 5% lower up to +5. It matched every mob in a +17 Temple of Sethraliss log to the point. No source has a mob's own damage, so it gives the level's damage multiplier only. `GET /api/spells?id=` explains the spell of a dungeon mob by its ID (404 for any other).
- The route editor reads the dungeon's **mobs at a key level**: a +level box in the toolbar (starting at the top bar character's newest key there), the mouse on an enemy tells its name, health at that level and forces, and a right click opens its card (type, class, level, health, the level's damage multiplier, forces, affix flags, spells with their text and Interruptible/Magic marks) with Add to / Take out of the selected pull for it or its pack and Select its pull. Each pull row lists its mobs by name (`3× Name`, a click opens the card) with their health, hovering a row highlights its pull on the map, and a right click on a row moves, deletes or recolors the pull.
- **Patrols** on the route map: each patrol Keystone.guru draws on the dungeon's newest mapping is a dashed line in its color (a *Patrols* button, on by default, hides them; it only shows in a dungeon that has any), and a patrolling mob's card says whether it walks a loop or back and forth. `reference/mplus-dungeons.json` keeps them (`patrols`, from each floor's `enemy_patrols.json`) and `GET /api/mplus/dungeons/{mapID}/map` serves them; this season only Temple of Sethraliss has one on its newest mapping.
- **Drawings** on a route's map: the editor's toolbar has Select, Line (freehand, simplified as you let go), Marker, Note and Erase, in any pull color, per floor and on Undo like any other edit; a key's Route tab shows them too. They are saved with the route (`drawings` on `PUT /api/mplus/routes/{id}` and in the route's reply, with the `limits`: 200 drawings, notes of 200 characters, lines of 500 points); the site refuses a bad kind or color, text off a note, or a point off the dungeon's floors. Routes saved before have none. MDT strings carry them both ways: an import reads MDT's shown lines and note pins (a pin without text is a marker) onto the floors and says how many it can't show (`droppedDrawings`: any off a floor or past the limits), and an export writes the route's drawings onto MDT's map and keeps those, and the ones the app doesn't read (undone drawings, text labels, lines erased apart), as they were. An imported drawing you leave alone exports as MDT wrote it (its brush size, an arrow); one you move or change goes back at MDT's default brush size, as a plain line. Each floor's place on MDT's map (`mdt` on the dungeon's floors) is fitted from where MDT and Keystone.guru both put its enemies.
- The route editor shows the route's **forces total** (percent, and forces of the dungeon's total, green from 100%) under the pull buttons.

### Changed
- **Route editor**: a click on an enemy puts its whole pack (Keystone.guru's, else MDT's group) in the selected pull or takes it out, Shift+click the enemy alone (a click on a pack part-way in another pull moves all of it here); a click anywhere on a pull's row selects that pull. The key levels the +level box accepts come from `GET /api/mplus/dungeons/{mapID}/mobs` (`minLevel`, `maxLevel`).

### Fixed
- `npm run dev` no longer gets a 403 on saving, importing or deleting: Vite's proxy now sends the API's own `Origin`.
- A website's `<img>`, `<script>` or `<iframe>` can no longer make the browser site (`go run ./cmd/site`) call its API (such requests carry no `Origin`, so the check missed them; the browser's `Sec-Fetch-Site` marks them), and a page on another port of this PC is refused too. A link to a page of the site still opens.
- *Read old logs* that stopped part-way (quit from the tray, or one log failing) after a new reader version no longer leaves the archived logs and Warcraft Logs imports unread: the next read goes on with them.
- A mob spell's hover box shows its name at once and its text when Wowhead answers; a name the combat log hasn't shown yet is asked again on the next hover instead of staying blank until the page reloads.
- The route map's Note tool keeps its text when the route is full, and a full route switches the tool back to Select. Drawing a line no longer redraws the map on every mouse move.
- `make ref` fails on a mob Keystone.guru has no health for, instead of writing 0; Warcraft Logs report codes are checked before they name a cache file.

## [0.4.5] - 2026-11-15

### Added
- The replay's focused player and picked enemy show a health bar in their box (kept on the track now, so saved keys are read again).

### Fixed
- A key finished over its timer is depleted, not timed (the log's success flag only says it was completed); saved keys are read again to fix it.

### Changed
- **Replay** moves players like Warcraft Logs': a dot glides in a straight line between two samples up to 10 s apart (however fast the move) and is hidden when its player hasn't been sampled for 10 s, instead of freezing and then jumping; a dead player's cross stays.

## [0.4.4] - 2026-11-14

### Fixed
- A player review no longer drops an unreadable track silently; a key's avoidable hits, a pull's stacks and soaks and a review's similar runs keep one order when rebuilt; deleting a saved key tells open windows and the overlay; an expired Warcraft Logs token is fetched again instead of failing until restart; the page's inline script (blocked by its policy) is a file, so the overlay opens without a splash; the Home tab logs a failed recap; a repeated log name is refused.

### Changed
- Code only, no behavior change: the settings' cleaning and character rule are `service.CleanSettings` and `CharacterNeedsCheck`; the import's files are written by `cachefile.WriteJSON`; one `wclAnswer` maps review and compare errors to statuses (and a raid pull's missing-pull 404 shares `respondError`); a key's status is `models.StatusRunning`, `StatusTimed`, `StatusDepleted`, `StatusAbandoned`; folder sizes moved to `clients/cachefile/size.go`; names only used in their own package are unexported. A player's parse reads the same everywhere (rounded down, as Warcraft Logs shows it).
- The privacy policy and README say a reviewed player's name and realm go to Warcraft Logs; the Store listing matches 0.4.3.
- **Home's combat logs** are grouped by the day they were written, the last 5 days one box each and everything older in one; a box picks all of its logs. *Read old logs* and *Read everything again* look like buttons.
- **Import from Warcraft Logs** always suggests `Name-Realm` in the character box; `characterGuess` is gone from `/api/settings`.
- **Replay**: a boss's name shows only once you pick it, names are outlined (a raider's in their class color, over their dot; an enemy's under it) and picking an enemy lists what each player has done to it (bottom right).

## [0.4.3] - 2026-11-13

### Added
- **Routes** for Mythic+: import a Mythic Dungeon Tools route (paste its export string, or pick it from MDT's saved routes), see its pulls and forces % on the dungeon's map, edit them (move a pack to another pull, new, delete and reorder pulls, undo) and copy the route back as an MDT string. One route per dungeon is starred: the live page and overlay show its next pull with forces, and a key's **Route** tab shows which planned pulls were done, skipped or added. Enemies on each dungeon's map come from `reference/mplus-dungeons.json` (`go run ./cmd/mplusref`, from MDT's clones joined to Keystone.guru's positions). An import keeps the checks an edit has: a pull color that isn't 6 hex digits gets MDT's first default, an enemy already in an earlier pull is dropped, and more than 60 pulls are refused.
- **Spec rules** (`spec-rules.toml`, beside `mplus.toml`): a table per spec ("Arcane Mage") of rules a player review checks on the player's own log, in three kinds: `with` (the share of a cast made while a buff was up), `together` (the share of a cast within N seconds of another) and `uptime` (a buff's share of the fight). A rule they fall short of is a line in the Compare tab with its share and its why. The file ships with the three kinds explained and no rules, since each spec's rules need checking against its guide first.
- The full review page's **Compare** tab and a **Logs** button on every player in its header: pick a player, press *Run review* (it says it spends Warcraft Logs points) and read the verdict, what to work on, then key stats, abilities and buffs as "you" against your usual, the top 20 logs' median and the best log, the abilities only one side used, each top log linked to Warcraft Logs, and the character's parse here. A stored review shows at once and has its own address (`?tab=compare&player=`), so it pops out on its own.
- **Player review** (its endpoints; the Compare tab shows it): `POST /api/mplus/keys/{start}/players/{player}/compare` and `/api/raid/pulls/{start}/players/{player}/compare` (and `GET` for the stored one) put a player's run next to their own similar runs (same dungeon within 2 levels, or the same boss and difficulty, in the same spec), the 20 top Warcraft Logs logs of their spec nearest in time (read over the same stretch for a wipe or an abandoned key) and their own parses there: key stats, an ability row each (casts a minute, or counts for ones cast at most once a minute), buff uptimes, advice costliest first and a plain verdict. It costs about 8 Warcraft Logs points a log, so about 165 for the 20 (measured Nov 13); it isn't run with fewer than 200 left. New `wcl.Client.RankedRuns`, `RaidEncounters` and `RunPlayer`, `service.Comparer`.
- The full review page gets a **Player** tab (a player's tiles, the cooldowns they cast at most once a minute on the fight's clock, the spikes that hit them and what covered them, their buff uptimes, flask, food and combat potions against the Bloodlusts, and their meters drilled in) and a **Deaths** tab (with who brought each player back). The combat log now also saves each ability's spell ID (`KeyAbility.id`, `TrackCast.i`), each player's own buffs as stretches (`KeyTrack.buffs`), resurrections (`Key.resurrections`) and a raid pull's specs (`RaidPull.specs`). `readerVersion` 17, so *Read old logs* fills them in; older fights say so in the tabs.
- **Open full review** on a finished key and a raid pull opens its own page (`/review/mplus/{start}`, `/review/raid/{start}`, tab and player in the address): Overview (tiles, what to fix, the review, the meters), Interrupts, Events (a feed of deaths, defensives, potions, interrupts, crowd control and dispels, filtered by kind and player) and Timeline (the replay on one clock). *Pop out* opens the same page in a window of its own (one per page; a new tab in a browser). The tabs are the shared ones (`ReviewPage.tsx`, `EventFeed.tsx`, `fightEvents` in `events.ts`, `pullFightData` in `KeyTabs.tsx`); the Player, Deaths and Compare tabs are below.
- A **Home** tab, first in the top bar: a setup guide on first run (log folder, a log written, a character, Warcraft Logs keys), then status, quick links, this month's numbers and **Your data**: the space each folder takes, the combat log list (now with Warcraft Logs imports, which can be deleted) and optional auto rules that archive old game logs and delete old imports, both off by default. The guide can be shown again from Settings.
- A splash while the app starts, so it opens on the right tab without flashing; short loads show nothing and longer ones a placeholder instead of "Loading…", and a saved key, pull or list stays on screen while the next one is read.
- Between pulls and keys the Raid and M+ pages show what the app is waiting for: the log folder and when the game last wrote, a three-step checklist, and tonight's pulls or today's keys.
- The timer has an underline up to your fastest timed time at that level (also in the overlay), and a running key says how much faster or slower than it the estimate is.
- The Recap's M+ tab lists each key with your DPS and HPS and your kicks, CC, dispels and priority damage % beside the party's, in place of the per-key averages.
- Hovering an ability's name (a death's recap, defensives and what was *still ready*, CC, buffs, the ability rows) says what it does and its cooldown, from Wowhead's tooltip for the spell ID the combat log gave the name (`GET /api/spells?name=`, cached a week in `wowhead/`). Names get a dotted underline; an ability the log hasn't shown since the app started shows just its name.
- A must-stop cast that wipes the raid when it goes off is the first line of a pull's *Fix next pull* ("Anguished Cry went off 1 time, which wipes the raid"): `wipe_stop` in `raid-mechanics.toml` (Ula'tek's Anguished Cry, the Weakened Doomscale's raid-wide stun), `wipe` on a `KeyStop`. `readerVersion` 15, so *Read old logs* fills it in.

### Changed
- The repo is public, under the [PolyForm Noncommercial License 1.0.0](LICENSE) (source-available: anyone may use it noncommercially, only the author may sell it) instead of all rights reserved. The privacy policy moved here from steezy2/wow-live (`PRIVACY.md`), and so did the issues link, the Store listing text and the User-Agent. Copies from before 0.1.5 stay MIT.
- The overlay is compact: columns sit closer (three columns are about 216 px wide instead of 312), no scrollbar shows while clicking or resizing, the dispels column reads "Dispels", and Pull DPS is offered only with the one-card setup.
- Settings panels stack in columns, so short panels no longer leave gaps.
- In the replay a focused player's casts and damage taken are on the left and the enemy's casts on the right.
- *Read old logs* fills in the names of enemies in already saved keys' replays (`readerVersion` 16).
- Every hover is the timeline's box (`Tip.tsx`, shown at once, above what the mouse is on) instead of the browser's slow plain tooltip: meter cells, key bar marks, the damage graph's point (its player, time and rate), the lanes. A new hover is one `tip(text, head)` or `spellTip(name)` on the element.
- An M+ key's **Buffs** tab reads like a raid pull's: a line per buff, "Missing Arcane Intellect", with who lacked it the whole key (or "everyone") and each gap as a player with its start and end on the key's clock (a click moves the replay). It now also catches a flask, food or party buff that came off mid-key (`buffGaps` on a key; `BuffGap` replaces `RaidMissing`), shown by the shared `BuffGaps.tsx`. `readerVersion` 14, so *Read old logs* fills it in.
- A raid pull's early deaths open the same recap as every other death (the last 10 s with health, heals and buttons) in place of a line of their last three hits.

### Fixed
- The replay named every enemy "Enemy": the track dropped the names the tracker had.
- A priority target is red (and counts as late in the review) only when it died last of its pack, not after more than half of it; a miniboss alone in its pack is never late. `readerVersion` 16, so *Read old logs* fills it in.

## [0.4.2] - 2026-11-12

### Fixed
- Switching quickly between the bottom tabs or between saved keys no longer shifts the page: the scrollbar's space is kept, the left bar stays put, and a saved key fades in once its track is read instead of showing the previous key and then popping in piece by piece.
- The left list of keys no longer hangs 32 px below the window (its padding was outside its height), so its end shows without scrolling the page, and it stays put when the tabs change.
- A death's or big hit's recap fills each event green to the player's health after it, in place of the tiny line graph; a row the log gives no health for (a melee swing, a cast) is worked out from the damage and healing around it and marked `~`. The recap arrow is an accent-coloured button and no longer overlaps the row's text in *Defensives and potions*.
- A raid pull's bottom tabs are twice as tall, and its missing buffs are a *Buffs* tab among them instead of a card under them.
- The bottom tabs are as tall as their tallest tab, so switching between them leaves the meters and everything else where they are (blank space under a short tab).

### Changed
- The Microsoft Store listing matches 0.4.1 (`docs/store-listing.md`) and has new images in `docs/store/`: six screenshots with headlines, poster, box art, tile icon and hero image, from real keys and pulls with made-up names. The README shows the same screenshots instead of the old ones in `docs/screenshots/`.

## [0.4.1] - 2026-11-12

### Added
- Each damage spike names what hit the party in it: the three abilities that did the most, with the enemy that cast each and its share ("Crushing Slam (Bonebreaker) 58%"), on keys and raid pulls alike (`causes` on a spike). `readerVersion` 11, so *Read old logs* fills them in.
- A **Recap** tab: this month, last month, this season (Raider.IO's dates), this year or all time, on a Mythic+ tab and a Raid tab that never count keys and pulls together. Mythic+: boxed tiles (keys, timed %, highest timed, hours, deaths a key), facts that open their key (fastest and closest timed, most deaths in a timed key, deadliest ability, most kicks, longest, most improved), your character (specs, median DPS or HPS per dungeon, what killed you), who you ran with (class colour and realm). Raid: pulls, kills and a card per boss (first kill, best wipe, your deaths, what hit hardest). Both: your Warcraft Logs parses. `GET /api/recap/periods`, `GET /api/recap?from=&to=` (`mplus` and `raid`); read from the saved keys only, no points.
- Keys and raid pulls from your own logs that are in your Warcraft Logs reports get linked to them whenever the import lists the reports (*Find fights*, or **Find my report** under a finished key or pull): their header links to the fight on Warcraft Logs and WoWAnalyzer, the lists tag them, and reading the log again keeps the link. Until then the header offers **Find my report**. `linked` in the import's status.
- Your character's Warcraft Logs parses this season (best and median per dungeon and boss, runs, spec) in the Recap tab: `GET /api/settings/warcraftlogs/parses`, read once a day.
- A character picker in the top bar: All characters or one of yours (every character whose game wrote a saved key or pull, named with its realm in its class colour, Settings' character picked at first). The saved keys, raid nights and the Recap show that character's runs. `GET /api/characters`, and `character` on `GET /api/mplus/keys`, `/api/raid/nights` and `/api/recap`.
- Keys and raid pulls keep each player's realm (`realms`), and raid pulls whose game wrote the log (`logger`). `readerVersion` 12, so *Read old logs* fills them in; until then a character's older runs show under every character of that name.
- The Recap's dungeon cards: runs and timed %, and at the highest level you timed there your timed runs (or *only once*) with their average, median and fastest time (untimed and abandoned keys never count), against the median of Warcraft Logs' top 100 speed-ranked runs at that level (`GET /api/recap/top-times`; a point or so per dungeon and level, kept a week; none without Warcraft Logs keys or under 10 ranked runs).
- The Recap's kicks, CC, dispels and priority damage % per key, the party's beside yours, and your deaths a key beside the party's.
- The overlay takes clicks by default: a click on a player's row shows their top 8 spells and their damage into each enemy over that card's span, the last pull or the whole key so far (the one-card meter: the whole key); Esc, a right click, the title or a new pull goes back. **Ctrl+Alt+K**, a tray item and a Settings switch let clicks through to the game instead (`clickThrough` in the overlay settings).
- A click on a death (the overlay's deaths line, or the Deaths tab) or a big hit shows that player's last 10 s: their health, each hit with who cast it, heals and shields with who gave them, and the buttons pressed (`recaps` on the track; `[big_hits] recap_seconds` in `mplus.toml`).
- **Big hits** are one player at a time: 50% of a tank's max HP (40% for anyone else) within 3 s that left them under 40% health, with what hit them, their cover as a death's and whether they died (`bigHits` on a key and a raid pull; `[big_hits]` in `mplus.toml`).
- Overlay settings: players per card and which columns (DPS, HPS, kicks, pull DPS, taken, avoidable, priority %, dispels, deaths), `rows` and `columns`, the choices served as `overlayColumns`.
- The replay's enemies are clickable: a ring and their casts, bottom right (`mobCasts` on the track; a mob's trail keeps its name).
- *Fix next key* names the DPS or tank who put the least damage into priority targets when it's under 20% of theirs or under half the party's average.
- An M+ key's **Buffs** tab: who was missing a flask, food, rune or party buff at the start.
- `readerVersion` 13, so *Read old logs* fills all of these in.

### Changed
- A new app icon: a purple keystone gem crossed by a live pulse line, in a gold-framed tile, without the red dot. `docs/store-listing.md` holds the Microsoft Store text and the screenshot plan for the next listing update.
- The overlay scales to its window instead of scrolling, and its text is shorter ("25% slower than best +17", "3 deaths +0:45 · last …"). It shows two cards (the pull and the whole key) by default: `oneCard` replaces `split`. New installs start at 90% opacity.
- The Defensives and potions and CC tabs are a card per player that opens to each use's time; a player's deaths are on their Defensives card. The player's Moments tab is gone. CC counts uses, and the tab says so.
- Priority targets (the Minibosses tab, renamed) say how long after the rest of its pack one died and its share of the damage; *Fix next key* reads "Priority target died late in 3 of 4 pulls, last up to 6 s after the rest of its pack (pull 5)".
- The replay leaves out a floor with fewer than 15 player points (stray points on a floor the party never stood on).
- The key and raid pull lists group anything older than 8 weeks by month instead of by week.
- A saved key's and pull's list entry carries its `report` (code and fight) and `imported`, instead of the imported fight's URL; the page makes the links.
- An M+ key's damage spikes are the worst 5 s instead of 3 s, and each spike says how long it is (`seconds`; a raid's is still `spike_seconds`, 6 s).
- Replays start at 1× with no trail.
- The M+ ready check before a key is gone (the live page, the overlay and `GET /api/mplus/ready`): what was missing at the start is in the key's Buffs tab. The overlay no longer shows before a key starts.
- Settings says an import from Warcraft Logs always leaves the hour's last 200 points for key reviews and parses, waiting for the hour below them (`reservePoints` in the import status).

### Fixed
- Starting WoW Live while it already runs opens the running copy's window instead of a second copy (with its own tray icon, overlay and log reading).
- *Quit* in the tray closes the main window and stops following the log, instead of the window's hide-to-tray cancelling its close. The window's X still hides it to the tray.

## [0.4.0] - 2026-11-12

### Changed
- *Find fights* lists the reports in the background: leaving Settings and coming back no longer loses it, a bar slides while it runs and *Stop* ends it. `POST /api/settings/warcraftlogs/import/plan` now answers 202 with the import's status at once, whose `plan` holds the result once its state leaves `listing`. What it found is saved (`plan.json` in the import's folder), so it's still there after a restart, until an import starts.
- Listing a character's reports caches each report's fights and players (one file a report, fetched again only when its end moves), so *Find fights* or an import after the 10-minute list cache spends points only on new or grown reports, not all 150 again.
- A key's meter cards leave out the Avoidable column: a key's avoidable damage is in its Avoidable tab at the bottom. Raid pulls keep the column.
- A raid pull's Mechanics card folds lines with the same name (a soak's hits, an add's spawns, the same cast going off) into one small card with their count; a click lists each time and who.
- Settings shows a sliding bar beside every action while it runs (saving, testing keys, *Find fights*, *Read old logs*, archiving, deleting, listing the logs), and an action left running when you leave Settings keeps going and shows again when you come back: busy, or the result it finished with while you were away (shown once).
- Deaths and damage spikes say *used Ice Cold* when the player pressed the defensive themselves, and keep *had* for one someone put on them (an external, a raid cooldown); an external the player put on someone else no longer counts as their own. `used` on a death and a spike member.
- Spirit Link Totem and the other `raid_cooldowns` (Zephyr too) count as raid cooldowns, not defensives: the Defensives tab marks them *(raid cooldown)*, and a death's or spike's "still had" no longer names them.
- Ula'tek's *Fix next pull* starts with the Blightscale Vipers that spawned: who spawned each (the egg carrier, with the cause and time) and how many nobody picked up, since a Viper all but wipes the raid (`spawner` on a spawn).
- Ula'tek's adds leave out Blightscale Spawn, the eggs the raid hits that never die: `not_adds` in `reference/raid-mechanics.toml` lists a boss's NPCs that aren't adds (`notAdds` in `raid-bosses.json`).
- A key's review leaves out the avoidable damage column (it's in the key's Avoidable tab); the advice still names who took the most. Reviews already made drop the column when shown.
- `readerVersion` 10, so *Read old logs* reads every log again.
- Build tooling: the Store release step uses `microsoft-store-apppublisher@v1.4` (Node 24, so no Node 20 warning), and the frontend lints with ESLint 10 and eslint-plugin-react-hooks 7 with its React Compiler rules on. The minutes since the last log in Settings and an import's "waiting N min" now update each minute instead of only when the page redraws.

### Fixed
- *Import from Warcraft Logs* failed a key Warcraft Logs names by its bosses and gives no zone ("no challenge map ID for "Adderis / Merektha / ... ""), as it does in a dungeon it hasn't mapped (Temple of Sethraliss): the dungeon is found by its instance ID, the end of the fight's encounter ID (61877 is instance 1877). The dungeon facts keep each dungeon's `instanceID` from Keystone.guru.

## [0.3.0] - 2026-11-12

### Added
- **Import from Warcraft Logs** in Settings: type a character (`Name-Realm`, checked against Warcraft Logs' realms; `character` in `/api/settings`, with `characterGuess` from the saved keys' logger) and press *Import*. The app lists the reports that character is in, then in the background fetches each Mythic+ key of the current season and each raid boss pull of the current raid not already saved, newest first, (a key or pull from your own logs wins, also when its log is read later) and saves it as if it had read the log: each fight's events are written back as a combat log (`<cache>\wcl-import\<code>-<fight>.zip`) and read by the same reader. Imported keys and pulls are tagged *Warcraft Logs* in the side panels with a link to the fight, count in estimates and baselines, and *Remove imported* deletes them. It keeps 200 of the hour's points for key reviews, waits for the hour when they run low, and goes on after a restart; *Stop* keeps what it fetched (`GET`/`POST`/`DELETE /api/settings/warcraftlogs/import`, `DELETE /api/settings/warcraftlogs/imported`). A key costs about 36 points, a raid pull about 14, listing about 2 per report. Converted fights match the local log on deaths, interrupts and must-stops, and on damage and healing within 0.3% for a key; some raid pets count differently (up to 1.8% of a raider's damage), since Warcraft Logs keeps no unit flags. `go run ./cmd/site --wcl-fight CODE#N` writes one fight's log for `--replay`. `readerVersion` 7, so *Read old logs* reads every log again.
- *Import from Warcraft Logs* shows what it would fetch before spending points: *Find fights* lists the character's M+ keys, raid kills and raid wipes with fights, hours and the points each would cost (about 0.27 points per player-minute, measured), and imports only the kinds ticked (raid wipes off at first, since they're most of the points). `POST /api/settings/warcraftlogs/import/plan` (a POST: listing spends points); `POST /api/settings/warcraftlogs/import` takes `{"kinds": [...]}`.
- An Augmentation Evoker's damage and healing count what its buffs add to allies, taken off the buffed players, as Warcraft Logs and Details credit it (the log's `*_DAMAGE_SUPPORT`, `*_HEAL_SUPPORT` and `SPELL_ABSORBED_SUPPORT` lines, and Warcraft Logs' support events in imported fights): on a top Augmentation key every player's damage is within 0.6% of Warcraft Logs'. `readerVersion` 8, so *Read old logs* reads every log again. Fights imported before this keep the Evoker's share counted twice (their saved copy wrote it as the Evoker's own damage): *Remove imported* and import them again to fix them, which spends their points again.

### Changed
- The side panels group saved keys and raid nights into dropdowns: one per day for the last 7 days ("Today", "Yesterday", "Mon, Nov 9"), then one per week ("Week of Nov 2"), each with its count. The newest group and the one holding the open key or pull start open.
- The timeline's marks show their lane, time and text in a box as soon as the mouse is on them, instead of the browser's slow tooltip.
- On a key, focusing a player cuts the timeline's Deaths, Defensives, Potions and Crowd control to that player's.

### Fixed
- *Import from Warcraft Logs* fetched fights you already had from your own log, and the same fight once per report it's in (two raiders uploading the same night): a local key or pull now skips every report's copy of it, and a fight in several reports is fetched once. On the owner's season that's 489 fights instead of 1,297.
- *Import from Warcraft Logs* failed fights whose events had an amount with a fraction ("cannot unmarshal number 75801.453125"): amounts, resources and stats are read as any number, rounded. Press *Import* again to retry the failed ones; their fetched pages are kept.
- *Import from Warcraft Logs* stopped with "The page argument must be a value between 1 and 5" for a character with more than 150 reports: Warcraft Logs serves only 5 pages of a character's recent reports, so the import now lists the newest 150 and goes on.

## [0.2.3] - 2026-11-11

### Removed
- The Raid tab's ready check between pulls (`ready` in `/api/raid/live`): it listed every raider as "not seen" for every raid buff. Each pull's **Missing buffs** card still shows who lacked a flask, food, Vantus Rune or raid buff at the pull and during it.

## [0.2.2] - 2026-11-11

### Fixed
- The Raid tab went black on a pull whose adds spawned from an enemy's cast (Ula'tek's Hatching Doom, Writhing Gestation): such a spawn saved its players as `null`, which the Mechanics card couldn't read. They're an empty list now, and the page reads pulls already saved that way.

## [0.2.1] - 2026-11-11

### Added
- The replays (M+ and raid) draw the other mobs too: a small unnamed red dot each, smaller than a player's, from when it acts until it dies (`mobs` in a track, sampled every 2 s; `readerVersion` 6 so *Read old logs* fills it in).
- The raid pull's **Missing buffs** card is one line per buff at the bottom of the page: who lacked it all pull ("everyone" when the whole raid did), then each shorter gap as a raider and its times.
- **Archive old combat logs** in Settings (*Your data*): the game's logs and the archived ones with date, size and the keys and pulls saved from each; logs older than 14 days and already read come ticked, the one the game is writing can't be picked. Archiving zips a log into the log folder's `WoW Live archive` folder (about a tenth of its size, read back and CRC-checked before the log is removed, any unread log read first) so live reading and *Read old logs* skip it; *Read everything again* and a `readerVersion` bump still read the zips. *Delete selected archives* removes zips for good after a confirm; saved keys and pulls stay (`GET /api/settings/logs`, `POST`/`DELETE /api/settings/logs/archive`).
- The **Avoidable** tab opens with a small card per player hit: what they took, from which ability, and how many hits.
- One set of meter cards for a finished key and a raid pull (`FightMeters.tsx`, shared by both pages). The cards follow the replay: for a key, the pull the replay is in and the whole key so far; for a raid, the whole pull so far. Kicks, dispels, avoidable and priority damage follow too, kept in the track's meters (`av`, `pd`, `k`, `ds`). Click a player and the cards drill in place: damage by spell over damage into each enemy, then healing over damage taken from, with avoidable abilities marked ⚠. The replay opens at the end, so the cards start with the whole fight, and Play from there starts over. The replay's own DPS/HPS cards, the raid's focus card beside the map and the M+ Player focus card next to the review are gone. *Read old logs* fills in the new fields (`readerVersion` 5).

### Changed
- Faster and lighter log reading, with every saved key and pull the same as before (checked by rebuilding them all from the owner's 13 logs). Reading a log streams it: the 1.4 GB log peaks at 55 MB of memory instead of about 400 MB and reads in 0.7 s instead of 1.8 s; the newest log on start is parsed on every core (2 s instead of 5 s); a line's time is read by hand (433 ns instead of 1 µs); the tracker marks a pull's seconds once per party instead of on every damage line and builds the flask and food list once (20 to 30% less tracker time, 40% fewer allocations). The replay finds positions and trails by binary search instead of scanning every point each frame.
- The key review table is more compact: one line per player, the spec and party rank in tooltips, "% of top" only where there is a top. The "big hits, defensive ready" column is gone (now the Big hits tab), and Healthstones shows only with a warlock in the group. Reviews saved earlier keep their columns.

### Fixed
- Reading the same logs again saves the same keys and pulls byte for byte: minibosses, must-stops, dispels, a key's boss health list, a pull's add spawns and lane marks no longer change order between reads (they followed Go's map order).
- *Read old logs* read the folder the app started with, not the one set in Settings since; two reads (the first-run read and the button) could run at once. One read, archive or delete runs at a time (409 for another).
- Security: downloads are capped at 64 MB; the page gets a Content-Security-Policy (own scripts, styles and API only, no framing), `nosniff` and `no-referrer`; the log folder must be a full path; Go 1.27.4 and `golang.org/x/net` 0.62 for the standard library and `x/net` advisories `govulncheck` reported (`npm audit`: none).

## [0.2.0] - 2026-11-11

### Added
- The M+ key page works like the raid pull page. One focused player and one clock: a click on a map dot, a graph line, a name, a Players row or the review's focus card focuses that player everywhere, and a time in the Deaths, Interrupts, Must-stops, Spikes and Bosses tabs moves the replay there (the raid pull's tabs too). A **Timeline** of lanes (pulls, bosses, deaths, Bloodlust, potions, defensives, spikes, must-stops, wasted kicks, crowd control; red where it went wrong) sits above the meters, and the focus card lists what happened to the player in order.
- The M+ replay keeps what the raid replay does: the boss's and minibosses' health over the map, the focused player's casts, damage taken, and ability and target bars up to the replay's moment (`casts`, `abilities`, `abilityTimes`, `targetSeries`, `health` in a key's track, in 10 s stretches). *Read old keys* fills them in for older keys.
- **Fix next key** above the review (at most `[suggestions] most` lines in `mplus.toml`): deaths with a defensive ready, the slowest pull against your best timed run at the level, must-stop casts that went off, Bloodlusts few took a potion with, damage spikes with no party cooldown, the longest wait and late minibosses; and your best run's time under the result (`suggestions`, `lanes`, `bestFinalMS`).
- Raid notes: a Vantus Rune is asked only in `[consumables] vantus_difficulties`, a party buff only when its class is in the group, a **Missing buffs** card replaces "Missing at the pull" (the start-of-pull check from 0:00 and each flask, food or raid buff lost for 5 s or more, "Arcane Intellect 2:00 until 2:35"; `buffGaps`), and Kith'ix light motes are expected of every living raider before Extinguish (`expect = 1304424` on the pickup rule): who missed one is red in Mechanics, a bad mark on a "Picked up a light mote" lane and a "Fix next pull" line.
- The key review counts **combat potions** over the key (a pre-pot included) instead of "potions at lust", and has health potion and healthstone counts.
- Review fixes to raid live: a raid pull's damage spike (lane and "No raid cooldown" suggestion) is the Spikes tab's worst burst, which a raid measures over `spike_seconds` (6 s) instead of an M+ key's 3 s, the six raid-wide externals (Anti-Magic Zone, Aura Mastery, Darkness, Power Word: Barrier, Rallying Cry, Spirit Link Totem) are one `raid_cooldowns` list in `mplus.toml` that M+ spike cover and the raid lanes both read, and a misspelt key in `mplus.toml` or `raid-rules.toml` fails at load, in the app too (`config.DecodeTOML`), with a test on the repo's own files. Saved pulls keep their old spike until *Read old logs* is run with "all".
- Raid mechanics that say who: each soak's casts with who soaked them against how many it wants, what raiders picked up (Ula'tek's eggs) wave by wave, who broke a tether and what it set off (Grasping Fangs → Blight Vein on 20), and each add's spawn with its cause (a Blightscale Viper from Noxious Splash on two egg carriers). The Mechanics card scrolls when long and Early deaths lists the first 5 (`[deaths] first`).
- Hand-kept boss mechanics for all ten bosses of The Venomous Abyss in `reference/raid-mechanics.toml` (an unknown key or boss name fails `raidref`), from Mythic Trap and Method, each spell ID checked against the owner's logs or a top Mythic kill's damage taken, debuffs and casts on Warcraft Logs; `cmd/raidref` merges it over what it sorts from Wowhead. Many of the generated IDs were a cast's where the app matches the hit, so they never matched (Sszorak's Tempest, The Twin Fangs' soaks, Throw Junk).
- Every M+ tab a raid pull shares under its meters (Deaths, Interrupts, Must-stops, Spikes, Defensives and potions, CC), and a **Raiders** table sortable by any column: DPS and damage, HPS and healing, damage taken and avoidable, active time, combat potions (pre-pots marked), health potions, healthstones and deaths. The M+ page has the same table as a *Players* tab.
- Flask, food, Vantus Rune and raid buffs at a raid pull's start (`[consumables] vantus` in `raid-rules.toml`). The overlay doesn't show it: the game logs no ready check.
- In the raid replay, the boss's health at the moment over the map's top right (the boss's own units, then the adds to burst; `health` in the track), and the focused raider's cast going and last casts at the bottom left. The focus card's damage, healing, damage taken and casts follow the replay's moment smoothly, a stretch in progress counting for the share of it that has passed (`casts`, `abilities` and `abilityTimes` in the track: about 1.07 MB for a 6-minute pull, against 514 KB before).
- Raid live: after each raid pull (any difficulty), a Raid tab and, between pulls, an overlay card with how far it got against tonight's best and the best ever, early deaths with their last hits and the defensives still ready, mass kills, mechanics by kind (avoidable hits, soaks, stacks, dispels, must-stop casts, adds, damage into the unit to burst), cooldowns and the meters (`/api/raid/live`, `/api/raid/nights`, `/api/raid/pulls/{start}`). Rules in `raid-rules.toml`; boss mechanics in `reference/raid-bosses.json` from `go run ./cmd/raidref` (Wowhead's cheat sheets, each boss's NPC IDs from a top Warcraft Logs kill, with `--check`). *Read old keys* is now *Read old logs* and saves raid pulls too.
- Raid replay: each raid pull's raiders and boss on the room's own map (the game's map art Warcraft Logs' replay draws, placed by the game's UiMapAssignment bounds from wago.tools, zoomed to where the raid fought), DPS and HPS graphs per raider or for the raid, a lane per mechanic (phases, deaths, Bloodlust, raid cooldowns, potions, the damage spike, avoidable hits, soaks, stacks, adds) and a "Fix next pull" list of suggestions from rules on the pull's facts against tonight's best pull (thresholds in `raid-rules.toml`'s `[suggestions]`), all on one clock (`/api/raid/pulls/{start}/track`, `/api/raid/maps/{uiMapID}`). `raidref` adds each boss's rooms and every boss Warcraft Logs lists for the raid (Nymrissa Wavecaller and Kith'ix have no Wowhead cheat sheet). The raid page shows class-coloured names, hides meter columns every raider has 0 in, and groups the night's pulls by boss. The M+ map draws a cross where a player died, and its damage graph can show the party's total or one player.
- Raid review, after the first night on it: one **focused raider** across the pull's page. A click on their dot on the replay's map, a meter row, their line on the graph or their name focuses them (a ring and their name on the map, a focus card beside it with what happened to them in order and their damage, healing, damage taken, casts and targets by ability, their line alone on the graph, and the pull's meters drilled into their abilities over their damage into each enemy and what they took damage from); a right click goes back to the whole raid. The replay's map zooms with the wheel, pans with a drag and has a *no trail* choice (the M+ map too). The line under the pointer on the graph comes to the front with the player's name and number. A click or drag anywhere on the mechanic lanes moves the replay, one line through every lane marks its moment over a time ruler (a tick every 15 to 60 s), and Bloodlust and raid cooldowns are stretches for as long as their buff was on a raider. A pull keeps each raider's abilities, damage taken by ability and damage into each enemy (`abilities`, `taken`, `targets`; *Read old logs* fills them in for older pulls).
- More "Fix next pull" rules, for pulls without reference mechanics or a better pull to compare with: several early deaths to one ability, slow dispels, no Bloodlust, raiders without a combat potion (one drunk before the pull counts, at 0:00 on the Potions lane), and raid cooldowns stacked (thresholds in `raid-rules.toml`'s `[suggestions]`).
- On the M+ map, a click on a player's dot or their row in the replay's meters follows them (a ring and their name, the others' trails faded), as on the raid replay; a right click shows the whole party again.

### Changed
- *Fix next key* is the key's own list: deaths to one ability, big hits with a defensive ready, the slowest pull against your best run (only when both had as many pulls; `slow_pull_seconds` joins `[suggestions]`), must-stops that went off, Bloodlusts few took a potion with, damage spikes with no party cooldown, the longest wait and late minibosses. The best-run line shows for timed and depleted keys only.
- A key's health box follows the boss being fought and the dungeon file's minibosses, and a running key's track leaves out casts and ability stretches until it ends. The review table's column groups come from the API (`group` on a metric).
- Player tables: fixed column widths, row separators, a faint rule between groups, class-coloured names, amber avoidable damage over the party's median, striping from 8 rows, and sideways scrolling with the name pinned on small screens (cards and the review table; deaths red, combat potions green).
- The replay's map moves smoothly: it draws every frame and units walk between their once-a-second points, while the rest of the page follows the clock 10 times a second (the M+ map too). On the raid pull's page the lanes and graph come first, then the map.
- Greater Invisibility isn't a defensive any more (`mplus.toml`), on both pages.
- *Read old logs* reads only logs that are new or changed since it last read them with the same reader (`learned.json` in the cache; `readerVersion` in `app/app.go`), and parses on every core: 75 ms with nothing new, 9 s for all ten logs (3.2 GB, was 17 s). *Read everything again* reads them all (`?all=true`); both say how many logs they read.
- Health potions show as "Health potion", not their full name (M+ and raid).
- "Missing at the pull" names who lacks their 2-set or 4-set, from the season's tier sets `raidref` reads off Wowhead (`[tier] sets` in `raid-rules.toml`, `tier` in `reference/raid-bosses.json`).
- Raid replay: the play bar sits between the graph and the map; the focused raider's cast box drops "s ago" and a box beside it lists what they took damage from up to the moment; their bars and targets follow the replay from 0:00 (never the whole pull) in 2 s stretches (`targetSeries` in the track; about 1.6 MB for a 6-minute pull). *Read everything again* fills older pulls in.
- The raid pull's page leads with what went wrong: Fix next pull, then Early deaths and Mechanics, then the replay and the meters (now titled *Damage done* and *Healing and damage taken*). Mechanics is grouped by kind, each line with when it happened and raiders in class colours; a time in either card moves the replay there. The side list shows a night's pulls newest first with when each started (`/api/raid/nights` lists them that way).
- The overlay's raid card shows the result, the boss's health as a bar with tonight's best pull marked, the first three deaths, the top three things to fix and the top DPS and HPS as class-coloured rows.
- A raid death's "still ready" list knows the defensives a raider pressed in the night's earlier pulls, not just this one, so "early deaths with a defensive ready" has something to say.

### Fixed
- During a raid pull the page and overlay re-read the raid only when the pull starts or ends, not on every log line. A raid pull's tether breaks right after each other and the stacks past the limit now come from the API (`then`, `over`). The swagger text for the raid track and `KeyTrack` matches the code, and *Read old logs* reads keys again (reader version 4) for the miniboss health and Fix next key rules.
- A Kith'ix mote wave picked up over 4 to 5 s is one wave, not several.
- Review fixes to raid night two: one rule for a spike a raid cooldown covers (cast by its end, buff still up at its start, or cast within the death window before it when no end was seen), a Viper whose egg carrier isn't found now says *Not picked up* with no player instead of naming whoever the cause landed on, `?all=` and `?force=` must be booleans (400 otherwise), and `raidref` fails rather than writing a file without the tier sets when Wowhead can't be read. Saved pulls keep their old Viper causes and spike cover until *Read old logs* is run with "all".
- A raid boss's health is its own: Ula'tek's counted its two Gore Rattle tails and the Venomous Heart, so a wipe at 48% showed 66%. The units named as the encounter count (or the mechanics file's `health` NPCs; all of a council). Pulls already saved keep their number until *Read old logs*. The tails and heart no longer show as adds.
- A combat potion isn't a defensive (M+ and raid): one drunk just before a death no longer counts as its cover, nor shows in a death's "still ready" list.
- Raid adds that die with no `UNIT_DIED` (Nymrissa's Bubblefin Shorerunners) count as killed when their health reaches 0, instead of "still up at the end"; an encounter nobody fought (a reset on the way in) isn't a pull.
- Vantus Runes count: their auras (Vantus Rune: Tides 1303171, Vantus Rune: Ula'tek 1303187) are never named in the log, so every raider showed it missing. Matched by ID (`[consumables] vantus_ids` in `raid-rules.toml`).
- Pre-pots count when the potion's buff is on the raider at the pull's start, and Liquid Luster is a combat potion (`combat_potion_ids` in `mplus.toml`, which also names a potion whose name the log never showed). M+ too.
- Each Ula'tek Viper names the raider whose egg (Noxious Shell) broke as Caustic Waves or Noxious Splash landed on them (`lost` in a spawn rule), one raider an add, instead of everyone the wave was still ticking on. Spawn causes count landings, not ticks.
- Raid spikes count the raid cooldowns whose buff was up (not M+ externals) and say "no raid cooldown" when none was. Ascendance, Celestial Conduit (healers only, `healer_only`) and Zephyr are raid cooldowns.
- Switching between saved M+ keys no longer flashes the last key's graphs.

## [0.1.7] - 2026-11-11

### Added
- Bottom tabs on both pages:
  - **Big hits**: damage spikes whose hardest-hit player had a defensive ready and used none (`unused` on a spike).
  - **Avoidable**: each avoidable ability with its damage, hits and who it hit (`avoidableHits`, now on raid pulls too). This replaces the raid Mechanics card's "Avoidable hits".
  - A tab with the focused player's moments, which replaces the M+ *Players* tab.
- The M+ graph has the raid graph's hover, one-player focus and party total, and a click on it moves the replay there (the replay's clock now lives on the key's page, shared by the map, the graph and the boss list).
- A click on a boss in the bottom tabs moves the replay to its pull and opens the fight: its length against your best timed run at the level (`bestBosses` on the key view) and the top runs (`topBosses` on the review, from the same Warcraft Logs run query as the party's damage taken), the deaths and must-stops that went off in it, and each player's DPS and HPS on it.
- More in the key review. Each player gets up to four things to work on (was three), now also: dying to the same ability more than once, DPS or HPS under 85% of their usual in the dungeon, a Bloodlust without a combat potion, starting without a flask or food, the group's lowest active time under 85%, and no crowd control while must-stop casts went off. The party's lines add what its deaths cost on the clock, Bloodlusts against how many the key had room for, minibosses that died after most of their pack, dispellable debuffs left to run out, and overlapped party cooldowns. Reviews already made keep their old advice.
- A `Makefile` for the everyday commands: `make app`, `make check` (what CI runs), `make site`, `make swag`, `make ref`; `make` lists them.

### Changed
- The key review shows every player's DPS and HPS against the top runs of their spec, not only their role's one: two Warcraft Logs rankings asks per player instead of one (a review measured 19 points). Reviews already made keep their single comparison.
- The forces bar (page and overlay) compares with your best timed run of the dungeon at that level: "12% faster than your best +15", with a tick where that run stood. It was the median of all your earlier runs at the same time, timed or not, and read like "45% ahead". `KeyForces.referencePct` is replaced by `pace`.
- The bottom tabs open on Minibosses.
- Settings says how to make the Warcraft Logs client: link Battle.net first, redirect URL `http://localhost`, *Public Client* unchecked, and copy the ID and secret before closing the page.
- `make ref` checks each dungeon's forces total against Keystone.guru's dungeon data instead of Raider.IO's top runs, which took 5 to 45 s a dungeon and failed now and then with a 500. Keystone.guru's totals match all eight of the season's; its forces per mob lag (Temple of Sethraliss), so they still come from Mythic Dungeon Tools.
- Crowd control counts knocks up and back too: Supernova, Gravity Lapse, Blast Wave, Bursting Shot, High Explosive Trap, Implosive Trap, Shining Force and Sundering, and Earthgrab Totem's root (`[cc]` in `mplus.toml`).
- The Settings page's fields and buttons look like fields and buttons (boxed, *Save* filled, *Remove* red), and its sections sit side by side where there's room.

### Fixed
- Den of Nalorakk's second floor (uiMap 2513) had no map: Keystone.guru has no tiles under its index and draws it as index 3 under another map ID. A floor without tiles now takes those of a floor with the same bounds (which then goes), and one left without any is drawn from where parties walked (`make ref`, which prints each floor it changes). So 2513 now draws on index 3's tiles, and the small entrance floor (2564), which index 3's copied bounds put off the map, shows where parties walked.
- Kings' Rest counted 587 of 608 forces in a finished +17 and said the dungeon file needed checking: its Half-Finished Mummies come under two NPC IDs and Mythic Dungeon Tools lists one. The other (273050) now counts the same (`npcTwins` in `service/mplusref.go`). *Read old logs* corrects a key saved before this.

## [0.1.6] - 2026-11-10

### Added
- A **player focus** card beside the review's advice: pick a party member to see their damage and healing by ability with casts and hits, what they took damage from, and what they cast, as bars in each ability's spell school colour (fire orange, frost blue, a blend for a spell of several). It opens on your own character: the key keeps whose game wrote the log (`logger`) and each ability's `school`. Keys saved before this have no abilities; *Read old logs* in Settings fills them in from the logs still in the game's folder.
- An abandoned key is reviewed like any other, against whole top runs over the time it ran, and shows the player focus card.
- *End as abandoned* at the top right of the live page while a key runs (`POST /api/mplus/live/abandon`), with how long ago the game last wrote its log; a key ended by it, or by the silence, runs again if the log goes on in it for 2 more minutes, and a timed end in that time still counts (Nov 10: the Den of Nalorakk +17's zone change out reached the log 5 minutes late). The game logs no end for a key the party left and writes its log in batches as fights start and end (measured Nov 10: 2:45 of a boss fight in one write), so the line that says they left can come minutes later.

### Changed
- A running key the log goes silent on is closed as abandoned after 10 minutes, saved and reviewed, instead of the page going back to "Waiting for the next key" after 3 (which a long boss fight also did, hiding the overlay). The key view's `quiet` field is gone; `silentMS` says how long the log has been silent.
- The live page and the README say when the ready check can show: the game often writes the lines from zoning in only as the key starts.
- The release workflow's `store` job submits to the Microsoft Store without waiting for approval: the `microsoft-store` environment's required reviewer went away with the repo going private.

### Fixed
- An abandoned key ends at its last line in the log, not at the line that showed it was over: one whose log just stopped read "Abandoned at 18:42:06" with 3k DPS once the next day's log started.
- The ready check keeps the buffs it has seen when the party zones into the same dungeon again, instead of going back to "not seen" for everyone.

## [0.1.5] - 2026-11-10

### Added
- A privacy policy for the Microsoft Store listing, on [steezy2/wow-live](https://github.com/steezy2/wow-live/blob/master/PRIVACY.md): what the app keeps on the PC and the public data it downloads.
- The release workflow also packs an MSIX for the Microsoft Store (`backend/cmd/app/msix`), with Start menu and Store logos rendered from the icon, once the repo's `MSIX_*` variables hold the app's Partner Center identity.
- A `v*` tag submits that MSIX to the Microsoft Store with the Store CLI (`msstore`, a `store` job in the release workflow), once the repo's `STORE_PRODUCT_ID` variable and the Entra secrets (`AZURE_AD_TENANT_ID`, `AZURE_AD_APPLICATION_CLIENT_ID`, `AZURE_AD_APPLICATION_SECRET`, `SELLER_ID`) are set; it waits for approval in the `microsoft-store` environment.

### Changed
- The Microsoft Store is the only install: a tag packs just the MSIX, now required, and keeps it on this repo's release. The privacy policy moved to [steezy2/wow-live](https://github.com/steezy2/wow-live), the app's public page, which the User-Agent now names, ahead of this repo going private. The code is now proprietary (all rights reserved) instead of MIT.
- CI and the release workflow use the GitHub actions' Node 24 versions (checkout, setup-go and setup-node v7, upload-artifact v7, download-artifact v8), ahead of Node 20's removal from the runners.

### Removed
- The NSIS installer, its SHA-256, the build provenance attestation and the copy of each release on steezy2/wow-live (`RELEASES_TOKEN`).

## [0.1.4] - 2026-11-09

### Added
- Screenshots in the README (a key mid-run, the review and the graph), from a replay of a real key with made-up names.
- A tag also releases the installer on steezy2/wow-live, the installers-only repo the first testers have (`RELEASES_TOKEN`).

### Changed
- Docs and code comments brought in line after the split: no links to the specs left in wow-raiding or mentions of its Python tools, attestation described as live, the User-Agent naming this repo, and v0.1.3 given its own section here.
- The key review shows each player's spec as one label with spaces ("Blood Death Knight", not "Blood DeathKnight"), in the table and the advice; the review's `class` field is gone and `spec` holds the label.

### Fixed
- The damage graph's last time label is no longer cut off at the right edge.
- Enemy forces count a kill the game logs only as `PARTY_KILL`, with no `UNIT_DIED`: a timed Blinding Vale +13 read 654/686 and asked for the dungeon file to be checked, when the file was right.

## [0.1.3] - 2026-11-08

### Added
- WoW Live in its own repo, split out of the private wow-raiding at its v0.1.3: the Go backend, the page, the installer and their CI, with the M+ rules in `mplus.toml`, an MIT licence, and releases with build provenance.
