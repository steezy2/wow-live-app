import { useCallback, useEffect, useSyncExternalStore } from 'react'

/** A settings section's state after its last action: busy (what's happening), done, or the error. */
export type Note = { busy?: string; done?: string; error?: string } | null

/** The notes by section, kept outside the page: a call still running when Settings is left goes on, and shows again
 * (busy, then done or its error) on the way back. */
const notes = new Map<string, Note>()
const listeners = new Set<() => void>()
const open = new Set<string>() // the sections on screen
const seen = new Set<string>() // the sections whose note has been on screen

const subscribe = (listener: () => void) => {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

/** A section's note. */
export const noteOf = (section: string): Note => notes.get(section) ?? null

/** Sets a section's note; one set while the section is away stays until it has been shown. */
export function setNote(section: string, n: Note) {
  notes.set(section, n)
  if (open.has(section)) seen.add(section)
  else seen.delete(section)
  listeners.forEach((l) => l())
}

/** Marks a section on screen: a finished note it has already shown is cleared, and returns its close. The note
 * counts as shown a tick later, so StrictMode's second mount still shows it. */
export function openSection(section: string): () => void {
  open.add(section)
  if (!notes.get(section)?.busy && seen.has(section)) setNote(section, null)
  const shown = setTimeout(() => seen.add(section))
  return () => {
    clearTimeout(shown)
    open.delete(section)
  }
}

/** A section's note by its name, and a setter: like useState, but a call still running outlives the page, and a
 * note that finished while the section was away shows once on the way back. */
export function useNote(section: string): [Note, (n: Note) => void] {
  const note = useSyncExternalStore(subscribe, () => noteOf(section))
  const set = useCallback((n: Note) => setNote(section, n), [section])
  useEffect(() => openSection(section), [section])
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
