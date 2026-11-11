import type { ReadyCheck } from './api'

/** The ready check before a key or a raid pull: who's missing a flask, food (in a raid, a Vantus Rune) or party
 * buff, and what the log hasn't shown yet. Compact on the overlay. */
export function ReadyPanel(props: { ready: ReadyCheck; compact?: boolean; raid?: boolean }) {
  const { ready, compact = false, raid = false } = props
  const list = (pick: (p: ReadyCheck['players'][number]) => string[]) =>
    ready.players
      .filter((p) => pick(p).length > 0)
      .map((p) => `${p.player} ${pick(p).join(', ')}`)
      .join(' · ')
  const missing = list((p) => p.missing)
  const notSeen = list((p) => p.notSeen)
  const since = new Date(ready.sinceMS).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
  return (
    <section className={compact ? 'small' : 'panel'}>
      {!compact && <h2>Ready check · {ready.dungeon}</h2>}
      {ready.players.length === 0 && <p className="muted">No one in the party in the log yet.</p>}
      {ready.players.length > 0 && !missing && !notSeen && <p>Everyone has their flask, food and buffs.</p>}
      {missing && <p className="warning">Missing: {missing}</p>}
      {notSeen && <p className="muted">Not seen yet: {notSeen}</p>}
      {!compact && (
        <p className="muted small">
          From the combat log since you zoned in at {since}{raid && " and the last pull's start"}: a flask or food
          from before then shows as not seen. The next start checks everyone again.
        </p>
      )}
    </section>
  )
}
