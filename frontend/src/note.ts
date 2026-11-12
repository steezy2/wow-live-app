import { useCallback, useEffect, useSyncExternalStore } from 'react'

/** A settings section's state after its last action: busy (what's happening), done, or the error. */
export type Note = { busy?: string; done?: string; error?: string } | null

/** The notes by section, kept outside the page: a call still running when Settings is left goes on, and shows again
 * (busy, then done or its error) on the way back. */
const notes = new Map<string, Note>()
const listeners = new Set<() => void>()

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** A section's note by its name, and a setter: like useState, but a call still running outlives the page (a finished
 * one's note is cleared when the section shows again). */
export function useNote(section: string): [Note, (n: Note) => void] {
  const note = useSyncExternalStore(subscribe, () => notes.get(section) ?? null)
  const set = useCallback(
    (n: Note) => {
      notes.set(section, n)
      listeners.forEach((l) => l())
    },
    [section],
  )
  useEffect(() => {
    if (!notes.get(section)?.busy) set(null)
  }, [section, set])
  return [note, set]
}

/** Runs a call with a note: busy while it runs, then done (from its answer) or the error. The returned promise fails
 * like the call, with the note already showing why. */
export function track<T>(call: Promise<T>, setNote: (n: Note) => void, busy: string, done: (v: T) => string) {
  setNote({ busy })
  return call.then(
    (v) => {
      setNote({ done: done(v) })
      return v
    },
    (err: unknown) => {
      setNote({ error: err instanceof Error ? err.message : String(err) })
      throw err
    },
  )
}
