import type { SimItemInfo, SimSwap } from './api'
import { full } from './format'
import { ItemLine } from './SimItem'
import { gainShare, groupLabel } from './simOptions'

/** A ranked row's DPS (when it has one), its gain over the sim's baseline and a bar for the gain against the best's. */
export function Gain({ gain, dps, best }: { gain: number; dps?: number; best: number }) {
  return (
    <div className="sim-gain">
      {dps !== undefined && <span className="muted small">{full(dps)} DPS</span>}
      <span className={gain >= 0 ? 'good' : 'warning'}>
        {gain > 0 ? '+' : ''}
        {gain === 0 ? '—' : full(gain)}
      </span>
      <span className="sim-gain-bar">
        <span style={{ width: `${gainShare(gain, best)}%` }} />
      </span>
    </div>
  )
}

/** The slots a combination changes, a line each: the item taken off, an arrow and the item put on. */
export function SwapRows({ swaps, info }: { swaps: SimSwap[]; info: Map<number, SimItemInfo> }) {
  return swaps.map((s) => (
    <div key={s.slot} className="sim-swap">
      <span className="muted small sim-swap-slot">{groupLabel(s.in?.group ?? s.out?.group ?? s.slot)}</span>
      {s.out ? <ItemLine item={s.out} info={info.get(s.out.id)} /> : <span className="muted">empty</span>}
      <span className="muted">→</span>
      {s.in ? <ItemLine item={s.in} info={info.get(s.in.id)} /> : <span className="muted">empty</span>}
    </div>
  ))
}
