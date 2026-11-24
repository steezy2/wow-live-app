import { iconURL, type SimEnhancements, type SimReference } from './api'
import { enchantsFor, groupLabel } from './simOptions'
import { tip } from './tipAttrs'

/** The slots (a ring's is "finger") the reference has enchants for, and a slot name enchantsFor takes for each. */
function enchantSlots(reference: SimReference): [string, string][] {
  const keys = new Map<string, string>()
  for (const e of reference.enchants) for (const s of e.slots) keys.set(s.replace(/[12]$/, ''), s)
  return [...keys]
}

/** The Enhancements card of Top Gear: enchants for each slot and gems to try on every ticked item that can take them,
 * each beside the item as it is. An item with no socket takes no gem, and a unique gem goes in one socket only. */
export function EnhancementsCard(props: {
  reference: SimReference
  value: SimEnhancements
  onChange: (v: SimEnhancements) => void
}) {
  const { reference, value, onChange } = props
  const toggle = <T,>(list: T[] | undefined, v: T) =>
    list?.includes(v) ? list.filter((x) => x !== v) : [...(list ?? []), v]
  const picked = Object.values(value.enchants ?? {}).reduce((n, l) => n + l.length, 0) + (value.gems?.length ?? 0)
  return (
    <section className="panel sim-consumables">
      <h2>Enchants and gems {picked > 0 && <span className="muted small">{picked} picked</span>}</h2>
      <p className="muted small">
        Pick enchants for a slot and gems to try on the items you tick below: each is sim&apos;d as a changed copy of
        the item, beside the item as it is. A unique gem is only ever on one item.
      </p>
      {enchantSlots(reference).map(([key, slot]) => (
        <div key={key} className="sim-consumable-kind">
          <h3>{groupLabel(key)}</h3>
          <div className="sim-omnium-node">
            {enchantsFor(reference, slot).map((e) => (
              <button
                key={e.id}
                className={value.enchants?.[key]?.includes(e.id) ? 'sim-rune picked' : 'sim-rune'}
                aria-pressed={!!value.enchants?.[key]?.includes(e.id)}
                onClick={() =>
                  onChange({ ...value, enchants: { ...value.enchants, [key]: toggle(value.enchants?.[key], e.id) } })
                }
                {...tip(e.stats, e.name)}
              >
                <span className="sim-icon">{e.icon && <img src={iconURL(e.icon)} alt="" loading="lazy" />}</span>
                <span className="small">{e.name}</span>
              </button>
            ))}
          </div>
        </div>
      ))}
      <div className="sim-consumable-kind">
        <h3>Gems</h3>
        <div className="sim-omnium-node">
          {reference.gems.map((g) => (
            <button
              key={g.id}
              className={value.gems?.includes(g.id) ? 'sim-rune picked' : 'sim-rune'}
              aria-pressed={!!value.gems?.includes(g.id)}
              onClick={() => onChange({ ...value, gems: toggle(value.gems, g.id) })}
              {...tip(g.stats, g.unique ? `${g.name} (unique)` : g.name)}
            >
              <span className="sim-icon">{g.icon && <img src={iconURL(g.icon)} alt="" loading="lazy" />}</span>
              <span className="small">{g.name}</span>
            </button>
          ))}
        </div>
      </div>
      {picked > 0 && (
        <div className="actions">
          <button onClick={() => onChange({})}>Clear</button>
        </div>
      )}
    </section>
  )
}
