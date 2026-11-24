import type { SimChoices, SimItem, SimOptions } from './api'

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
 * worn ones first; empty groups left out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[], order: string[]) {
  return order
    .map((group) => ({
      key: group,
      label: groupLabel(group),
      column: DOLL_COLUMNS[group] ?? 2,
      items: [
        ...equipped.flatMap((item, index) => (item.group === group ? [{ item, index, worn: true }] : [])),
        ...bags.flatMap((item, index) => (item.group === group ? [{ item, index, worn: false }] : [])),
      ],
    }))
    .filter((g) => g.items.length > 0)
}

/** The distinct item IDs of items. */
export const itemIDs = (items: SimItem[]): number[] => [...new Set(items.map((i) => i.id))]

/** How wide a gain's bar is, in percent of the best gain's (0 for a loss or when nothing gains). */
export function gainShare(gain: number, best: number): number {
  return best > 0 ? Math.max(0, (gain / best) * 100) : 0
}
