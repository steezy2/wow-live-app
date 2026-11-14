import { useState } from 'react'
import { eventKinds, fightEvents, type EventKind } from './events'
import { When, Who, type Seeker } from './Seek'
import type { FightData } from './KeyTabs'

/** The fight's feed on its clock: deaths, defensives, potions, interrupts, crowd control and dispels, with a filter by
 * kind and by player; a time moves the replay when the page has one (seek). */
export function EventFeed({ data, seek }: { data: FightData; seek?: Seeker }) {
  const events = fightEvents(data)
  const [kinds, setKinds] = useState<Set<EventKind>>(new Set(eventKinds))
  const [player, setPlayer] = useState('')
  const players = [...new Set(events.map((e) => e.player))].sort()
  const shown = events.filter((e) => kinds.has(e.kind) && (!player || e.player === player))
  const toggle = (kind: EventKind) => {
    const next = new Set(kinds)
    if (!next.delete(kind)) next.add(kind)
    setKinds(next)
  }
  return (
    <>
      <div className="feed-filters">
        {eventKinds.map((k) => (
          <label key={k} className="small">
            <input type="checkbox" checked={kinds.has(k)} onChange={() => toggle(k)} /> {k}
          </label>
        ))}
        <select aria-label="Player" value={player} onChange={(e) => setPlayer(e.target.value)}>
          <option value="">Everyone</option>
          {players.map((p) => (
            <option key={p}>{p}</option>
          ))}
        </select>
      </div>
      <ul className="list scroll">
        {shown.map((e, i) => (
          <li key={i}>
            <span>
              <Who player={e.player} seek={seek} /> <span className="muted">{e.kind}</span> {e.text}
            </span>
            <span className="muted">
              <When ms={e.clockMS} seek={seek} />
            </span>
          </li>
        ))}
        {shown.length === 0 && <li className="muted">None</li>}
      </ul>
    </>
  )
}
