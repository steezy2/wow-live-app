import type { Note } from './note'

/** A settings section's note, beside its controls: what's happening, that it's done, or the error. */
export function NoteLine({ note }: { note: Note }) {
  if (!note) return null
  if (note.error) return <p className="warning">{note.error}</p>
  return <p className={note.busy ? 'muted' : 'good'}>{note.busy ?? note.done}</p>
}
