/** A settings section's state after its last action: busy (what's happening), done, or the error. */
export type Note = { busy?: string; done?: string; error?: string } | null

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
