import { useEffect, useState } from 'react'
import { fetchSimItems, type SimItem, type SimItemInfo } from './api'
import { itemIDs } from './simOptions'

/** What Wowhead says of each of items (by ID), once it has answered; an item it doesn't know is missing. */
export function useItemInfo(items: SimItem[]): Map<number, SimItemInfo> {
  const [info, setInfo] = useState<Map<number, SimItemInfo>>(new Map())
  const key = itemIDs(items).join(',')
  useEffect(() => {
    if (!key) return
    const controller = new AbortController()
    fetchSimItems(key.split(',').map(Number), controller.signal).then(
      (found) => setInfo((old) => new Map([...old, ...found.map((i): [number, SimItemInfo] => [i.id, i])])),
      () => undefined, // the page draws an item without its icon
    )
    return () => controller.abort()
  }, [key])
  return info
}

/** An item as the page names it: Wowhead's name, else the export's, else its ID. */
export const itemName = (item: SimItem, info?: SimItemInfo) => info?.name ?? item.name ?? `item ${item.id}`
