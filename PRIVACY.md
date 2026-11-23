# WoW Live privacy policy

Last updated: November 23, 2026

WoW Live runs on your PC and has no accounts, analytics, ads or tracking. The developer receives no data from it.

## What it reads and keeps on your PC

- **World of Warcraft's combat log**, from the game's Logs folder. It holds the character names of you and the players in your group. WoW Live reads it on your PC and never uploads it.
- **What it works out from the log** (your saved keys and raid pulls, deaths and meters), its settings and its own log file, in the app's data folder on your PC. Uninstalling the app deletes them.
- **Your Warcraft Logs API client ID and secret**, if you enter them, in Windows Credential Manager. They are sent only to Warcraft Logs, to sign in to its API.

## What it requests over the internet

WoW Live downloads public game data. None of these requests include anything from your combat log, and only Warcraft Logs gets a character name (below).

- **Warcraft Logs** (warcraftlogs.com), when you've entered API keys: top runs for a dungeon, key level and spec to compare against, and the name and realm of a character, to list its public reports and download their fights (when you import or look up your own runs) or to read its parses (when you run a player review, for the player you picked, who can be anyone in your group). See Warcraft Logs' own privacy policy.
- **Raider.IO** (raider.io): the season's dungeon timers and this week's affixes, for your region.
- **Keystone.guru** (assets.keystone.guru): dungeon map images.
- **Wowhead** (nether.wowhead.com and wow.zamimg.com): the text of a spell when you hover its name, the icons of talents and spells, and the names, icons and quality of the gear in a character you paste in the Sim tab (only the item IDs are sent, never the character's name or realm).
- **github.com** (this repository's `simc-engine` release), only when you press the Sim tab's download button: the SimulationCraft program (`simc.exe`, about 116 MB) and its checksum, over HTTPS; the app checks the file against the checksum before it keeps it. A sim runs on your PC; of the character you paste, only its gear's item IDs leave it (to Wowhead, above).
- **Warcraft Logs' image server** (assets.rpglogs.com): raid room map images, for the raid replay.

As with any web request, those sites can see your IP address. Each request names WoW Live and its GitHub page.

## Children

WoW Live doesn't knowingly collect data from anyone, including children.

## Changes and contact

Changes to this policy are made here, in this repository. For questions, open an issue at https://github.com/steezy2/wow-live-app/issues.
