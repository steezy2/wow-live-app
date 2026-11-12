# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- *Import from Warcraft Logs* shows what it would fetch before spending points: *Find fights* lists the character's M+ keys, raid kills and raid wipes with fights, hours and the points each would cost (about 0.27 points per player-minute, measured), and imports only the kinds ticked (raid wipes off at first, since they're most of the points). `GET /api/settings/warcraftlogs/import/plan`; `POST /api/settings/warcraftlogs/import` takes `{"kinds": [...]}`.
- An Augmentation Evoker's damage counts what its buffs add to allies, taken off the buffed players, as Warcraft Logs and Details credit it (the log's `*_DAMAGE_SUPPORT` lines, and Warcraft Logs' support events in imported fights): on a top Augmentation key every player's damage is within 0.6% of Warcraft Logs'. `readerVersion` 8, so *Read old logs* reads every log and imported fight again.
- **Import from Warcraft Logs** in Settings: type a character (`Name-Realm`, checked against Warcraft Logs' realms; `character` in `/api/settings`, with `characterGuess` from the saved keys' logger) and press *Import*. The app lists the reports that character is in, then in the background fetches each Mythic+ key of the current season and each raid boss pull of the current raid not already saved, newest first, (a key or pull from your own logs wins, also when its log is read later) and saves it as if it had read the log: each fight's events are written back as a combat log (`<cache>\wcl-import\<code>-<fight>.zip`) and read by the same reader. Imported keys and pulls are tagged *Warcraft Logs* in the side panels with a link to the fight, count in estimates and baselines, and *Remove imported* deletes them. It keeps 200 of the hour's points for key reviews, waits for the hour when they run low, and goes on after a restart; *Stop* keeps what it fetched (`GET`/`POST`/`DELETE /api/settings/warcraftlogs/import`, `DELETE /api/settings/warcraftlogs/imported`). A key costs about 36 points, a raid pull about 14, listing about 2 per report. Converted fights match the local log on deaths, interrupts and must-stops, and on damage and healing within 0.3% for a key; some raid pets count differently (up to 1.8% of a raider's damage), since Warcraft Logs keeps no unit flags. `go run ./cmd/site --wcl-fight CODE#N` writes one fight's log for `--replay`. `readerVersion` 7, so *Read old logs* reads every log again.

### Changed
- The side panels group saved keys and raid nights into dropdowns: one per day for the last 7 days ("Today", "Yesterday", "Mon, Nov 9"), then one per week ("Week of Nov 2"), each with its count. The newest group and the one holding the open key or pull start open.

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
- Faster and lighter log reading, with every saved key and pull the same as before (checked by rebuilding them all from the owner's 13 logs). Reading a log streams it: the 1.4 GB log peaks at 55 MB of memory instead of about 400 MB and reads in 0.7 s instead of 1.8 s; the newest log on start is parsed on every core (2 s instead of 5 s); a line's time is read by hand (433 ns instead of 1 �s); the tracker marks a pull's seconds once per party instead of on every damage line and builds the flask and food list once (20 to 30% less tracker time, 40% fewer allocations). The replay finds positions and trails by binary search instead of scanning every point each frame.
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
