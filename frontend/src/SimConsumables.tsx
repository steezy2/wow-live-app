import {
  SIM_CONSUMABLE_KINDS,
  type SimCompare,
  type SimConsumableKind,
  type SimConsumables,
  type SimReference,
} from './api'

/** The page's name for each consumable kind. */
const CONSUMABLE_LABELS: Record<SimConsumableKind, string> = {
  flask: 'Flask',
  food: 'Food',
  potion: 'Potion',
  augment: 'Augment rune',
  weapon: 'Weapon oil',
}

/** One dropdown for each consumable kind, each on "Default" (what simc assumes for the spec) until one is picked. */
export function ConsumableSelects(props: {
  reference: SimReference
  value: SimConsumables
  onChange: (c: SimConsumables) => void
}) {
  const { reference, value, onChange } = props
  return (
    <div className="sim-consumables">
      {SIM_CONSUMABLE_KINDS.map((kind) => (
        <label key={kind}>
          {CONSUMABLE_LABELS[kind]}
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
  reference: SimReference
  compare: SimCompare
  max: number
  onChange: (c: SimCompare) => void
}) {
  const { reference, compare, max, onChange } = props
  const values = reference.consumables[compare.kind]
  return (
    <div className="sim-compare">
      <label>
        Compare
        <select
          value={compare.kind}
          onChange={(e) => onChange({ kind: e.target.value as SimConsumableKind, values: [] })}
        >
          {SIM_CONSUMABLE_KINDS.map((k) => (
            <option key={k} value={k}>
              {CONSUMABLE_LABELS[k]}s
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
