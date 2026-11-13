# Microsoft Store listing

The text and images for the Store page (Partner Center > WoW Live > Store listings > English). This file is the one
source of truth for them: change it here, then paste into Partner Center with the next submission. Matches 0.4.1.

Rules for the copy: lead with what the player gets, not how it works; say "M+" and "raid" in the first two lines
(that's all the page shows before *Read more*); no Blizzard logos, art or class icons; no claim the app doesn't make.

## Short description (top of the page)

Know why your key depleted. WoW Live times your Mythic+ keys and raid pulls live from the combat log, then shows
every death, every missed kick and what to fix before the next pull. Free, no addon, nothing uploaded.

## Description

Know why your key depleted, and why the raid wiped.

WoW Live reads World of Warcraft's combat log as you play and turns it into answers: a live M+ timer and forces
overlay, every death with what killed the player and the defensive they didn't press, and a short "fix next pull"
list after every key and every raid pull. No addon to install, nothing to upload, no waiting for a log to process.

MYTHIC+, LIVE
• The clock against the timer with the +2 and +3 marks, death penalty included, and enemy forces.
• The result you're heading for, with the odds of each, from your own runs and the top runs on Warcraft Logs.
• The must-stop casts that went off, deaths, interrupts, dispels and DPS / HPS for the last pull and the whole key.
• An overlay over the game that fits its window: the two cards and the columns you pick. Click a player for their
  top spells and damage into each enemy, or let clicks through to the game with Ctrl+Alt+K.

AFTER EVERY KEY
• Each player against their spec's top runs on Warcraft Logs: DPS, kicks, deaths with a defensive ready, potions at
  Bloodlust, with up to four things to work on and one done well.
• Every death and big hit, recapped: the last 10 seconds of health, each hit and who cast it, the heals and shields
  received and the buttons pressed.
• Damage spikes that name what hit the party, and a "Fix next key" list: late priority targets, missed kicks,
  Bloodlusts and deaths that cost the timer.
• A replay on the dungeon's map with a timeline of pulls, deaths, Bloodlust and cooldowns, synced to the meters.

RAID
• After each pull: how far it got, the early deaths and why, and what went wrong by mechanic (soaks, tethers, adds,
  dispels, avoidable hits), with a short list of what to fix on the next pull.
• A replay of the pull on the room's map, raid cooldowns and potions, missing flasks, food and Vantus Runes.

YOUR SEASON
• A Recap for each of your characters, Mythic+ and Raid apart: keys timed, highest key, your fastest and closest
  runs, the ability that kills you most, who you ran with, your times against the top runs and your Warcraft Logs
  parses.
• Read your old combat logs, or import keys and raid pulls from Warcraft Logs, and they're all saved.

PRIVATE AND SAFE
It isn't an addon and doesn't touch the game: it only reads the log file the game writes on your PC. No account, no
ads, no tracking, and nothing from your combat log leaves your PC.

GET STARTED IN A MINUTE
1. In the game, turn on Advanced Combat Logging (Options > System > Network).
2. Type /combatlog before the key or the pull.
3. Play in Windowed or Windowed (Fullscreen) so the overlay shows over the game.
Optional: add a free Warcraft Logs API client in Settings for the comparison with top runs.

WoW Live is free. Questions and bugs: github.com/steezy2/wow-live

World of Warcraft is a trademark of Blizzard Entertainment. WoW Live isn't affiliated with or endorsed by Blizzard.
Dungeon maps are Keystone.guru's and raid maps Warcraft Logs'.

## Product features (up to 20, shown as a list)

1. Live Mythic+ timer with +2 and +3 marks and enemy forces
2. Overlay over the game that fits its window, with the cards and columns you pick
3. Every death and big hit recapped: the last 10 seconds of hits, heals and buttons pressed
4. Must-stop casts that went off, interrupts, dispels and crowd control
5. DPS, HPS and avoidable damage for the last pull and the whole key
6. Review of each player against the top runs of their spec on Warcraft Logs
7. A "fix next pull" list after every key and raid pull
8. Damage spikes that name the abilities and enemies behind them
9. Replay on the dungeon's map with a timeline of pulls, deaths and cooldowns
10. Raid pulls: how far it got, early deaths and mechanics failed, by raider
11. A Recap of your season per character, Mythic+ and Raid apart, with your parses
12. Reads old combat logs and imports from Warcraft Logs
13. No addon, nothing uploaded, nothing from your log leaves your PC

## Search terms (up to 7)

World of Warcraft · Mythic Plus · M+ timer · WoW raid · combat log · Warcraft Logs · WoW overlay

## Category

Keep *Utilities & tools* unless Partner Center offers a games companion category; people find this by search, so
the search terms and the first two lines matter more than the category.

## Images

`docs/store/` holds what Partner Center takes, made from real screens of real keys and pulls with made-up names,
each under a one-line headline baked into the image (gamers skim the images, not the text). The first three show on
the page, so the order is what sells.

| File | Headline | Screen |
|---|---|---|
| `1-live.png` | Your Mythic+ key, live | The live page mid-key with the overlay over it |
| `2-key.png` | Know why your key depleted | A key's Fix next key list and Timeline |
| `3-raid.png` | Fix the wipe before the next pull | A raid pull's Fix next pull, early deaths and mechanics |
| `4-death.png` | Every death, explained | A death recap's last 10 seconds |
| `5-replay.png` | Replay the key on the map | The map replay |
| `6-recap.png` | Your whole season | The Recap's Mythic+ tab |

Also in `docs/store/`: `poster.png` (9:16 poster art, 720 × 1080, the main logo on Windows), `box.png` (1:1 box
art, 1080 × 1080), `hero.png` (16:9 super hero art, 1920 × 1080, which must not show the product title) and
`logo.png` (300 × 300 app tile icon), so the page doesn't fall back to the package's small logo. The README shows the
same screenshots. To make them again: replay a key with `go run ./cmd/site --replay`, capture at 1920 × 1080 with the
names swapped for made-up ones, and put a headline over each.
