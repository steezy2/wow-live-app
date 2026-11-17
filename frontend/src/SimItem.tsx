import type { ReactNode } from 'react'
import type { SimItem, SimItemInfo } from './api'
import { itemName } from './simItems'

/** An item's icon in a box coloured by its quality; an empty box until the icon is known. */
export function ItemIcon({ info }: { info?: SimItemInfo }) {
  return (
    <span className={`sim-icon q${info?.quality ?? 1}`}>
      {info?.icon && <img src={`/api/icons/${info.icon}`} alt="" loading="lazy" />}
    </span>
  )
}

/** An item's icon, name (in its quality's colour) and item level on one line, with extra after it. */
export function ItemLine({ item, info, extra }: { item: SimItem; info?: SimItemInfo; extra?: ReactNode }) {
  return (
    <span className="sim-item">
      <ItemIcon info={info} />
      <span className={`sim-item-name q${info?.quality ?? 1}`}>{itemName(item, info)}</span>
      {item.level ? <span className="muted small">{item.level}</span> : null}
      {extra}
    </span>
  )
}
