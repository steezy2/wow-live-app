import type { SimChoices, SimItem, SimOptions } from './api'

/** What is wrong with the fight options, in words, or null when they fit choices (the app refuses the same ones). */
export function simOptionsError(o: SimOptions, c: SimChoices): string | null {
  if (!Number.isInteger(o.length) || o.length < c.minLength || o.length > c.maxLength)
    return `The fight length is ${c.minLength} to ${c.maxLength} seconds.`
  if (!Number.isInteger(o.targets) || o.targets < 1 || o.targets > c.maxTargets)
    return `Targets are 1 to ${c.maxTargets}.`
  return null
}

/** The slots Top Gear lists its items under, in the order they are worn, by a slot's group ("finger1" and "finger2" are rings). */
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

/** The items of each slot group (indexes into the profile's equipped and bags lists), in wearing order; empty groups left out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[]) {
  return SLOT_GROUPS.map(([key, label]) => ({
    label,
    items: [
      ...equipped.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: true }] : [])),
      ...bags.flatMap((item, index) => (groupKey(item.slot) === key ? [{ item, index, worn: false }] : [])),
    ],
  })).filter((g) => g.items.length > 0)
}
