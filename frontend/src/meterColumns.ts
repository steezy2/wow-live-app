import type { KeyMeterRow } from './api'
import { amount } from './format'

/** A card column: its heading, the value it sorts and draws by, and how it shows. */
export interface Column {
  label: string
  value: (r: KeyMeterRow) => number
  show?: (r: KeyMeterRow) => string
  title?: (r: KeyMeterRow) => string
  /** Starts a new group (offense, defense): drawn with a faint rule on its left. */
  group?: boolean
  /** Colours the cell, given the row and every row of the card. */
  tone?: (r: KeyMeterRow, rows: KeyMeterRow[]) => 'warn' | undefined
}

/** Amber for avoidable damage over the party's median. */
const aboveMedian = (r: KeyMeterRow, rows: KeyMeterRow[]) => {
  const sorted = rows.map((x) => x.avoidable).sort((a, b) => a - b)
  const median = (sorted[Math.floor((sorted.length - 1) / 2)] + sorted[Math.floor(sorted.length / 2)]) / 2
  return r.avoidable > median ? 'warn' : undefined
}

/** The raid's offense columns: damage, kicks and dispels. */
export const raidOffense: Column[] = [
  { label: 'DPS', value: (r) => r.dps },
  { label: 'Damage', value: (r) => r.damage },
  {
    label: 'Kicks',
    group: true,
    value: (r) => r.interrupts,
    show: (r) => String(r.interrupts),
  },
  {
    label: 'Dispels',
    value: (r) => r.dispels + r.purges,
    show: (r) => String(r.dispels + r.purges),
    title: (r) => `${r.dispels} dispels, ${r.purges} purges`,
  },
]

/** The M+ offense columns: the raid's, and the share of damage into minibosses. */
export const offense: Column[] = [
  ...raidOffense,
  {
    label: 'Priority',
    value: (r) => (r.damage ? (100 * r.priorityDamage) / r.damage : 0),
    show: (r) => `${Math.round(r.damage ? (100 * r.priorityDamage) / r.damage : 0)}%`,
    title: () => "share of damage into the pulls' minibosses",
  },
]

/** The defense columns; avoidable damage shows "–" when the dungeon has no avoidable list. */
export function defense(listed: boolean): Column[] {
  return [
    { label: 'HPS', value: (r) => r.hps },
    { label: 'Healing', value: (r) => r.healing },
    { label: 'Taken', value: (r) => r.damageTaken, group: true },
    {
      label: 'Avoidable',
      value: (r) => r.avoidable,
      show: (r) => (listed ? amount(r.avoidable) : '–'),
      tone: listed ? aboveMedian : undefined,
    },
  ]
}
