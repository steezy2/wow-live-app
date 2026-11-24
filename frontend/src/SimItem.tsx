import type { ReactNode } from 'react'
import { iconURL, type SimGem, type SimItem, type SimItemInfo } from './api'
import { tip } from './tipAttrs'

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

/** An item's sockets, one box each: the gem's icon in a filled one, a dark ring in an empty one. */
export function Sockets({ item, gems }: { item: SimItem; gems: SimGem[] }) {
  if (!item.sockets) return null
  return (
    <span className="sim-sockets">
      {Array.from({ length: item.sockets }, (_, i) => {
        const id = item.opts?.gem_id?.[i]
        const gem = gems.find((g) => g.id === id)
        const text = gem ? `${gem.name}${gem.stats ? ` · ${gem.stats}` : ''}` : id ? `Gem ${id}` : 'Empty socket'
        return (
          <span key={i} className={id ? 'sim-socket filled' : 'sim-socket'} {...tip(text, `Socket ${i + 1}`)}>
            {gem && <img src={iconURL(gem.icon)} alt="" loading="lazy" />}
          </span>
        )
      })}
    </span>
  )
}
