import type { BuffGap, KeyMeterRow, KeyTrack } from './api'
import { buffUptimes, cooldownLanes } from './events'
import { amount, coverOf } from './format'
import type { FightData } from './KeyTabs'
import { RaidLanes } from './RaidReplay'
import { Cover, Spell } from './Spell'
import { When, type Seeker } from './Seek'

/** How many of a player's buffs the uptime list shows, longest up first. */
const BUFF_ROWS = 12

/** One player's page of the full review: their numbers as tiles, the cooldowns they pressed on the fight's clock, the
 * damage spikes that hit them and what covered them, their buff uptimes and what they came prepared with. Their
 * damage, healing, damage taken and targets by ability are the meters' drill-in (Overview), opened for them. */
export function PlayerReview(props: {
  player: string
  row?: KeyMeterRow
  data: FightData
  track: KeyTrack | null | undefined
  buffGaps: BuffGap[]
  lusts: number[]
  durationMS: number
  seek: Seeker
  /** The replay's moment, drawn on the lanes. */
  now: number
}) {
  const { player, row, data, track, buffGaps, lusts, durationMS, seek, now } = props
  const mine = <T extends { player: string }>(list: T[]) => list.filter((x) => x.player === player)
  const potions = mine(data.casts).filter((c) => c.kind === 'combat potion').length
  const lacked = [...new Set(buffGaps.filter((g) => g.player === player && g.fromMS === 0).map((g) => g.buff))]
  const tiles: [string, string][] = [
    [amount(row?.damage ?? 0), 'damage'],
    [amount(row?.healing ?? 0), 'healing'],
    [amount(row?.damageTaken ?? 0), 'damage taken'],
    [amount(row?.avoidable ?? 0), 'avoidable damage taken'],
    [String(mine(data.deaths).length), 'deaths'],
    [String(mine(data.interrupts).length), 'kicks'],
    [String(mine(data.casts).filter((c) => c.kind === 'defensive').length), 'defensives used'],
    [String(row?.dispels ?? 0), 'dispels'],
  ]
  const lanes = track?.casts?.[player] ? cooldownLanes(track.casts[player], durationMS) : null
  const buffs = track?.buffs?.[player] ? buffUptimes(track.buffs[player], durationMS) : null
  const spikes = data.spikes.filter((s) => s.members.some((m) => m.player === player))
  return (
    <>
      <div className="recap-tiles">
        {tiles.map(([value, label]) => (
          <div key={label} className="card small">
            <b>{value}</b>
            <span className="muted small">{label}</span>
          </div>
        ))}
      </div>
      <section className="panel">
        <h2>Cooldowns</h2>
        {lanes === null ? (
          <p className="muted">Not recorded for this fight: <i>Read old logs</i> fills it in.</p>
        ) : lanes.length === 0 ? (
          <p className="muted">Nothing cast at most once a minute.</p>
        ) : (
          <RaidLanes lanes={lanes} end={durationMS} now={now} onSeek={seek.onSeek} />
        )}
      </section>
      <section className="panel">
        <h2>Spikes that hit them</h2>
        <ul className="list">
          {spikes.map((s) => {
            const m = s.members.find((x) => x.player === player)!
            return (
              <li key={s.clockMS}>
                <span>
                  <When ms={s.clockMS} seek={seek} /> {amount(m.taken)} of {amount(s.damage)}:{' '}
                  <Cover {...coverOf(m)} />
                </span>
              </li>
            )
          })}
          {spikes.length === 0 && <li className="muted">None</li>}
        </ul>
      </section>
      <section className="panel">
        <h2>Buff uptime</h2>
        {buffs === null ? (
          <p className="muted">Not recorded for this fight: <i>Read old logs</i> fills it in.</p>
        ) : (
          <ul className="list">
            {buffs.slice(0, BUFF_ROWS).map((b) => (
              <li key={b.id}>
                <Spell name={b.name} />
                <span className="muted">{b.pct.toFixed(0)}%</span>
              </li>
            ))}
            {buffs.length === 0 && <li className="muted">None</li>}
          </ul>
        )}
      </section>
      <section className="panel">
        <h2>Preparation</h2>
        <p>
          {lacked.length > 0 ? <span className="bad">Missing at the start: {lacked.join(', ')}</span> : 'Flask, food and raid buffs were up.'}
          {' · '}
          {potions} combat {potions === 1 ? 'potion' : 'potions'}
          {lusts.length > 0 && ` for ${lusts.length} Bloodlust${lusts.length === 1 ? '' : 's'}`}
        </p>
      </section>
    </>
  )
}
