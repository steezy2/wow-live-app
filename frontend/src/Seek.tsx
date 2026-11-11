import { classColors } from './classColors'
import { clock } from './format'
import type { PlayerEvent } from './events'

/** Each player's class, by name. */
export type Classes = Record<string, string>

/** What a ledger needs to link a row to the replay: move it to a time, colour names and focus a player. */
export interface Seeker {
  onSeek: (clockMS: number) => void
  classes: Classes
  onFocus: (player: string) => void
}

/** A player's name in their class colour; with onPick, a button that focuses them. */
export function Name({ player, classes, onPick }: { player: string; classes: Classes; onPick?: (p: string) => void }) {
  const style = { color: classColors[classes[player]] }
  return onPick ? (
    <button className="name pick" style={style} onClick={() => onPick(player)} title={`Focus ${player}`}>
      {player}
    </button>
  ) : (
    <b className="name" style={style}>
      {player}
    </b>
  )
}

/** A time on the replay's clock; a click moves the replay there. */
export function At({ ms, onSeek }: { ms: number; onSeek: (clockMS: number) => void }) {
  return (
    <button className="at muted" onClick={() => onSeek(ms)} title="Show this moment in the replay">
      {clock(ms)}
    </button>
  )
}

/** A time that moves the replay when the page has one (seek), else plain. */
export function When({ ms, seek }: { ms: number; seek?: Seeker }) {
  return seek ? <At ms={ms} onSeek={seek.onSeek} /> : <>{clock(ms)}</>
}

/** A player's name that focuses them when the page has a replay (seek), else bold. */
export function Who({ player, seek }: { player: string; seek?: Seeker }) {
  return seek ? <Name player={player} classes={seek.classes} onPick={seek.onFocus} /> : <b>{player}</b>
}

/** A player's moments in order, each time linked to the replay. */
export function EventList({ events, onSeek }: { events: PlayerEvent[]; onSeek: (clockMS: number) => void }) {
  if (events.length === 0) return null
  return (
    <ul className="list events">
      {events.map((e, i) => (
        <li key={i}>
          <span>
            <At ms={e.clockMS} onSeek={onSeek} /> <span className={e.bad ? 'bad' : ''}>{e.text}</span>
          </span>
        </li>
      ))}
    </ul>
  )
}
