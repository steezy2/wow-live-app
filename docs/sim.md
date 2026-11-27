# The Sim tab

How the desktop app's Sim tab runs SimulationCraft. `CLAUDE.md` points here; this file is the one place for these facts.

## Page

- `Sim.tsx` over `SimGear.tsx` (the gear by slot), `SimItem.tsx` / `simItems.ts` (item icons from `GET /api/sim/items`,
  `service.Items` over `wowhead.Client.Item`, asked `SimChoices.MaxItemLookups` at a time), `SimResult.tsx` (the
  report: head, best gear, ranked rows with the equipped gear among them, details), `simOptions.ts`.
- `SimResult.tsx` sits over `SimSources.tsx` (a Droptimizer's bosses and dungeons ranked) and `SimBreakdown.tsx` (the
  school-coloured ability and buff bars and a Quick Sim's sample ability log, `SimResult.Log` from json2's
  `action_sequence`). Spell icons and hover texts come from `GET /api/sim/spells` (`Items.SpellInfos`), not
  `/api/spells`, which refuses a player's own spells.
- The page's time left is `nextLeft` in `simOptions.ts`, from the plan's seconds and counting down.

## Engine

- `clients/simc` installs `simc.exe` from this repo's `simc-engine` release over HTTPS, checked against the sha256 in
  its `engine.json` plus `MZ` and banner checks (or copies a `simc.exe` the owner names).
- `.github/workflows/simc.yml` rebuilds that release weekly from the official nightly in simulationcraft/simc-publish's
  Actions artifacts, checking their sha256, and runs only on `main`. simulationcraft.org itself is plain HTTP with no
  checksum, so the app never uses it.

## Input

- `service/simprofile.go` reads the pasted `/simc` export into validated fields (`ParseSimC`, `SimInput`). The export
  is untrusted: simc's `input=`/`output=` lines never reach the file, and each request re-parses the text.
- Every name that reaches simc's input is looked up in `reference/sim-options.json` (`go run ./cmd/simoptref` from
  Raidbots' public game data, read through `clients/raidbots` like `cmd/talentref`, which refuses a consumable name
  that isn't `[a-z0-9_]`; embedded in the app; `GET /api/sim/options`), never copied from the request.
- `SimRequest.Custom` adds allowlisted simc lines (`service/simcustom.go`, `customLines`).

## Runs

- `service/sim.go` runs up to `maxParallelSims` at once (`Sims.runs`; failed and cancelled ones in `Sims.ended`) and
  maps json2. Progress is read from simc's lines by `parseSimProgress`, the parallel `Profilesets (3*4): i/N` one
  included.
- A big sim's stages are `service/simsmart.go` (`smartStages`, `survivors`; `Sims.execute` runs them). Every kind's
  stages are `Sims.race` (the smart stages, each its share of the progress bar by `smartCosts`, simc's setup per
  profileset included: `profilesetSetup`) and `Sims.finish` (state, save).
- `POST /api/sim/plan` counts any request's runs and times them (`Sims.Plan`, `planSeconds`; Top Gear's combinations by
  `comboCount`, refused by `comboRefusal` in the same words as Start) and names its variants (`SimPlan.Notes`) for the
  page's cost line and chips (`simPlan.ts`).
- A `stats` sim's `calculate_scale_factors` is read into weights and a Pawn string (`service/simstats.go`).
- `controller/sim.go` serves `/api/sim`. Tests use an in-process fake engine (`fakeEngine`).

## Top Gear

- `simgear.go` adds profilesets (`maxCombos`). Whether a bag weapon is two-handed is asked of Wowhead
  (`wowhead.Client.TwoHanded`).
- `Variants` (`service/simoptions.go`): a changed copy of a picked item, with its upgrade-track bonus ID swapped,
  `SocketBonus` added, a `gem_id` and an `enchant_id`, joining the candidates before `gearGroups` counts them. A socket
  is added only on a head, wrist or waist without one, as `SimReference.SocketSlots` from Raidbots' season data says
  (`canAddSocket`; `ParseProfile` gives each item its `Sockets` and `AddSocket`).
- *Copy and Modify* follows Raidbots': `SimVariant` also takes an `Embellishment`, a `Recraft` (reagent IDs from
  `reference/sim-crafting.json`, `cmd/simcraftref`, loaded into `SimReference.Crafting` and never served;
  `ParseProfile` gives each item its `Embellishments`, `Recrafts`, `Removable` and `Converts`), a `Remove` and a
  Catalyst conversion (`service/simcraft.go`: a conversion group's bonus ID on the item, the item the class wears in its
  slot; `SimRequest.CatalystCharges` caps them per combination in `gearRules`).
- `SimRequest.Enhancements` tries enchants per slot and gems on the picked items as extra candidates
  (`service/simenhance.go`; unique gems by limit category are never on two items: `uniqueGemsOK`).

## Consumables, Omnium and fight options

- A request names `Consumables` (`flask=`, `food=`, `potion=`, `augmentation=`, `temporary_enchant=main_hand:` lines)
  as lists per kind, and an Omnium Folio (`omnium_talents=entryID:1/...`, entry IDs of Raidbots' tree; simc ignores the
  rank).
- One value is a baseline line; two or more of a kind or of an Omnium node are compared: `consumableDims` and
  `omniumInput` give the `compareDim`s and `compareCombos` makes every combination of them as profilesets
  (`gearCombo.line`). A Quick Sim's rows are `SimResult.Options`; the other kinds cross each combination with every
  value (`withCompare`) and name it in `SimCombo.Consumable`.
- `SimChoices.Scenarios` and `SimOptions.NoRaidBuffs` start the fight options.

## Droptimizer

- `simdrops.go` (`maxDrops`). Drop data is `reference/sim-drops.json` (`go run ./cmd/simref`, embedded in the app)
  with item levels in the hand-kept `reference/sim-drops.toml`.
- `Upgrade` (`dropUpgrade`, `upgradedLevel`) sims each drop as dropped (empty), at the top rank of its upgrade track
  (`max`, `trackOf`/`rankLevel`) plus `venomstoneLevels` on a Hero or Myth weapon, trinket or neck (Ascendant
  Venomstones: Myth 334 to 341), at the rank of the gear worn in its slot (`match`) or at a rank number.
- A drop of a piece already worn in that slot at that item level or higher is left out: a sim of it against itself
  only shows noise.

## Suggest

- A `suggest` sim (`service/suggest.go`, `suggestRun`) is one job of several races: the stat weights while `Sims.Tops`
  (`SuggestLogs`, the `Comparer`; refused without it or without saved Warcraft Logs keys, `Sims.KeysSet`) reads the top
  players of the request's `Target`.
- Targets are `SimChoices.SuggestTargets` (`Sims.suggestTargets`): Mythic+ overall (`suggestLogsPerDungeon` logs in
  each of the first `suggestDungeons` dungeons at `suggestKeyLevel`) or one raid boss of `Sims.Bosses` on a ranked
  difficulty; `Comparer.TopBuilds` picks logs near the median ranked time. The page keeps the pick per character in
  `localStorage`.
- The pure `buildSuggestions` (`suggest_build.go`) makes one profileset per enchant, gem, flask or potion change (no
  food: every food's buff is "Well Fed" in the logs; `changeCombo`, built from the reference and the export's items;
  `maxSuggestions`), then Top Gear over everything carried and the Droptimizer at its defaults fully upgraded (`max`,
  Venomstones included; `suggestGear`), each worn item at the top of its track with Venomstones where they fit
  (`upgradeSuggestions`, `SimVariant.Venomstone`), then every shown change together (`combined`).
- `gained` drops a change under `suggestMinGain` or within `suggestZ` errors on both fights. Every race but the stat
  weights is `suggestRun.raceBoth`: the same combinations on each of `suggestFights` (two `simScenarios`, looked up
  once by `scenarioNamed`, which panics on a renamed one; `suggestFight.options`), giving `Suggestion.Gain` and
  `AoEGain`; the change shows when either passes. `planSuggest` times those runs on each fight's options.

## Talents

- The export's `talents=` is the game's loadout string, read and written by `service/talentcode.go` (`DecodeTalents`,
  `EncodeTalents`, `validTalents`; `SimProfile.Picks`) against `reference/talent-trees.json`.
- `talentSuggestions` swaps in what `suggestShare` of the top logs took (`maxTalentSuggestions`) for a choice node's
  other entry or the least-taken talent of the same rank in its section that `validTalents` allows (points, links,
  `ReqPoints` gates; `replacedTalent`).
- Never a free or tiered node, whose entry the string doesn't say, nor the hero tree the character doesn't play (the
  `SubTree` of its `HeroChoice` entry: the string holds both and simc ignores the other).
- A talent with no allowed swap is dropped with a note; a swap without a gain is kept as `talentOther` (evidence, its
  loss if any; not in `Dropped`).
