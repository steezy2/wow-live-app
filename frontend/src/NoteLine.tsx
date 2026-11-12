import type { Note } from './note'

/** A settings section's note, beside its controls: a loading bar and what's happening, that it's done, or the error. */
export function NoteLine({ note }: { note: Note }) {
  if (!note) return null
  if (note.error) return <p className="warning">{note.error}</p>
  if (!note.busy) return <p className="good">{note.done}</p>
  return (
    <>
      <div className="busy-bar" role="progressbar" aria-label={note.busy} />
      <p className="muted">{note.busy}</p>
    </>
  )
}
