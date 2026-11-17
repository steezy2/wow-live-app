import { useEffect, useEffectEvent, useState } from 'react'
import { subscribe } from './api'
import { errorText } from './format'

/** A saved key or pull, read now and again whenever the saved keys and pulls change (a report linked to it, say). The
 * previous one stays in data while another is read; a read that works clears an earlier error. */
export function useSaved<T>(
  start: number,
  fetch: (start: number, signal: AbortSignal) => Promise<T>,
): { data: T | null; error: string | null } {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const read = useEffectEvent(fetch)
  useEffect(() => {
    const controller = new AbortController()
    const off = subscribe(
      () =>
        void read(start, controller.signal).then(
          (got) => {
            setData(got)
            setError(null)
          },
          (err: unknown) => controller.signal.aborted || setError(errorText(err)),
        ),
    )
    return () => {
      controller.abort()
      off()
    }
  }, [start])
  return { data, error }
}
