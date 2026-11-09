import { useEffect, useState } from 'react'
import { fetchLiveKey, fetchReady, moveOverlay, onAppEvent, type ReadyCheck, subscribe, type KeyView } from './api'

/** How often a running clock ticks between answers. */
const TICK_MS = 250

interface Reading {
  key: KeyView | null
  at: number
}

/** The live key, read again on each change the site pushes, and its clock ticking between answers; onEnded runs when
 * a key ends. endedHere is the start of a key that ended while watching. */
export function useLiveKey(onEnded?: () => void) {
  const [reading, setReading] = useState<Reading | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const [endedHere, setEndedHere] = useState<number | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    let status: string | undefined
    const read = () =>
      fetchLiveKey(controller.signal)
        .then((key) => {
          if (status === 'running' && key?.status !== 'running') {
            onEnded?.()
            setEndedHere(key?.start ?? null)
          }
          status = key?.status
          setReading({ key, at: Date.now() })
          setError(null)
        })
        .catch((err: unknown) => {
          if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
        })
    const unsubscribe = subscribe(() => void read())
    return () => {
      controller.abort()
      unsubscribe()
    }
  }, [onEnded])

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), TICK_MS)
    return () => clearInterval(timer)
  }, [])

  const key = reading?.key ?? null
  const clockMS =
    key?.status === 'running' && !key.quiet && reading ? Math.max(key.clockMS, key.clockMS + (now - reading.at)) : (key?.clockMS ?? 0)
  return { loaded: reading !== null, key, clockMS, error, endedHere }
}

/** The ready check before the next key, read again on each change the site pushes; null outside a Mythic dungeon
 * between keys. */
export function useReady(): ReadyCheck | null {
  const [ready, setReady] = useState<ReadyCheck | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    const unsubscribe = subscribe(() => void fetchReady(controller.signal).then(setReady, () => undefined))
    return () => {
      controller.abort()
      unsubscribe()
    }
  }, [])
  return ready
}

/** Whether the desktop app says the overlay is being moved (it takes clicks to be dragged and resized). */
export function useOverlayMoving(): boolean {
  const [moving, setMoving] = useState(false)
  useEffect(() => {
    const off = onAppEvent('overlay-moving', (on) => setMoving(on === true))
    moveOverlay() // it may already be moving (the hotkey), before this page was open
    return off
  }, [])
  return moving
}
