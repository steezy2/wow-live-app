import { Fragment } from 'react'
import type { ReadyCheck } from './api'
import { Name } from './Seek'

type ReadyPlayer = ReadyCheck['players'][number]

/** The ready check before a key: who's missing a flask, food or party buff (a row per player), and what the log hasn't
 * shown yet; names in class colours. Compact on the overlay. */
export function ReadyPanel(props: { ready: ReadyCheck; compact?: boolean }) {
  const { ready, compact = false } = props
  const classes = Object.fromEntries(ready.players.map((p) => [p.player, p.class ?? '']))
  const name = (player: string) => <Name player={player} classes={classes} />
  const rows = (pick: (p: ReadyPlayer) => string[]) => {
    const players = ready.players.filter((p) => pick(p).length > 0)
    if (players.length === 0) return null
    return players.map((p, i) => (
      <Fragment key={p.player}>
        {i > 0 && ' · '}
        {name(p.player)} {pick(p).join(', ')}
      </Fragment>
    ))
  }
  const missing = rows((p) => p.missing)
  const notSeen = rows((p) => p.notSeen)
  const all = !missing && !notSeen
  const since = new Date(ready.sinceMS).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  return (
    <section className={compact ? 'small' : 'panel'}>
      {!compact && <h2>Ready check · {ready.dungeon}</h2>}
      {ready.players.length === 0 && <p className="muted">No one in the party in the log yet.</p>}
      {ready.players.length > 0 && all && <p>Everyone has their flask, food and buffs.</p>}
      {missing && <p className="warning">Missing: {missing}</p>}
      {notSeen && <p className="muted">Not seen yet: {notSeen}</p>}
      {!compact && (
        <p className="muted small">
          Checked from the combat log since you zoned in at {since}. Anything drunk or cast before then shows as "not
          seen" until the key starts.
        </p>
      )}
    </section>
  )
}
