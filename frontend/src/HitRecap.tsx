import { type ReactNode, useState } from 'react'
import type { HitRecap, KeyTrack, RecapEvent } from './api'
import { amount, seconds } from './format'

/** The player's first recap at or after fromMS on the key's clock: a death's (its clockMS) or a big hit's (the start of
 * its stretch, since its recap ends about 3 s later). Undefined when the track has none. */
// The recap and its lookup live together; a function here doesn't break fast refresh in practice.
// eslint-disable-next-line react-refresh/only-export-components
export function recapOf(track: KeyTrack | null | undefined, player: string, fromMS: number): HitRecap | undefined {
  return track?.recaps?.find((r) => r.player === player && r.clockMS >= fromMS)
}

/** The recap's length. */
const RECAP_MS = 10_000
const WIDTH = 120
const HEIGHT = 24

/** Their health over the recap from the events that know it (melee swings don't); a death ends at 0. Nothing without
 * a known point. */
function HealthLine({ recap, died }: { recap: HitRecap; died?: boolean }) {
  const start = recap.clockMS - RECAP_MS
  const known = recap.events.filter((e) => e.hp)
  if (known.length === 0) return null
  const point = (t: number, hp: number) =>
    `${(((t - start) / RECAP_MS) * WIDTH).toFixed(1)},${(HEIGHT - (hp / 100) * HEIGHT).toFixed(1)}`
  const points = known.map((e) => point(e.t, e.hp!))
  if (died) points.push(point(recap.clockMS, 0))
  return (
    <svg className="recap-hp" viewBox={`0 0 ${WIDTH} ${HEIGHT}`} role="img" aria-label="Health over the last 10 s">
      <polyline points={points.join(' ')} fill="none" stroke="var(--bad)" strokeWidth="1.5" />
    </svg>
  )
}

const sum = (events: RecapEvent[], kind: RecapEvent['k']) =>
  events.filter((e) => e.k === kind).reduce((n, e) => n + (e.n ?? 0), 0)

/** One recap: a summary (taken, healed, shielded, buttons used, and what was ready), their health line, and the last
 * events, newest at the bottom with the time before the end; limit keeps the newest ones. */
export function HitRecapView(props: { recap: HitRecap; ready?: string[]; died?: boolean; limit?: number }) {
  const { recap, ready, died, limit } = props
  const events = limit ? recap.events.slice(-limit) : recap.events
  const used = [...new Set(recap.events.filter((e) => e.k === 'cast').map((e) => e.a))]
  const healed = sum(recap.events, 'heal')
  const shields = sum(recap.events, 'absorb')
  const summary = [
    `Took ${amount(sum(recap.events, 'damage'))}`,
    healed > 0 && `healed ${amount(healed)}`,
    shields > 0 && `shields ${amount(shields)}`,
  ].filter(Boolean)
  return (
    <div className="recap">
      <p className="small">
        {summary.join(' · ')} in 10 s{used.length > 0 && ` · used ${used.join(', ')}`}
        {ready && ready.length > 0 && ` · ready ${ready.join(', ')}`}
      </p>
      <HealthLine recap={recap} died={died} />
      <ul className="recap-rows">
        {events.map((e, i) => (
          <li key={i} className={e.k === 'damage' ? 'bad' : e.k === 'cast' ? 'cast' : 'good'}>
            <span className="muted">-{seconds(recap.clockMS - e.t)}</span>
            <span>
              {e.a}
              {e.s && ` (${e.s})`}
            </span>
            <span>{e.n ? `${e.k === 'damage' ? '−' : '+'}${amount(e.n)}` : ''}</span>
            <span className="muted">{e.hp ? `${Math.round(e.hp)}%` : ''}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** A list row that expands to its recap when it has one (a click anywhere but on a name or time, or the arrow); without
 * one it is a plain row. */
export function RecapRow(props: { children: ReactNode; recap?: HitRecap; ready?: string[]; died?: boolean }) {
  const { children, recap, ready, died } = props
  const [open, setOpen] = useState(false)
  if (!recap) return <li className="stack">{children}</li>
  return (
    <li
      className="stack recap-row"
      onClick={(e) => !(e.target as Element).closest('button') && setOpen(!open)}
    >
      <button className="back" aria-expanded={open} aria-label="Recap" onClick={() => setOpen(!open)}>
        {open ? '▾' : '▸'}
      </button>
      {children}
      {open && <HitRecapView recap={recap} ready={ready} died={died} />}
    </li>
  )
}
