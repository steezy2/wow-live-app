import type { ReadyCheck } from './api'

/** The ready check before a key: who's missing a flask, food or party buff, and what the log hasn't shown yet.
 * Compact on the overlay. */
export function ReadyPanel({ ready, compact = false }: { ready: ReadyCheck; compact?: boolean }) {
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
      {ready.players.length > 0 && !missing && !notSeen && <p>Everyone has their flask, food and party buffs.</p>}
      {missing && <p className="warning">Missing: {missing}</p>}
      {notSeen && <p className="muted">Not seen yet: {notSeen}</p>}
      {!compact && (
        <p className="muted small">
          From the combat log since you zoned in at {since}: a flask or food from before then shows as not seen. The
          key's start checks everyone again.
        </p>
      )}
    </section>
  )
}
