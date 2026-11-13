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

const NAME_PX = 96
const NUMBER_PX = 64
const GAP_PX = 8 // --s2
/** The overlay's closer columns. */
const DENSE_NAME_PX = 72
const DENSE_NUMBER_PX = 44
const DENSE_GAP_PX = 4 // --s1

/** A card's grid: the name column and a number column each, with the card's least width (dense for the overlay). */
export function cardGrid(columns: number, dense: boolean) {
  const [name, number, gap] = dense ? [DENSE_NAME_PX, DENSE_NUMBER_PX, DENSE_GAP_PX] : [NAME_PX, NUMBER_PX, GAP_PX]
  return {
    grid: { gridTemplateColumns: `minmax(${name}px, 1fr) repeat(${columns}, ${number}px)` },
    minWidth: name + columns * (number + gap),
  }
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

/** Each overlay column's short heading, by its settings id (models.OverlayColumns). */
export const OVERLAY_LABELS: Record<OverlayColumn, string> = {
  dps: 'DPS',
  hps: 'HPS',
  kicks: 'Kicks',
  pullDps: 'Pull DPS',
  taken: 'Taken',
  avoidable: 'Avoid.',
  priority: 'Prio %',
  dispels: 'Dispels',
  deaths: 'Deaths',
}

/** The columns an overlay card draws, in order: Pull DPS leads the one-card meter and is left out of the two cards,
 * where it would repeat DPS. The saved choice keeps it, so turning one card back on restores it. */
export function overlayIds(chosen: OverlayColumn[], oneCard: boolean): OverlayColumn[] {
  const rest = chosen.filter((id) => id !== 'pullDps')
  return oneCard ? ['pullDps', ...rest] : rest
}

/** An overlay card's column by its settings id: pull is the last pull, for the Pull DPS column, and deaths the deaths
 * inside the card's span, for the Deaths column. The others are the page's own columns (label in columns), under the
 * overlay's heading and without a group rule. */
export function overlayColumn(id: OverlayColumn, pull: KeyMeter, deaths: KeyDeath[]): Column {
  const short = (columns: Column[], label: string): Column => ({
    ...columns.find((c) => c.label === label)!,
    label: OVERLAY_LABELS[id],
    group: false,
  })
  switch (id) {
    case 'dps':
      return short(offense, 'DPS')
    case 'hps':
      return short(defense(false), 'HPS')
    case 'kicks':
      return short(offense, 'Kicks')
    case 'pullDps': {
      const byPlayer = new Map(pull.rows.map((r) => [r.player, r.dps]))
      return { label: OVERLAY_LABELS[id], value: (r) => byPlayer.get(r.player) ?? 0 }
    }
    case 'taken':
      return short(defense(false), 'Taken')
    case 'avoidable':
      return short(defense(true), 'Avoidable')
    case 'priority':
      return short(offense, 'Priority')
    case 'dispels':
      return short(offense, 'Dispels')
    case 'deaths': {
      const count = (r: KeyMeterRow) => deaths.filter((d) => d.player === r.player).length
      return { label: OVERLAY_LABELS[id], value: count, show: (r) => String(count(r)) }
    }
  }
}
