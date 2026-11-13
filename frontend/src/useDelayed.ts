import { useEffect, useState } from 'react'

/** How long something must stay loading before a placeholder shows, so a quick load never flashes one. */
export const LOADING_DELAY_MS = 250

/** Sets the flag at once when it is false, and only after delayMS when it is true; returns what stops the wait. */
export function delayFlag(set: (shown: boolean) => void, flag: boolean, delayMS: number): () => void {
  if (!flag) {
    set(false)
    return () => undefined
  }
  const timer = setTimeout(() => set(true), delayMS)
  return () => clearTimeout(timer)
}

/** flag, but turning true only after it has stayed true for delayMS (turning false is immediate). */
export function useDelayed(flag: boolean, delayMS = LOADING_DELAY_MS): boolean {
  const [shown, setShown] = useState(false)
  useEffect(() => delayFlag(setShown, flag, delayMS), [flag, delayMS])
  return shown
}
