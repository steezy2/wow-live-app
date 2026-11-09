import type { KeyMeterRow } from './api'
import { amount } from './format'

/** A card column: its heading, the value it sorts and draws by, and how it shows. */
export interface Column {
  label: string
  value: (r: KeyMeterRow) => number
  show?: (r: KeyMeterRow) => string
  title?: (r: KeyMeterRow) => string
}

/** The offense columns. */
export const offense: Column[] = [
  { label: 'DPS', value: (r) => r.dps },
  { label: 'Damage', value: (r) => r.damage },
  {
    label: 'Kicks',
    value: (r) => r.interrupts,
    show: (r) => String(r.interrupts),
  },
  {
    label: 'Dispels',
    value: (r) => r.dispels + r.purges,
    show: (r) => String(r.dispels + r.purges),
    title: (r) => `${r.dispels} dispels, ${r.purges} purges`,
  },
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
    { label: 'Taken', value: (r) => r.damageTaken },
    {
      label: 'Avoidable',
      value: (r) => r.avoidable,
      show: (r) => (listed ? amount(r.avoidable) : '–'),
    },
  ]
}
