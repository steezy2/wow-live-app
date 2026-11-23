import type { SimChoices, SimItem, SimOptions } from './api'

/** What is wrong with the fight options, in words, or null when they fit choices (the app refuses the same ones). */
export function simOptionsError(o: SimOptions, c: SimChoices): string | null {
  if (!Number.isInteger(o.length) || o.length < c.minLength || o.length > c.maxLength)
    return `The fight length is ${c.minLength} to ${c.maxLength} seconds.`
  if (!Number.isInteger(o.targets) || o.targets < 1 || o.targets > c.maxTargets)
    return `Targets are 1 to ${c.maxTargets}.`
  return null
}

/** The slots gear is listed under, in the order they are worn, by a slot's group ("finger1" and "finger2" are rings),
 * with the paper doll's column each is drawn in: 0 down the left, 1 down the right, 2 along the bottom (weapons). */
const SLOT_GROUPS: [string, string, number][] = [
  ['head', 'Head', 0],
  ['neck', 'Neck', 0],
  ['shoulder', 'Shoulders', 0],
  ['back', 'Back', 0],
  ['chest', 'Chest', 0],
  ['wrist', 'Wrists', 0],
  ['hands', 'Hands', 1],
  ['waist', 'Waist', 1],
  ['legs', 'Legs', 1],
  ['feet', 'Feet', 1],
  ['finger', 'Rings', 1],
  ['trinket', 'Trinkets', 1],
  ['main_hand', 'Main hand', 2],
  ['off_hand', 'Off-hand', 2],
]

/** An item's group key: "finger" for both rings, "trinket" for both trinkets, else its slot. */
const groupKey = (slot: string) => slot.replace(/[12]$/, '')

/** The items of each slot group (indexes into the profile's equipped and bags lists), in wearing order; empty
 * groups left out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[]) {
  return SLOT_GROUPS.map(([key, label, column]) => ({
    key,
    label,
    column,
    items: [
      ...equipped.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: true }] : [])),
      ...bags.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: false }] : [])),
    ],
  })).filter((g) => g.items.length > 0)
}

/** The distinct item IDs of items. */
export const itemIDs = (items: SimItem[]): number[] => [...new Set(items.map((i) => i.id))]

/** How wide a gain's bar is, in percent of the best gain's (0 for a loss or when nothing gains). */
export function gainShare(gain: number, best: number): number {
  return best > 0 ? Math.max(0, (gain / best) * 100) : 0
}
