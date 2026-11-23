import type { ReactNode } from 'react'
import { iconURL, type SimItem, type SimItemInfo } from './api'

/** An item's icon in a box coloured by its quality (empty until the icon is known), its name (Wowhead's, else the
 * export's, else its ID) in its quality's colour and its item level on one line, with extra after it. */
export function ItemLine({ item, info, extra }: { item: SimItem; info?: SimItemInfo; extra?: ReactNode }) {
  const quality = `q${info?.quality ?? 1}`
  return (
    <span className="sim-item">
      <span className={`sim-icon ${quality}`}>
        {info?.icon && <img src={iconURL(info.icon)} alt="" loading="lazy" />}
      </span>
      <span className={`sim-item-name ${quality}`}>{info?.name ?? item.name ?? `item ${item.id}`}</span>
      {item.level ? <span className="muted small">{item.level}</span> : null}
      {item.note && <span className="sim-note">{item.note}</span>}
      {extra}
    </span>
  )
}
