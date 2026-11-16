import { useEffect, useEffectEvent, useState } from 'react'
import { errorText } from './format'

/** What read gave for the request named by key, or why it failed; empty until then, while another key is read (so an
 * old answer never shows for a new request) and with a null key, which reads nothing. A request still running when
 * the key changes is aborted. */
export function useFetched<T>(
  key: string | null,
  read: (signal: AbortSignal, key: string) => Promise<T>,
): { data?: T; error?: string } {
  const [answer, setAnswer] = useState<{ key: string; data?: T; error?: string }>()
  const start = useEffectEvent(read)
  useEffect(() => {
    if (key === null) return
    const controller = new AbortController()
    start(controller.signal, key).then(
      (data) => setAnswer({ key, data }),
      (err: unknown) => controller.signal.aborted || setAnswer({ key, error: errorText(err) }),
    )
    return () => controller.abort()
  }, [key])
  return answer?.key === key ? answer : {}
}
