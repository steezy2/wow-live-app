import type { KeyCast, KeyDeath, KeyMeter } from './api'
import { MeterCard } from './MeterCards'
import { defense, raidOffense, type Column } from './meterColumns'

/** Each player's fight in one table, sortable by any column: damage, healing, damage taken (avoidable too), active
 * time, combat potions (pre-pots marked), health potions, healthstones and deaths. With onPick, a row focuses them. */
export function PlayerTable(props: {
  title: string
  meter: KeyMeter
  casts: KeyCast[]
  activity?: Record<string, number>
  deaths: KeyDeath[]
  onPick?: (player: string) => void
}) {
  const { title, meter, casts, activity, deaths, onPick } = props
  const used = (kind: KeyCast['kind'], player: string) =>
    casts.filter((c) => c.kind === kind && c.player === player).length
  const prepot = (player: string) => casts.some((c) => c.player === player && c.prepot)
  const count = (label: string, kind: KeyCast['kind']): Column => ({
    label,
    value: (r) => used(kind, r.player),
    show: (r) => String(used(kind, r.player)),
  })
  const columns: Column[] = [
    ...raidOffense.slice(0, 2),
    ...defense(true),
    {
      label: 'Active',
      value: (r) => activity?.[r.player] ?? 0,
      show: (r) => (activity?.[r.player] === undefined ? '–' : `${Math.round(activity[r.player])}%`),
      title: () => 'share of their time alive with a cast going or a hit landing',
    },
    {
      ...count('Combat pots', 'combat potion'),
      show: (r) => `${used('combat potion', r.player)}${prepot(r.player) ? ' (pre)' : ''}`,
      title: (r) => (prepot(r.player) ? 'one drunk before the pull' : ''),
    },
    count('Health pots', 'health potion'),
    count('Healthstones', 'healthstone'),
    {
      label: 'Deaths',
      value: (r) => deaths.filter((d) => d.player === r.player).length,
      show: (r) => String(deaths.filter((d) => d.player === r.player).length),
    },
  ]
  return <MeterCard title={title} meter={meter} columns={columns} onPick={onPick} sortable />
}
