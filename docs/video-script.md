# Video scripts

Two videos to record later: a 50 second trailer for the Store page and a 3 minute walkthrough for YouTube and Reddit.
Record at 1920 × 1080, 30 fps, from a replay of a real key (`go run ./cmd/site --replay PATH --key 2 --speed 30`, or
the app on a real night) with made-up names, as the screenshots do. Use the app's dark theme. No Blizzard footage,
logos or music; use royalty-free music or none. A voice-over reads the lines in **bold**; on-screen text is in
*italics*. If the price changes, update the "free" lines.

## Trailer (50 s, Store page)

The Store autoplays it muted, so every beat has captions and the video works without sound.

| Time | Screen | Voice and caption |
|---|---|---|
| 0-4 | Black, then a key's timer ticking red past 30:00 | **"Your key just depleted. Why?"** *Why did it deplete?* |
| 4-10 | The live page mid-key, overlay over the game | **"WoW Live reads your combat log as you play."** *Live timer, forces, odds* |
| 10-18 | Click a death: the last 10 seconds, health bars falling, hits, heals, the ready defensive highlighted | **"Every death explained: the hit, the heal, the defensive you didn't press."** *Every death, explained* |
| 18-26 | Compare tab: you against the top logs, bars and the "work on" list | **"See how you stack up against the top runs of your spec."** *Compare with the top runs* (sample data) |
| 26-32 | Fix next key list, then the Timeline scrubbing | **"Then a short list of what to fix next key."** *Fix next key* |
| 32-38 | Raid pull: early deaths and mechanics failed | **"Raiding? The same for every pull."** *Fix the wipe before the next pull* |
| 38-43 | Map replay, scrubbing the timeline | **"Replay it on the map."** *Replay the key* |
| 43-50 | Logo, "Free on the Microsoft Store", the three setup steps as captions | **"Free. No addon. Your log never leaves your PC."** *Free · No addon · Private* |

Notes: the first 4 seconds decide whether anyone stays, so open on the pain, not the logo. Show the app large, never a
whole 1080p page shrunk; zoom to the part the line talks about.

## Walkthrough (3 min, YouTube and Reddit)

Chapters (add them to the description). The speaker is the owner, in first person.

1. **0:00 Hook (15 s).** Show a depleted key. "I got tired of guessing why my keys failed, so I built an app that
   tells me. It's free."
2. **0:15 What it is (20 s).** It reads the combat log on your PC. Not an addon, no upload, nothing touches the game.
   Show the Home tab and the setup guide.
3. **0:35 Set up (25 s).** Advanced Combat Logging on, `/combatlog` before the key, Windowed mode. Optional Warcraft
   Logs API client for the comparison, with the redirect URL and *Public Client* unchecked.
4. **1:00 During the key (30 s).** Live page and overlay: timer, forces, the result you're heading for, must-stop
   casts. Ctrl+Alt+K for click-through. Say the log arrives in batches, so a pull's numbers show when it ends.
5. **1:30 After the key (50 s).** Fix next key, then open a death: the last 10 seconds. Then the Compare tab: how a
   player's DPS, kicks and deaths sit against the top logs, and the "work on" list. Then the replay on the map and
   the Timeline.
6. **2:20 Raid (20 s).** A pull's early deaths, mechanics failed and Fix next pull.
7. **2:40 Your season and close (20 s).** The Recap, Read old logs, then where to get it: "Free on the Microsoft
   Store; source and bugs on GitHub (steezy2/wow-live-app)." Ask for feedback and say what's next.

Checklist before recording: names made up in every shot (players, guild, realm); no Warcraft Logs keys on screen;
Settings closed when the secret is visible; close other windows; the overlay over a neutral, non-Blizzard backdrop
or a plain window.
