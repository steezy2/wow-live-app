import { useEffect, useState } from 'react'
import {
  fetchLiveKey,
  fetchRaidLive,
  moveOverlay,
  onAppEvent,
  type OverlayPreview,
  previewOverlay,
  type RaidLive,
  subscribe,
  type KeyView,
} from './api'

import { errorText } from './format'

/** How often a running clock ticks between answers. */
const TICK_MS = 250

interface Reading {
  key: KeyView | null
  at: number
}

/** The live key, read again on each change the site pushes, and its clock ticking between answers; onEnded runs when
 * a key ends. endedHere is the start of a key that ended while watching, and silentMS how long ago the game last
 * wrote a running key's log. */
export function useLiveKey(onEnded?: () => void) {
  const [reading, setReading] = useState<Reading | null>(null)
  const [error, setError] = useState<string | null>(null)
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
          if (!controller.signal.aborted) setError(errorText(err))
        })
    const unsubscribe = subscribe(() => void read())
    return () => {
      controller.abort()
      unsubscribe()
    }
  }, [onEnded])

  const key = reading?.key ?? null
  const now = useNow(key?.status === 'running' ? TICK_MS : 0) // no clock to tick without a running key
  const since = key?.status === 'running' && reading ? Math.max(0, now - reading.at) : 0
  return {
    loaded: reading !== null,
    key,
    clockMS: (key?.clockMS ?? 0) + since,
    silentMS: (key?.silentMS ?? 0) + since,
    error,
    endedHere,
  }
}

/** One clock per tick length, shared by every component that reads it and stopped while none does. */
const clocks = new Map<number, { listeners: Set<() => void>; stop?: () => void }>()

/** Joins the tickMS clock (started by the first to join); returns what leaves it. A clock reads again at once when
 * the page shows after being hidden, and skips its ticks while it is hidden. */
export function joinClock(tickMS: number, onTick: () => void): () => void {
  let clock = clocks.get(tickMS)
  if (!clock) {
    clock = { listeners: new Set() }
    clocks.set(tickMS, clock)
  }
  const shared = clock
  const tick = () => shared.listeners.forEach((listener) => listener())
  if (shared.listeners.size === 0) {
    const timer = setInterval(() => document.hidden || tick(), tickMS)
    const shown = () => document.hidden || tick()
    document.addEventListener('visibilitychange', shown)
    shared.stop = () => {
      clearInterval(timer)
      document.removeEventListener('visibilitychange', shown)
    }
  }
  shared.listeners.add(onTick)
  return () => {
    shared.listeners.delete(onTick)
    if (shared.listeners.size === 0) {
      shared.stop?.()
      clocks.delete(tickMS)
    }
  }
}

/** The time now, read again every tickMS (never, for 0). */
export function useNow(tickMS: number): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => (tickMS ? joinClock(tickMS, () => setNow(Date.now())) : undefined), [tickMS])
  return now
}

/** The raid now, read again on each change the site pushes while enabled (no raid pull runs during a key, so the
 * overlay skips it then); null until the first answer. */
export function useRaidLive(enabled = true): RaidLive | null {
  return usePushed(fetchRaidLive, enabled)
}

/** read's answer, asked again on each change the site pushes while enabled; null until the first answer. */
function usePushed<T>(read: (signal: AbortSignal) => Promise<T | null>, enabled = true): T | null {
  const [value, setValue] = useState<T | null>(null)
  useEffect(() => {
    if (!enabled) return
    const controller = new AbortController()
    const unsubscribe = subscribe(() => void read(controller.signal).then(setValue, () => undefined))
    return () => {
      controller.abort()
      unsubscribe()
      setValue(null)
    }
  }, [read, enabled])
  return value
}

/** What the desktop app says the overlay shows dummy data of (none for the real thing). */
export function useOverlayPreview(): OverlayPreview | undefined {
  const [kind, setKind] = useState<OverlayPreview>()
  useEffect(() => {
    const off = onAppEvent('overlay-previewing', (k) => setKind(k === 'mplus' || k === 'raid' ? k : undefined))
    previewOverlay() // it may already be on, before this page was open
    return off
  }, [])
  return kind
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
