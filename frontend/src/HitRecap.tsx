import { type CSSProperties, type ReactNode, useState } from 'react'
import type { HitRecap, RecapEvent } from './api'
import { amount, seconds } from './format'
import { healthAfter } from './replay'
import { Spell, Spells } from './Spell'

/** How long a recap covers: from its first event to its end (the backend's recap_seconds at most). */
const spanOf = (recap: HitRecap) => (recap.events.length > 0 ? recap.clockMS - recap.events[0].t : 0)

const sum = (events: RecapEvent[], kind: RecapEvent['k']) =>
  events.filter((e) => e.k === kind).reduce((n, e) => n + (e.n ?? 0), 0)

/** One recap: a summary (taken, healed, shielded, buttons used, and what was ready) and the last events, newest at
 * the bottom with the time before the end, each filled green to their health after it (worked out from the damage and healing where the log gives none); limit keeps the newest ones. */
export function HitRecapView(props: { recap: HitRecap; ready?: string[]; limit?: number }) {
  const { recap, ready, limit } = props
  const health = healthAfter(recap)
  const events = limit ? recap.events.slice(-limit) : recap.events
  const offset = recap.events.length - events.length
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
        {summary.join(' · ')} in {seconds(spanOf(recap))}
        {used.length > 0 && (
          <>
            {' · used '}
            <Spells names={used} />
          </>
        )}
        {ready && ready.length > 0 && (
          <>
            {' · ready '}
            <Spells names={ready} />
          </>
        )}
      </p>
      <ul className="recap-rows">
        {events.map((e, i) => {
          const hp = health[offset + i]
          return (
          <li
            key={i}
            className={e.k === 'damage' ? 'bad' : e.k === 'cast' ? 'cast' : 'good'}
            style={hp.pct === undefined ? undefined : ({ '--hp': `${hp.pct}%` } as CSSProperties)}
          >
            <span className="muted">-{seconds(recap.clockMS - e.t)}</span>
            <span>
              <Spell name={e.a} />
              {e.s && ` (${e.s})`}
            </span>
            <span>{e.n ? `${e.k === 'damage' ? '−' : '+'}${amount(e.n)}` : ''}</span>
            <span className="muted">{hp.pct === undefined ? '' : `${hp.guess ? '~' : ''}${Math.round(hp.pct)}%`}</span>
          </li>
          )
        })}
      </ul>
    </div>
  )
}

/** A list row that expands to its recap when it has one (a click anywhere but on a name or time, or the arrow); without
 * one it is a plain row. */
export function RecapRow(props: { children: ReactNode; recap?: HitRecap; ready?: string[] }) {
  const { children, recap, ready } = props
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
      {open && <HitRecapView recap={recap} ready={ready} />}
    </li>
  )
}
