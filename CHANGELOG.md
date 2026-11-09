# Changelog

What changed, newest first. Each PR adds its lines under `[Unreleased]`.

## [Unreleased]

### Added
- A privacy policy (`PRIVACY.md`), for the Microsoft Store listing: what the app keeps on the PC and the public data it downloads.
- The release workflow also packs an MSIX for the Microsoft Store (`backend/cmd/app/msix`), with Start menu and Store logos rendered from the icon, once the repo's `MSIX_*` variables hold the app's Partner Center identity.

### Changed
- CI and the release workflow use the GitHub actions' Node 24 versions (checkout, setup-go and setup-node v7, upload-artifact v7, download-artifact v8, attest-build-provenance v4), ahead of Node 20's removal from the runners.

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
