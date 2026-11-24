import { useState } from 'react'
import type { SimItem, SimReference, SimVariant } from './api'
import { enchantsFor, upgradeTrackOf } from './simOptions'

/** The name a gem or enchant is listed by: its quality when it has one, and its stats. */
const optionText = (o: { name: string; stats?: string; quality?: number }) =>
  `${o.name}${o.quality ? ` (Q${o.quality})` : ''}${o.stats ? ` · ${o.stats}` : ''}`

/** The changes to try on one item, as a form: another upgrade level of its track, an added socket, a gem and an enchant
 * for its slot. Adding makes a variant, which Top Gear sims beside the item. */
function VariantForm(props: { item: SimItem; reference: SimReference; onAdd: (v: Change) => void }) {
  const { item, reference, onAdd } = props
  const on = upgradeTrackOf(reference, item)
  const [upgrade, setUpgrade] = useState(0)
  const [socket, setSocket] = useState(false)
  const [gem, setGem] = useState(0)
  const [enchant, setEnchant] = useState(0)
  const nothing = !upgrade && !socket && !gem && !enchant
  return (
    <div className="sim-variant-form">
      {on && (
        <label>
          Upgrade to
          <select value={upgrade} onChange={(e) => setUpgrade(Number(e.target.value))}>
            <option value={0}>Keep {on.track.name}</option>
            {on.track.levels
              .filter((l) => l.level > on.level.level)
              .map((l) => (
                <option key={l.level} value={l.level}>
                  {on.track.name} {l.level}/{on.track.levels.length} ({l.itemLevel})
                </option>
              ))}
          </select>
        </label>
      )}
      <label className="inline">
        <input type="checkbox" checked={socket} onChange={(e) => setSocket(e.target.checked)} /> Add a socket
      </label>
      <label>
        {socket ? 'Gem in the new socket' : 'Gem in its first socket'}
        <select value={gem} onChange={(e) => setGem(Number(e.target.value))}>
          <option value={0}>None</option>
          {reference.gems.map((g) => (
            <option key={g.id} value={g.id}>
              {optionText(g)}
            </option>
          ))}
        </select>
      </label>
      <label>
        Enchant
        <select value={enchant} onChange={(e) => setEnchant(Number(e.target.value))}>
          <option value={0}>None</option>
          {enchantsFor(reference, item.slot).map((e) => (
            <option key={e.id} value={e.id}>
              {optionText(e)}
            </option>
          ))}
        </select>
      </label>
      <button
        disabled={nothing}
        onClick={() => {
          onAdd({
            upgrade: upgrade || undefined,
            socket: socket || undefined,
            gem: gem || undefined,
            enchant: enchant || undefined,
          })
          setUpgrade(0)
          setSocket(false)
          setGem(0)
          setEnchant(0)
        }}
      >
        Try it too
      </button>
    </div>
  )
}

/** What a variant form fills in: the rest of a SimVariant (which item) comes from the row it is on. */
type Change = Pick<SimVariant, 'upgrade' | 'socket' | 'gem' | 'enchant'>

/** Under an item of Top Gear: its variants (changed copies sim'd as candidates, each removable, named by the plan's
 * notes, which follow the variants' order) and a "Change…" form that adds one. */
export function ItemVariants(props: {
  item: SimItem
  worn: boolean
  index: number
  reference: SimReference
  variants: SimVariant[]
  notes: string[]
  onAdd: (v: SimVariant) => void
  onRemove: (v: SimVariant) => void
}) {
  const { item, worn, index, reference, variants, notes } = props
  return (
    <div className="sim-variants">
      {variants.map(
        (v, i) =>
          !!v.worn === worn &&
          v.index === index && (
            <span key={JSON.stringify(v)} className="sim-variant-chip">
              {notes[i] ?? '…'}
              <button className="delete" aria-label="Remove this variant" onClick={() => props.onRemove(v)}>
                ×
              </button>
            </span>
          ),
      )}
      <details>
        <summary className="muted small">Upgrade, socket, enchant…</summary>
        <VariantForm item={item} reference={reference} onAdd={(c) => props.onAdd({ worn, index, ...c })} />
      </details>
    </div>
  )
}
