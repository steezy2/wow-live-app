# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- Raid live: after each raid pull (any difficulty), a Raid tab and, between pulls, an overlay card with how far it got against tonight's best and the best ever, early deaths with their last hits and the defensives still ready, mass kills, mechanics by kind (avoidable hits, soaks, stacks, dispels, must-stop casts, adds, damage into the unit to burst), cooldowns and the meters (`/api/raid/live`, `/api/raid/nights`, `/api/raid/pulls/{start}`). Rules in `raid-rules.toml`; boss mechanics in `reference/raid-bosses.json` from `go run ./cmd/raidref` (Wowhead's cheat sheets, each boss's NPC IDs from a top Warcraft Logs kill, with `--check`). *Read old keys* is *Read old logs* and saves raid pulls too. Not released yet.
- Raid replay: each raid pull's raiders and boss on the room's own map (the game's map art Warcraft Logs' replay draws, placed by the game's UiMapAssignment bounds from wago.tools, zoomed to where the raid fought), DPS and HPS graphs per raider or for the raid, a lane per mechanic (phases, deaths, Bloodlust, raid cooldowns, potions, the damage spike, avoidable hits, soaks, stacks, adds) and a "Fix next pull" list of suggestions from rules on the pull's facts against tonight's best pull (thresholds in `raid-rules.toml`'s `[suggestions]`), all on one clock (`/api/raid/pulls/{start}/track`, `/api/raid/maps/{uiMapID}`). `raidref` adds each boss's rooms and every boss Warcraft Logs lists for the raid (Nymrissa Wavecaller and Kith'ix have no Wowhead cheat sheet). The raid page shows class-coloured names, hides meter columns every raider has 0 in, and groups the night's pulls by boss. The M+ map draws a cross where a player died, and its damage graph can show the party's total or one player.

### Changed
- The release workflow's `store` job submits to the Microsoft Store without waiting for approval: the `microsoft-store` environment's required reviewer went away with the repo going private.

### Fixed
- Raid adds that die with no `UNIT_DIED` (Nymrissa's Bubblefin Shorerunners) count as killed when their health reaches 0, instead of "still up at the end"; an encounter nobody fought (a reset on the way in) isn't a pull.

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
