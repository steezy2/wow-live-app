import type {
  SimChoices,
  SimConsumables,
  SimEnchant,
  SimItem,
  SimOptions,
  SimReference,
  SimUpgradeLevel,
  SimUpgradeTrack,
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

/** The distinct item IDs of items. */
export const itemIDs = (items: SimItem[]): number[] => [...new Set(items.map((i) => i.id))]

/** How wide a gain's bar is, in percent of the best gain's (0 for a loss or when nothing gains). */
export function gainShare(gain: number, best: number): number {
  return best > 0 ? Math.max(0, (gain / best) * 100) : 0
}
