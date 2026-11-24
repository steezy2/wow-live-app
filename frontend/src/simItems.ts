import { useEffect, useState } from 'react'
import { fetchSimItems, fetchSimSpells, type SimItem, type SimItemInfo, type SpellInfo } from './api'
import { itemIDs } from './simOptions'

/** What the server's lookup says of each of ids, asked chunk IDs at a time (SimChoices.maxItemLookups), as the answers
 * come; an ID it doesn't know is missing. */
function useLookups<T extends { id: number }>(
  ids: number[],
  chunk: number,
  get: (ids: number[], signal: AbortSignal) => Promise<T[]>,
): Map<number, T> {
  const [info, setInfo] = useState<Map<number, T>>(new Map())
  const key = ids.join(',')
  useEffect(() => {
    if (!key) return
    const controller = new AbortController()
    const asked = key.split(',').map(Number)
    for (let i = 0; i < asked.length; i += chunk)
      get(asked.slice(i, i + chunk), controller.signal).then(
        (found) => setInfo((old) => new Map([...old, ...found.map((f): [number, T] => [f.id, f])])),
        () => undefined, // the page draws a name without its icon
      )
    return () => controller.abort()
  }, [key, chunk, get])
  return info
}

/** What Wowhead says of each of items (by ID). */
export const useItemInfo = (items: SimItem[], chunk: number): Map<number, SimItemInfo> =>
  useLookups(itemIDs(items), chunk, fetchSimItems)

/** What Wowhead says of each of spells (by ID): icon, cooldown and text. */
export const useSpellInfo = (ids: number[], chunk: number): Map<number, SpellInfo> =>
  useLookups(ids, chunk, fetchSimSpells)
