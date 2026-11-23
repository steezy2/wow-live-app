# Microsoft Store listing

The text and images for the Store page (Partner Center > WoW Live > Store listings > English). This file is the one
source of truth for them: change it here, then paste into Partner Center with the next submission. The video scripts
are in [video-script.md](video-script.md).

Rules for the copy: lead with what the player gets, not how it works; say "M+" and "raid" in the first two lines
(that's all the page shows before *Read more*); one idea a line, no feature the page can't show in a screenshot; no
claim the app doesn't make; no Blizzard logos, art or class icons in the listing's own images.

## Short description (top of the page)

Know why your key depleted. WoW Live reads your combat log live: the M+ timer, every death explained and how you
compare with the top runs, then what to fix before the next pull. Free, no addon, your log stays on your PC.

## Description

Know why your key depleted, and why the raid wiped.

WoW Live reads World of Warcraft's combat log while you play and tells you what happened: a live M+ timer, every
death explained, and a short "fix next pull" list after every key and raid pull. No addon, no upload, no waiting for
a log to process.

DURING THE KEY
• The clock against the +2 and +3 marks and enemy forces, with the result you're heading for and the odds of each.
• Must-stop casts that went off, deaths, interrupts, dispels and DPS / HPS for the last pull and the whole key.
• An overlay right over the game. Press Ctrl+Alt+K to let clicks through to it.

AFTER EVERY KEY
• Death recaps: the last 10 seconds before each death, with every hit, heal and shield, and the defensive that was
  still ready.
• See how you compare: each player against the top runs of their spec on Warcraft Logs (DPS, abilities, buffs,
  talents), with up to five things to work on.
• A "Fix next key" list and a timeline of every pull, death, Bloodlust and cooldown.
• A replay on the dungeon's map, synced to the meters.

ROUTES
• Import a Mythic Dungeon Tools route, or record your own run as a new one.
• Edit it on the dungeon's map with mob health at your key level, and draw lines, markers and notes.
• Follow it live: the overlay shows "Next: pull N" as you play.
• After the key, see which pulls you skipped.

RAID
• After each pull: how far it got, the early deaths and why, and what failed by mechanic (soaks, tethers, adds,
  dispels, avoidable hits), with what to fix next pull.
• A replay on the room's map, plus missing flasks, food and Vantus Runes.

SIMS
• Paste the export of the SimulationCraft addon and sim your character on your PC: a Quick Sim, Top Gear for the gear
  you carry, and a Droptimizer for what the season's raid and dungeons drop.
• Uses SimulationCraft, downloaded the first time you ask for a sim. Your character stays on your PC: only its gear's
  item IDs go to Wowhead, for their names and icons.

YOUR SEASON
• A Recap per character: keys timed, your fastest and closest runs, the ability that kills you most, who you ran
  with and your Warcraft Logs parses. Read your old logs or import from Warcraft Logs.

PRIVATE
It isn't an addon and doesn't touch the game: it only reads the log file the game writes on your PC. No account, no
ads, no tracking. Your combat log never leaves your PC; Warcraft Logs only sees the characters you look up or
review.

START IN A MINUTE
1. In the game, turn on Advanced Combat Logging (Options > System > Network).
2. Type /combatlog before the key or the pull.
3. Play in Windowed or Windowed (Fullscreen) so the overlay shows over the game.
Optional: add a free Warcraft Logs API client in Settings for the comparison with top runs.

WoW Live is free. Its source is public for noncommercial use (PolyForm Noncommercial 1.0.0). Questions and bugs:
github.com/steezy2/wow-live-app

World of Warcraft is a trademark of Blizzard Entertainment. WoW Live isn't affiliated with or endorsed by Blizzard,
and the game's spell names, text and icons belong to Blizzard. Dungeon maps and patrols are Keystone.guru's, routes
and enemies Mythic Dungeon Tools', talent trees Raidbots', raid map edges wago.tools', raid maps and top runs
Warcraft Logs', season timers Raider.IO's and spell text and icons Wowhead's.

## What's new in this version

• Plan routes: import a Mythic Dungeon Tools route or record your own run, edit it with mob health at your key level,
  draw on the map, follow it live and see which pulls you skipped.
• Compare each player with the top runs of their spec, now with their talents and buffs, and faster to run.
• Death recaps show the last 10 seconds of every death and big hit.
• Replay a key or a raid pull on the map, with a timeline of deaths, Bloodlust and cooldowns.
• Import your keys and raid pulls from Warcraft Logs, or read your old combat logs.
• Opens on Home, with a short guide for first-time setup.

## Product features (up to 20, shown as a list)

1. Live Mythic+ timer with +2 and +3 marks, enemy forces and the odds of each result
2. Overlay over the game that fits its window
3. Every death recapped: the last 10 seconds of hits, heals and the defensive still ready
4. See how you compare: each player against the top runs of their spec on Warcraft Logs
5. A "fix next pull" list after every key and raid pull
6. Must-stop casts that went off, interrupts, dispels and avoidable damage
7. Replay on the dungeon's map with a timeline of pulls, deaths and cooldowns
8. Raid pulls: how far it got, early deaths and mechanics failed
9. Season Recap per character, with your parses
10. Routes: import a Mythic Dungeon Tools route and follow it live
11. Reads old combat logs and imports from Warcraft Logs
12. No addon, no account, your combat log never leaves your PC
13. Character sims on your PC with SimulationCraft: Quick Sim, Top Gear and a Droptimizer

## Search terms (up to 7)

World of Warcraft · Mythic Plus · M+ timer · WoW raid · combat log · Warcraft Logs · WoW overlay

## Category

Keep *Utilities & tools* unless Partner Center offers a games companion category; people find this by search, so
the search terms and the first two lines matter more than the category.

## Images

`docs/store/` holds what Partner Center takes, made from real screens of real keys and pulls with made-up names,
each under a one-line headline baked into the image (gamers skim the images, not the text). The first three show on
the page, so the order is what sells: the live key (the hook), a death explained (the pain) and the comparison (the
payoff). Each screen shows one thing, large; crop to it rather than showing a whole page.

| # | File | Headline | Screen | Status |
|---|---|---|---|---|
| 1 | `1-live.png` | Your Mythic+ key, live | Live page mid-key with the overlay over it | Made |
| 2 | `4-death.png` | Every death, explained | A death recap: each hit and heal as a health bar falling to 0%, the ready defensive named | Made (the last 10 seconds, health falling to 0%) |
| 3 | `compare.png` | See how you stack up | Compare tab: verdict, the "work on" list, key stats and abilities against the top logs | Made, sample data |
| 4 | `2-key.png` | Know why your key depleted | Fix next key and Timeline | Made |
| 5 | `routes.png` | Plan it. Follow it live. | The Routes tab: a route's pulls on the dungeon's map with forces per pull | Made |
| 6 | `3-raid.png` | Fix the wipe before the next pull | A raid pull's Fix next pull and early deaths | Made |
| 7 | `5-replay.png` | Replay the key on the map | The map replay | Made |
| 8 | `6-recap.png` | Your whole season | The Recap's Mythic+ tab | Made |

The comparison screenshot uses sample numbers, not a real review (the owner's choice); the names are made up too, and
the image carries a small "sample data" mark. Keep it to the values a real review shows. The talent tree shows
Blizzard's icons, so the Store image leaves the tree out.

Also in `docs/store/`: `poster.png` (9:16 poster art, 720 × 1080, the main logo on Windows), `box.png` (1:1 box
art, 1080 × 1080), `hero.png` (16:9 super hero art, 1920 × 1080, which must not show the product title) and
`logo.png` (300 × 300 app tile icon), so the page doesn't fall back to the package's small logo. The README shows the
same screenshots. To make them again: replay a key with `go run ./cmd/site --replay`, capture at 1920 × 1080 with the
names swapped for made-up ones, and put a headline over each (headless Edge over the DevTools protocol did the
death and compare shots: made-up names swapped in the page's text, the sample review answering the compare
request, then the crop framed under the headline). A trailer from the video script goes in Partner
Center's *Trailers* (1920 × 1080 MP4, with a 1920 × 1080 thumbnail).

## Before each submission

- Privacy policy URL: `https://github.com/steezy2/wow-live-app/blob/main/PRIVACY.md`. It lists every host the app
  calls at runtime (checked against the code on 2026-11-16: Warcraft Logs and its image server, Raider.IO,
  Keystone.guru's map tiles, Wowhead and its icon host); the build-time `cmd/` tools call others (GitHub, Raidbots,
  wago.tools) that the app doesn't. Add a new runtime host there in the same change.
- The manifest asks only for `internetClient` and `runFullTrust` (a desktop app). Don't add a capability the app
  doesn't use.
- Licence: PolyForm Noncommercial 1.0.0 for the source; the Store copy is free. The credits above must match the
  Home tab's credit line and the README.
- Keystone.guru's repository has no licence and its terms reserve all rights, so the maps and data are used
  without a written permission. Ask the author (their GitHub issues or Discord) and keep the reply here.
- Age rating: no user-generated content, no purchases, no ads.
