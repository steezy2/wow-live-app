import type {
  SimChoices,
  SimConsumables,
  SimEnchant,
  SimItem,
  SimOptions,
  SimReference,
  SimUpgradeLevel,
  SimUpgradeTrack,
  Suggestion,
  SuggestTargetChoice,
  TalentNode,
} from './api'

/** What a Suggest report is ranked by: the larger gain of the two fights, or one of them. */
export type RankBy = 'best' | 'single' | 'aoe'

/** The suggestions in the order `by` ranks them: the server's for 'best' (the larger gain); else by the gain on that
 * fight within each of the server's groups (the tested changes, the stats, the talents that add no DPS: the backend's
 * untestedRank), which keep the server's order. */
export function rankSuggestions(list: Suggestion[], by: RankBy): Suggestion[] {
  if (by === 'best') return list
  const group = (s: Suggestion) => (s.kind === 'stat' || s.kind === 'talentOther' ? s.kind : '')
  const first = (s: Suggestion) => list.findIndex((t) => group(t) === group(s))
  const gain = (s: Suggestion) => (by === 'single' ? s.gain : s.aoeGain) ?? 0
  return [...list].sort((a, b) => first(a) - first(b) || gain(b) - gain(a))
}

/** What is wrong with the fight options, in words, or null when they fit choices (the app refuses the same ones). */
export function simOptionsError(o: SimOptions, c: SimChoices): string | null {
  if (!Number.isInteger(o.length) || o.length < c.minLength || o.length > c.maxLength)
    return `The fight length is ${c.minLength} to ${c.maxLength} seconds.`
  if (!Number.isInteger(o.targets) || o.targets < 1 || o.targets > c.maxTargets)
    return `Targets are 1 to ${c.maxTargets}.`
  return null
}

/** What each kind of sim is called (a sim saved before consumables moved into each kind says "consumables"). */
const KIND_LABELS: Record<string, string> = {
  quick: 'Quick Sim',
  topgear: 'Top Gear',
  drops: 'Droptimizer',
  stats: 'Stat Weights',
  suggest: 'Suggestions',
  consumables: 'Consumables',
}

/** A kind of sim's name on the page; a kind it doesn't know shows as the app names it. */
export const kindLabel = (kind: string) => KIND_LABELS[kind] ?? kind

/** The page's names for the items' groups (SimItem.group). */
const GROUP_LABELS: Record<string, string> = {
  shoulder: 'Shoulders',
  wrist: 'Wrists',
  finger: 'Rings',
  trinket: 'Trinkets',
  main_hand: 'Main hand',
  off_hand: 'Off-hand',
}

/** A group's name on the page: "Rings" for "finger", else the group capitalized. */
export const groupLabel = (group: string) => GROUP_LABELS[group] ?? group.charAt(0).toUpperCase() + group.slice(1)

/** The paper doll's column each group is drawn in: 0 down the left, 1 down the right, 2 along the bottom (weapons). */
const DOLL_COLUMNS: Record<string, number> = {
  head: 0, neck: 0, shoulder: 0, back: 0, chest: 0, wrist: 0,
  hands: 1, waist: 1, legs: 1, feet: 1, finger: 1, trinket: 1,
}

/** The items of each group in order (SimChoices.gearGroups), as indexes into the profile's equipped and bags lists,
 * the highest item level first (worn or carried; items with no level last, in the export's order); empty groups left
 * out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[], order: string[]) {
  return order
    .map((group) => ({
      key: group,
      label: groupLabel(group),
      column: DOLL_COLUMNS[group] ?? 2,
      items: [
        ...equipped.flatMap((item, index) => (item.group === group ? [{ item, index, worn: true }] : [])),
        ...bags.flatMap((item, index) => (item.group === group ? [{ item, index, worn: false }] : [])),
      ].sort((a, b) => (b.item.level ?? 0) - (a.item.level ?? 0)),
    }))
    .filter((g) => g.items.length > 0)
}

/** The upgrade track an item is on and its level in it, found by its bonus IDs; null for an item on none. */
export function upgradeTrackOf(
  ref: SimReference,
  item: SimItem,
): { track: SimUpgradeTrack; level: SimUpgradeLevel } | null {
  const bonuses = item.opts?.bonus_id ?? []
  for (const track of ref.upgrades) {
    const level = track.levels.find((l) => bonuses.includes(l.bonusID))
    if (level) return { track, level }
  }
  return null
}

/** The enchants that fit a slot ("finger1", "main_hand"). */
export const enchantsFor = (ref: SimReference, slot: string): SimEnchant[] =>
  ref.enchants.filter((e) => e.slots.includes(slot))

/** How many combinations the consumables picked make: the product of the values of each kind with two or more (1 when
 * nothing is compared). */
export const comparedCount = (c: SimConsumables): number =>
  Object.values(c).reduce((n, values) => (values.length > 1 ? n * values.length : n), 1)

/** The entry IDs of an Omnium Folio string ("136814:1/136817:1"). */
export const omniumPicks = (text: string | undefined): number[] =>
  (text ?? '').split('/').flatMap((pair) => (/^\d+:\d+$/.test(pair) ? [Number(pair.split(':')[0])] : []))

/** The picks as the app sends them, in the tree's node order and each node's entry order: the export's format, but a
 * node can have several picks (they are compared). */
export function omniumString(nodes: TalentNode[], picks: number[]): string {
  return [...nodes]
    .sort((a, b) => a.id - b.id)
    .flatMap((n) => n.entries.filter((e) => picks.includes(e.id)).map((e) => `${e.id}:1`))
    .join('/')
}

/** The picks after clicking an entry: it is picked as well as the others of its node, or unpicked when it was. */
export const togglePick = (picks: number[], entry: number): number[] =>
  picks.includes(entry) ? picks.filter((p) => p !== entry) : [...picks, entry]

/** The distinct item IDs of items. */
export const itemIDs = (items: SimItem[]): number[] => [...new Set(items.map((i) => i.id))]

/** Where this browser keeps a character's last Suggest target, by name. */
const targetKey = (character: string) => `sim.suggestTarget.${character}`

/** The Suggest target last picked for a character, if it is still one of the choices, else Mythic+ overall (the
 * first). */
export function savedSuggestTarget(choices: SimChoices, character: string): SuggestTargetChoice {
  let name: string | null = null
  try {
    name = localStorage.getItem(targetKey(character))
  } catch {
    // storage off: the default
  }
  return choices.suggestTargets.find((t) => t.name === name) ?? choices.suggestTargets[0]
}

/** Keeps the Suggest target picked for a character in this browser (best effort). */
export function saveSuggestTarget(character: string, name: string) {
  try {
    localStorage.setItem(targetKey(character), name)
  } catch {
    // storage off: not remembered
  }
}

/** How wide a gain's bar is, in percent of the best gain's (0 for a loss or when nothing gains). */
export function gainShare(gain: number, best: number): number {
  return best > 0 ? Math.max(0, (gain / best) * 100) : 0
}
