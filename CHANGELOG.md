# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- Raid mechanics that say who: each soak's casts with who soaked them against how many it wants, what raiders picked up (Ula'tek's eggs) wave by wave, who broke a tether and what it set off (Grasping Fangs → Blight Vein on 20), and each add's spawn with its cause (a Blightscale Viper from Noxious Splash on two egg carriers). The Mechanics card scrolls when long and Early deaths lists the first 5 (`[deaths] first`).
- Hand-kept boss mechanics for all ten bosses of The Venomous Abyss in `reference/raid-mechanics.toml` (an unknown key or boss name fails `raidref`), from Mythic Trap and Method, each spell ID checked against the owner's logs or a top Mythic kill's damage taken, debuffs and casts on Warcraft Logs; `cmd/raidref` merges it over what it sorts from Wowhead. Many of the generated IDs were a cast's where the app matches the hit, so they never matched (Sszorak's Tempest, The Twin Fangs' soaks, Throw Junk).
- Every M+ tab a raid pull shares under its meters (Deaths, Interrupts, Must-stops, Spikes, Defensives and potions, CC), and a **Raiders** table sortable by any column: DPS and damage, HPS and healing, damage taken and avoidable, active time, combat potions (pre-pots marked), health potions, healthstones and deaths. The M+ page has the same table as a *Players* tab.
- A ready check on the Raid tab between pulls (flask, food, Vantus Rune and raid buffs, `[consumables] vantus` in `raid-rules.toml`; `ready` in `/api/raid/live`), seeded from each pull's start less the raid buffs of those who died, and "Missing at the pull" under the pull's title. The overlay doesn't show it: the game logs no ready check.
- In the raid replay, the boss's health at the moment over the map's top right (the boss's own units, then the adds to burst; `health` in the track), and the focused raider's cast going and last casts at the bottom left. The focus card's damage, healing and casts bars follow the replay's moment (`casts` and `abilities` in the track, about 1 MB for a 6-minute pull, twice what it was).
- Raid live: after each raid pull (any difficulty), a Raid tab and, between pulls, an overlay card with how far it got against tonight's best and the best ever, early deaths with their last hits and the defensives still ready, mass kills, mechanics by kind (avoidable hits, soaks, stacks, dispels, must-stop casts, adds, damage into the unit to burst), cooldowns and the meters (`/api/raid/live`, `/api/raid/nights`, `/api/raid/pulls/{start}`). Rules in `raid-rules.toml`; boss mechanics in `reference/raid-bosses.json` from `go run ./cmd/raidref` (Wowhead's cheat sheets, each boss's NPC IDs from a top Warcraft Logs kill, with `--check`). *Read old keys* is *Read old logs* and saves raid pulls too. Not released yet.
- Raid replay: each raid pull's raiders and boss on the room's own map (the game's map art Warcraft Logs' replay draws, placed by the game's UiMapAssignment bounds from wago.tools, zoomed to where the raid fought), DPS and HPS graphs per raider or for the raid, a lane per mechanic (phases, deaths, Bloodlust, raid cooldowns, potions, the damage spike, avoidable hits, soaks, stacks, adds) and a "Fix next pull" list of suggestions from rules on the pull's facts against tonight's best pull (thresholds in `raid-rules.toml`'s `[suggestions]`), all on one clock (`/api/raid/pulls/{start}/track`, `/api/raid/maps/{uiMapID}`). `raidref` adds each boss's rooms and every boss Warcraft Logs lists for the raid (Nymrissa Wavecaller and Kith'ix have no Wowhead cheat sheet). The raid page shows class-coloured names, hides meter columns every raider has 0 in, and groups the night's pulls by boss. The M+ map draws a cross where a player died, and its damage graph can show the party's total or one player.
- Raid review, after the first night on it: one **focused raider** across the pull's page. A click on their dot on the replay's map, a meter row, their line on the graph or their name focuses them (a ring and their name on the map, a focus card beside it with what happened to them in order and their damage, healing, damage taken, casts and targets by ability, their line alone on the graph, and the pull's meters drilled into their abilities over their damage into each enemy and what they took damage from); a right click goes back to the whole raid. The replay's map zooms with the wheel, pans with a drag and has a *no trail* choice (the M+ map too). The line under the pointer on the graph comes to the front with the player's name and number. A click or drag anywhere on the mechanic lanes moves the replay, one line through every lane marks its moment over a time ruler (a tick every 15 to 60 s), and Bloodlust and raid cooldowns are stretches for as long as their buff was on a raider. A pull keeps each raider's abilities, damage taken by ability and damage into each enemy (`abilities`, `taken`, `targets`; *Read old logs* fills them in for older pulls).
- More "Fix next pull" rules, for pulls without reference mechanics or a better pull to compare with: several early deaths to one ability, slow dispels, no Bloodlust, raiders without a combat potion (one drunk before the pull counts, at 0:00 on the Potions lane), and raid cooldowns stacked (thresholds in `raid-rules.toml`'s `[suggestions]`).
- A **player focus** card beside the review's advice: pick a party member to see their damage and healing by ability with casts and hits, what they took damage from, and what they cast, as bars in each ability's spell school colour (fire orange, frost blue, a blend for a spell of several). It opens on your own character: the key keeps whose game wrote the log (`logger`) and each ability's `school`. Keys saved before this have no abilities; *Read old keys* in Settings fills them in from the logs still in the game's folder.
- More in the key review. Each player gets up to four things to work on (was three), now also: dying to the same ability more than once, DPS or HPS under 85% of their usual in the dungeon, a Bloodlust without a combat potion, starting without a flask or food, the group's lowest active time under 85%, and no crowd control while must-stop casts went off. The party's lines add what its deaths cost on the clock, Bloodlusts against how many the key had room for, minibosses that died after most of their pack, dispellable debuffs left to run out, and overlapped party cooldowns. Reviews already made keep their old advice.
- A `Makefile` for the everyday commands: `make app`, `make check` (what CI runs), `make site`, `make swag`, `make ref`; `make` lists them.
- An abandoned key is reviewed like any other, against whole top runs over the time it ran, and shows the player focus card.
- *Party left? End the key* on the live page once the game's log has been silent for a minute (`POST /api/mplus/live/abandon`), with how long ago the game last wrote its log. The game logs no end for a key the party left and writes its log in batches as fights start and end (measured Nov 10: 2:45 of a boss fight in one write), so the line that says they left can come minutes later.

### Changed
- The replay's map moves smoothly: the clock runs every frame and units walk between their once-a-second points (the M+ map too). On the raid pull's page the lanes and graph come first, then the map.
- Greater Invisibility isn't a defensive any more (`mplus.toml`), on both pages.
- The raid pull's page leads with what went wrong: Fix next pull, then Early deaths and Mechanics, then the replay and the meters (now titled *Damage done* and *Healing and damage taken*). Mechanics is grouped by kind, each line with when it happened and raiders in class colours; a time in either card moves the replay there. The side list shows a night's pulls newest first with when each started (`/api/raid/nights` lists them that way).
- The overlay's raid card shows the result, the boss's health as a bar with tonight's best pull marked, the first three deaths, the top three things to fix and the top DPS and HPS as class-coloured rows.
- A raid death's "still ready" list knows the defensives a raider pressed in the night's earlier pulls, not just this one, so "early deaths with a defensive ready" has something to say.
- `make ref` checks each dungeon's forces total against Keystone.guru's dungeon data instead of Raider.IO's top runs, which took 5 to 45 s a dungeon and failed now and then with a 500. Keystone.guru's totals match all eight of the season's; its forces per mob lag (Temple of Sethraliss), so they still come from Mythic Dungeon Tools.
- Crowd control counts knocks up and back too: Supernova, Gravity Lapse, Blast Wave, Bursting Shot, High Explosive Trap, Implosive Trap, Shining Force and Sundering, and Earthgrab Totem's root (`[cc]` in `mplus.toml`).
- The Settings page's fields and buttons look like fields and buttons (boxed, *Save* filled, *Remove* red), and its sections sit side by side where there's room.
- A running key the log goes silent on is closed as abandoned after 10 minutes, saved and reviewed, instead of the page going back to "Waiting for the next key" after 3 (which a long boss fight also did, hiding the overlay); if the log then goes on in the key, it runs again. The key view's `quiet` field is gone; `silentMS` says how long the log has been silent.
- The live page and the README say when the ready check can show: the game often writes the lines from zoning in only as the key starts.
- The release workflow's `store` job submits to the Microsoft Store without waiting for approval: the `microsoft-store` environment's required reviewer went away with the repo going private.

### Fixed
- A raid boss's health is its own: Ula'tek's counted its two Gore Rattle tails and the Venomous Heart, so a wipe at 48% showed 66%. The units named as the encounter count (or the mechanics file's `health` NPCs; all of a council). Pulls already saved keep their number until *Read old logs*. The tails and heart no longer show as adds.
- A combat potion isn't a defensive (M+ and raid): one drunk just before a death no longer counts as its cover, nor shows in a death's "still ready" list.
- Raid adds that die with no `UNIT_DIED` (Nymrissa's Bubblefin Shorerunners) count as killed when their health reaches 0, instead of "still up at the end"; an encounter nobody fought (a reset on the way in) isn't a pull.
- Kings' Rest counted 587 of 608 forces in a finished +17 and said the dungeon file needed checking: its Half-Finished Mummies come under two NPC IDs and Mythic Dungeon Tools lists one. The other (273050) now counts the same (`npcTwins` in `service/mplusref.go`). *Read old keys* corrects a key saved before this.
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
