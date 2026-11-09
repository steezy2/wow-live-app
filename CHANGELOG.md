# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- Raid live: after each raid pull (any difficulty), a Raid tab and, between pulls, an overlay card with how far it got against tonight's best and the best ever, early deaths with their last hits and the defensives still ready, mass kills, mechanics by kind (avoidable hits, soaks, stacks, dispels, must-stop casts, adds, damage into the unit to burst), cooldowns and the meters (`/api/raid/live`, `/api/raid/nights`, `/api/raid/pulls/{start}`). Rules in `raid-rules.toml`; boss mechanics in `reference/raid-bosses.json` from `go run ./cmd/raidref` (Wowhead's cheat sheets, with `--check`). *Read old keys* is *Read old logs* and saves raid pulls too. Not released yet.

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
