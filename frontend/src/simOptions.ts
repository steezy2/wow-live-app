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

/** An item as the page names it: its name (or ID) and item level. */
export const itemText = (item: SimItem) => `${item.name ?? `item ${item.id}`}${item.level ? ` (${item.level})` : ''}`

/** The items of each group in order (SimChoices.gearGroups), as indexes into the profile's equipped and bags lists,
 * worn ones first; empty groups left out. */
export function gearGroups(equipped: SimItem[], bags: SimItem[], order: string[]) {
  return order
    .map((group) => ({
      label: groupLabel(group),
      items: [
        ...equipped.flatMap((item, index) => (item.group === group ? [{ item, index, worn: true }] : [])),
        ...bags.flatMap((item, index) => (item.group === group ? [{ item, index, worn: false }] : [])),
      ],
    }))
    .filter((g) => g.items.length > 0)
}
