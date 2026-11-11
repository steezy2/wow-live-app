# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- The M+ graph has the raid graph's hover, one-player focus and party total, and a click on it moves the replay there (the replay's clock now lives on the key's page, shared by the map, the graph and the boss list).
- A click on a boss in the bottom tabs moves the replay to its pull and opens the fight: its length against your best timed run at the level (`bestBosses` on the key view) and the top runs (`topBosses` on the review, from the same Warcraft Logs run query as the party's damage taken), the deaths and must-stops that went off in it, and each player's DPS and HPS on it.
- A **player focus** card beside the review's advice: pick a party member to see their damage and healing by ability with casts and hits, what they took damage from, and what they cast, as bars in each ability's spell school colour (fire orange, frost blue, a blend for a spell of several). It opens on your own character: the key keeps whose game wrote the log (`logger`) and each ability's `school`. Keys saved before this have no abilities; *Read old keys* in Settings fills them in from the logs still in the game's folder.
- More in the key review. Each player gets up to four things to work on (was three), now also: dying to the same ability more than once, DPS or HPS under 85% of their usual in the dungeon, a Bloodlust without a combat potion, starting without a flask or food, the group's lowest active time under 85%, and no crowd control while must-stop casts went off. The party's lines add what its deaths cost on the clock, Bloodlusts against how many the key had room for, minibosses that died after most of their pack, dispellable debuffs left to run out, and overlapped party cooldowns. Reviews already made keep their old advice.
- A `Makefile` for the everyday commands: `make app`, `make check` (what CI runs), `make site`, `make swag`, `make ref`; `make` lists them.
- An abandoned key is reviewed like any other, against whole top runs over the time it ran, and shows the player focus card.
- *Party left? End the key* on the live page once the game's log has been silent for a minute (`POST /api/mplus/live/abandon`), with how long ago the game last wrote its log. The game logs no end for a key the party left and writes its log in batches as fights start and end (measured Nov 10: 2:45 of a boss fight in one write), so the line that says they left can come minutes later.

### Changed
- The key review shows every player's DPS and HPS against the top runs of their spec, not only their role's one: two Warcraft Logs rankings asks per player instead of one (a review measured 19 points). Reviews already made keep their single comparison.
- The forces bar (page and overlay) compares with your best timed run of the dungeon at that level: "12% faster than your best +15", with a tick where that run stood. It was the median of all your earlier runs at the same time, timed or not, and read like "45% ahead". `KeyForces.referencePct` is replaced by `pace`.
- The bottom tabs open on Minibosses.
- Settings says how to make the Warcraft Logs client: link Battle.net first, redirect URL `http://localhost`, *Public Client* unchecked, and copy the ID and secret before closing the page.
- `make ref` checks each dungeon's forces total against Keystone.guru's dungeon data instead of Raider.IO's top runs, which took 5 to 45 s a dungeon and failed now and then with a 500. Keystone.guru's totals match all eight of the season's; its forces per mob lag (Temple of Sethraliss), so they still come from Mythic Dungeon Tools.
- Crowd control counts knocks up and back too: Supernova, Gravity Lapse, Blast Wave, Bursting Shot, High Explosive Trap, Implosive Trap, Shining Force and Sundering, and Earthgrab Totem's root (`[cc]` in `mplus.toml`).
- The Settings page's fields and buttons look like fields and buttons (boxed, *Save* filled, *Remove* red), and its sections sit side by side where there's room.
- A running key the log goes silent on is closed as abandoned after 10 minutes, saved and reviewed, instead of the page going back to "Waiting for the next key" after 3 (which a long boss fight also did, hiding the overlay); if the log then goes on in the key, it runs again. The key view's `quiet` field is gone; `silentMS` says how long the log has been silent.
- The live page and the README say when the ready check can show: the game often writes the lines from zoning in only as the key starts.
- The release workflow's `store` job submits to the Microsoft Store without waiting for approval: the `microsoft-store` environment's required reviewer went away with the repo going private.

### Fixed
- Den of Nalorakk's second floor (uiMap 2513) had no map: Keystone.guru has no tiles under its index and draws it as index 3 under another map ID. A floor without tiles now takes those of a floor with the same bounds (which then goes), and one left without any is drawn from where parties walked (`make ref`, which prints each floor it changes). So 2513 now draws on index 3's tiles, and the small entrance floor (2564), which index 3's copied bounds put off the map, shows where parties walked.
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
