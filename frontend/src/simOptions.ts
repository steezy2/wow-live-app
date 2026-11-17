import type {
  SimChoices,
  SimConsumables,
  SimEnchant,
  SimItem,
  SimOptions,
  SimReference,
  SimUpgradeLevel,
  SimUpgradeTrack,
  SimVariant,
  TalentNode,
} from './api'

/** What is wrong with the fight options, in words, or null when they fit choices (the app refuses the same ones). */
export function simOptionsError(o: SimOptions, c: SimChoices): string | null {
  if (!Number.isInteger(o.length) || o.length < c.minLength || o.length > c.maxLength)
    return `The fight length is ${c.minLength} to ${c.maxLength} seconds.`
  if (!Number.isInteger(o.targets) || o.targets < 1 || o.targets > c.maxTargets)
    return `Targets are 1 to ${c.maxTargets}.`
  return null
}

/** The slots Top Gear lists its items under, in the order they are worn, by a slot's group ("finger1" and "finger2"
 * are rings). */
const SLOT_GROUPS: [string, string][] = [
  ['head', 'Head'],
  ['neck', 'Neck'],
  ['shoulder', 'Shoulders'],
  ['back', 'Back'],
  ['chest', 'Chest'],
  ['wrist', 'Wrists'],
  ['hands', 'Hands'],
  ['waist', 'Waist'],
  ['legs', 'Legs'],
  ['feet', 'Feet'],
  ['finger', 'Rings'],
  ['trinket', 'Trinkets'],
  ['main_hand', 'Main hand'],
  ['off_hand', 'Off-hand'],
]

/** An item's group key: "finger" for both rings, "trinket" for both trinkets, else its slot. */
const groupKey = (slot: string) => slot.replace(/[12]$/, '')

/** The items of each slot group (indexes into the profile's equipped and bags lists), in wearing order, the highest
 * item level first (worn or carried; items with no level last, in the export's order); empty groups left out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[]) {
  return SLOT_GROUPS.map(([key, label]) => ({
    key,
    label,
    items: [
      ...equipped.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: true }] : [])),
      ...bags.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: false }] : [])),
    ].sort((a, b) => (b.item.level ?? 0) - (a.item.level ?? 0)),
  })).filter((g) => g.items.length > 0)
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

/** A variant in words: "Myth 3/6 · socket + Quick Lapis · Amani Mastery". */
export function variantLabel(ref: SimReference, item: SimItem, v: SimVariant): string {
  const parts: string[] = []
  const on = upgradeTrackOf(ref, item)
  if (v.upgrade && on) parts.push(`${on.track.name} ${v.upgrade}/${on.track.levels.length}`)
  const gem = ref.gems.find((g) => g.id === v.gem)?.name
  if (v.socket || gem) parts.push([v.socket && 'socket', gem].filter(Boolean).join(' + '))
  const enchant = ref.enchants.find((e) => e.id === v.enchant)
  if (enchant) parts.push(enchant.name)
  return parts.join(' · ')
}

/** The consumables set, without the kinds left on simc's default. */
export function pickedConsumables(c: SimConsumables): SimConsumables {
  return Object.fromEntries(Object.entries(c).filter(([, v]) => v)) as SimConsumables
}

/** The entry IDs of an Omnium Folio string ("136814:1/136817:1"). */
export const omniumPicks = (text: string | undefined): number[] =>
  (text ?? '').split('/').flatMap((pair) => (/^\d+:\d+$/.test(pair) ? [Number(pair.split(':')[0])] : []))

/** The picks as the export writes them, in the tree's node order, one entry to a node. */
export function omniumString(nodes: TalentNode[], picks: number[]): string {
  return [...nodes]
    .sort((a, b) => a.id - b.id)
    .flatMap((n) => {
      const entry = n.entries.find((e) => picks.includes(e.id))
      return entry ? [`${entry.id}:1`] : []
    })
    .join('/')
}

/** The picks after clicking an entry: it is picked (replacing another of its node) or, when it was, unpicked. */
export function togglePick(node: TalentNode, picks: number[], entry: number): number[] {
  const others = picks.filter((p) => !node.entries.some((e) => e.id === p))
  return picks.includes(entry) ? others : [...others, entry]
}

/** The distinct item IDs of items, at most the 120 an items request takes. */
export function itemIDs(items: SimItem[]): number[] {
  return [...new Set(items.map((i) => i.id))].slice(0, 120)
}

/** The average item level of the worn items that have one, rounded (0 when none has). */
export function averageLevel(equipped: SimItem[]): number {
  const levels = equipped.flatMap((i) => (i.level ? [i.level] : []))
  return levels.length ? Math.round(levels.reduce((a, b) => a + b, 0) / levels.length) : 0
}

/** How wide a gain's bar is, in percent of the best gain's (0 for a loss or when nothing gains). */
export function gainShare(gain: number, best: number): number {
  return best > 0 ? Math.min(100, Math.max(0, (gain / best) * 100)) : 0
}

/** The slot groups drawn down the left of the paper doll, down its right, and along the bottom (the weapons). */
export const DOLL_COLUMNS: string[][] = [
  ['head', 'neck', 'shoulder', 'back', 'chest', 'wrist'],
  ['hands', 'waist', 'legs', 'feet', 'finger', 'trinket'],
  ['main_hand', 'off_hand'],
]
