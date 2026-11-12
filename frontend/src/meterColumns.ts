import type { KeyDeath, KeyMeter, KeyMeterRow, OverlayColumn } from './api'
import { median } from './format'

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
  return r.avoidable > median(rows.map((x) => x.avoidable)) ? 'warn' : undefined
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

/** The defense columns, with avoidable damage for a raid (a key's is in its Avoidable tab instead). */
export function defense(avoidable: boolean): Column[] {
  const columns: Column[] = [
    { label: 'HPS', value: (r) => r.hps },
    { label: 'Healing', value: (r) => r.healing },
    { label: 'Taken', value: (r) => r.damageTaken, group: true },
  ]
  return avoidable ? [...columns, { label: 'Avoidable', value: (r) => r.avoidable, tone: aboveMedian }] : columns
}

/** The column of the same name from a list, headed with a short label and without a group rule. */
function short(columns: Column[], label: string, as = label): Column {
  return { ...columns.find((c) => c.label === label)!, label: as, group: false }
}

/** An overlay card's column by its settings id (models.OverlayColumns): pull is the last pull, for the Pull DPS
 * column, and deaths the deaths inside the card's span, for the Deaths column. */
export function overlayColumn(id: OverlayColumn, pull: KeyMeter, deaths: KeyDeath[]): Column {
  switch (id) {
    case 'dps':
      return short(offense, 'DPS')
    case 'hps':
      return short(defense(false), 'HPS')
    case 'kicks':
      return short(offense, 'Kicks')
    case 'pullDps': {
      const byPlayer = new Map(pull.rows.map((r) => [r.player, r.dps]))
      return { label: 'Pull DPS', value: (r) => byPlayer.get(r.player) ?? 0 }
    }
    case 'taken':
      return short(defense(false), 'Taken')
    case 'avoidable':
      return short(defense(true), 'Avoidable', 'Avoid.')
    case 'priority':
      return short(offense, 'Priority', 'Prio %')
    case 'dispels':
      return short(offense, 'Dispels', 'Disp.')
    case 'deaths': {
      const count = (r: KeyMeterRow) => deaths.filter((d) => d.player === r.player).length
      return { label: 'Deaths', value: count, show: (r) => String(count(r)) }
    }
  }
}

/** An overlay column's heading, for the settings' checkboxes. */
export function overlayColumnLabel(id: OverlayColumn): string {
  return overlayColumn(id, { startMS: 0, endMS: 0, ongoing: false, rows: [] }, []).label
}
