import type { SimCompare, SimConsumables, SimReference } from './api'

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

/** The compare mode: pick a consumable kind and tick the values to sim against each other (and against the
 * baseline, which is the sim's own consumables). */
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
    <div className="sim-compare">
      <label>
        Compare
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
        <span className="muted small">{compare.values.length} ticked</span>
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
              <span className="sim-icon">{c.icon && <img src={`/api/icons/${c.icon}`} alt="" loading="lazy" />}</span>
              {c.name}
            </label>
          </li>
        ))}
      </ul>
    </div>
  )
}
