# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- A **player focus** card beside the review's advice: pick a party member to see their damage and healing by ability with casts and hits, what they took damage from, and what they cast, as bars. Keys saved before this have no abilities; *Read old keys* in Settings fills them in from the logs still in the game's folder.
- An abandoned key is reviewed like any other, against whole top runs over the time it ran, and shows the player focus card.
- *Party left? End the key* on the live page once the game's log has been silent for a minute (`POST /api/mplus/live/abandon`), with how long ago the game last wrote its log. The game logs no end for a key the party left and writes its log in batches as fights start and end (measured Nov 10: 2:45 of a boss fight in one write), so the line that says they left can come minutes later.

### Changed
- A running key the log goes silent on is closed as abandoned after 10 minutes, saved and reviewed, instead of the page going back to "Waiting for the next key" after 3 (which a long boss fight also did, hiding the overlay); if the log then goes on in the key, it runs again. The key view's `quiet` field is gone; `silentMS` says how long the log has been silent.
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
