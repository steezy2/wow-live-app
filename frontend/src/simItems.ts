import { useEffect, useState } from 'react'
import { fetchSimItems, type SimItem, type SimItemInfo } from './api'
import { itemIDs } from './simOptions'

/** What Wowhead says of each of items (by ID), asked chunk IDs at a time (SimChoices.maxItemLookups), as the answers
 * come; an item it doesn't know is missing. */
export function useItemInfo(items: SimItem[], chunk: number): Map<number, SimItemInfo> {
  const [info, setInfo] = useState<Map<number, SimItemInfo>>(new Map())
  const key = itemIDs(items).join(',')
  useEffect(() => {
    if (!key) return
    const controller = new AbortController()
    const ids = key.split(',').map(Number)
    for (let i = 0; i < ids.length; i += chunk)
      fetchSimItems(ids.slice(i, i + chunk), controller.signal).then(
        (found) => setInfo((old) => new Map([...old, ...found.map((f): [number, SimItemInfo] => [f.id, f])])),
        () => undefined, // the page draws an item without its icon
      )
    return () => controller.abort()
  }, [key, chunk])
  return info
}
