import type { SimChoices, SimItem, SimOptions } from './api'

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

/** The items of each slot group (indexes into the profile's equipped and bags lists), in wearing order; empty
 * groups left out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[]) {
  return SLOT_GROUPS.map(([key, label]) => ({
    key,
    label,
    items: [
      ...equipped.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: true }] : [])),
      ...bags.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: false }] : [])),
    ],
  })).filter((g) => g.items.length > 0)
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
