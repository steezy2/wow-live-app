import { iconURL, type SimConsumables, type SimReference } from './api'
import { comparedCount } from './simOptions'
import { itemTip, tip } from './tipAttrs'

/** The page's name for each consumable kind; a kind it doesn't know shows as the app names it. */
const CONSUMABLE_LABELS: Record<string, string> = {
  flask: 'Flask',
  food: 'Food',
  potion: 'Potion',
  augment: 'Augment rune',
  weapon: 'Weapon oil',
}
const labelOf = (kind: string) => CONSUMABLE_LABELS[kind] ?? kind

/** What a kind's picks mean, in words. */
const pickText = (n: number) => (n === 0 ? 'simc default' : n === 1 ? 'used as it is' : `${n} compared`)

/** The consumables card, laid out like the Omnium Folio's: a row of icons for each kind (in the app's order), each
 * one to click. A kind with none picked keeps simc's default for the spec, one is used by the whole sim, and two or
 * more are compared, every combination of the compared kinds at most max of them (a Top Gear or Droptimizer sim runs
 * each of its combinations once for each). */
export function ConsumablesCard(props: {
  kinds: string[]
  reference: SimReference
  value: SimConsumables
  max: number
  onChange: (c: SimConsumables) => void
}) {
  const { kinds, reference, value, max, onChange } = props
  const combos = comparedCount(value)
  const picked = (kind: string) => value[kind] ?? []
  const toggle = (kind: string, v: string) => {
    const now = picked(kind)
    onChange({ ...value, [kind]: now.includes(v) ? now.filter((p) => p !== v) : [...now, v] })
  }
  return (
    <section className="panel sim-consumables">
      <h2>
        Consumables {combos > 1 && <span className="muted small">{combos} combinations compared</span>}
      </h2>
      <p className="muted small">
        Pick what to sim with: nothing keeps simc&apos;s default, one is used by the whole sim, and two or more of a
        kind are compared. Top Gear and the Droptimizer run each of their combinations once for every one.
      </p>
      {combos > max && <p className="warning">That is more than the {max} combinations a sim compares: untick some.</p>}
      {kinds.map((kind) => (
        <div key={kind} className="sim-consumable-kind">
          <h3>
            {labelOf(kind)} <span className="muted small">{pickText(picked(kind).length)}</span>
          </h3>
          <div className="sim-omnium-node">
            <button
              className={picked(kind).length === 0 ? 'sim-rune picked' : 'sim-rune'}
              aria-pressed={picked(kind).length === 0}
              onClick={() => onChange({ ...value, [kind]: [] })}
              {...tip('What SimulationCraft assumes for your spec; pick nothing unless you want to try something else')}
            >
              <span className="small">SimC default</span>
            </button>
            {reference.consumables[kind].map((c) => (
              <button
                key={c.value}
                className={picked(kind).includes(c.value) ? 'sim-rune picked' : 'sim-rune'}
                aria-pressed={picked(kind).includes(c.value)}
                onClick={() => toggle(kind, c.value)}
                {...itemTip(c.itemID, c.name)}
              >
                <span className="sim-icon">{c.icon && <img src={iconURL(c.icon)} alt="" loading="lazy" />}</span>
                <span className="small">{c.name}</span>
              </button>
            ))}
          </div>
        </div>
      ))}
      {kinds.some((k) => picked(k).length > 0) && (
        <div className="actions">
          <button onClick={() => onChange({})}>Back to simc&apos;s defaults</button>
        </div>
      )}
    </section>
  )
}
