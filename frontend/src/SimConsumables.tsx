import { iconURL, type SimCompare, type SimConsumables, type SimReference } from './api'

/** The page's name for each consumable kind; a kind it doesn't know shows as the app names it. */
const CONSUMABLE_LABELS: Record<string, string> = {
  flask: 'Flask',
  food: 'Food',
  potion: 'Potion',
  augment: 'Augment rune',
  weapon: 'Weapon oil',
}
const labelOf = (kind: string) => CONSUMABLE_LABELS[kind] ?? kind

/** One dropdown for each consumable kind (in the app's order), each on "Default" (what simc assumes for the spec)
 * until one is picked. */
export function ConsumableSelects(props: {
  kinds: string[]
  reference: SimReference
  value: SimConsumables
  onChange: (c: SimConsumables) => void
}) {
  const { kinds, reference, value, onChange } = props
  return (
    <div className="sim-consumables">
      {kinds.map((kind) => (
        <label key={kind}>
          {labelOf(kind)}
          <select value={value[kind] ?? ''} onChange={(e) => onChange({ ...value, [kind]: e.target.value })}>
            <option value="">Default</option>
            {reference.consumables[kind].map((c) => (
              <option key={c.value} value={c.value}>
                {c.name}
              </option>
            ))}
          </select>
        </label>
      ))}
    </div>
  )
}

/** Compare consumables, folded away until opened: pick a consumable kind and tick the values to sim against each
 * other. Each ticked one replaces the baseline's for its kind; a Top Gear or Droptimizer sim runs every combination
 * once per value, within its limit. */
export function ConsumableCompare(props: {
  kinds: string[]
  reference: SimReference
  compare: SimCompare
  max: number
  onChange: (c: SimCompare) => void
}) {
  const { kinds, reference, compare, max, onChange } = props
  const values = reference.consumables[compare.kind]
  return (
    <details className="sim-compare">
      <summary>
        Compare {labelOf(compare.kind).toLowerCase()}s
        {compare.values.length > 0 && <span className="muted small"> · {compare.values.length} ticked</span>}
      </summary>
      <label>
        Kind
        <select value={compare.kind} onChange={(e) => onChange({ kind: e.target.value, values: [] })}>
          {kinds.map((k) => (
            <option key={k} value={k}>
              {labelOf(k)}s
            </option>
          ))}
        </select>
      </label>
      <div className="actions">
        <button onClick={() => onChange({ ...compare, values: values.slice(0, max).map((c) => c.value) })}>All</button>
        <button onClick={() => onChange({ ...compare, values: [] })}>None</button>
      </div>
      <ul className="sim-compare-list">
        {values.map((c) => (
          <li key={c.value}>
            <label className="inline">
              <input
                type="checkbox"
                checked={compare.values.includes(c.value)}
                onChange={(e) =>
                  onChange({
                    ...compare,
                    values: e.target.checked
                      ? [...compare.values, c.value]
                      : compare.values.filter((v) => v !== c.value),
                  })
                }
              />
              <span className="sim-icon">{c.icon && <img src={iconURL(c.icon)} alt="" loading="lazy" />}</span>
              {c.name}
            </label>
          </li>
        ))}
      </ul>
    </details>
  )
}
