import { iconURL, type SimReference } from './api'
import { groupLabel } from './simOptions'
import { tip } from './tipAttrs'

/** What a gem or enchant says on hover: its name, quality and stats. */
const optionTip = (o: { name: string; stats?: string; quality?: number }) =>
  `${o.name}${o.quality ? ` (Q${o.quality})` : ''}${o.stats ? ` · ${o.stats}` : ''}`

/** The enchants and gems to try, as a card laid out like the Omnium Folio's: a row of icons for each slot's enchants
 * and one for the gems, each to click. Nothing picked keeps what is worn; each enchant picked is tried on every worn
 * item its slot fits, and each gem in the first socket of every worn item that has one (Top Gear sims them as
 * variants of those items). */
export function EnhancementsCard(props: {
  reference: SimReference
  enchants: number[]
  gems: number[]
  onEnchants: (ids: number[]) => void
  onGems: (ids: number[]) => void
}) {
  const { reference, enchants, gems, onEnchants, onGems } = props
  const picks = enchants.length + gems.length
  const slots = [...new Set(reference.enchants.map((e) => e.slots[0]))] // one row for each slot group
  const toggle = (list: number[], id: number) => (list.includes(id) ? list.filter((p) => p !== id) : [...list, id])
  const chip = (o: { id: number; name: string; stats?: string; quality?: number; icon: string }, picked: number[],
    pick: (ids: number[]) => void) => (
    <button
      key={o.id}
      className={picked.includes(o.id) ? 'sim-rune picked' : 'sim-rune'}
      aria-pressed={picked.includes(o.id)}
      onClick={() => pick(toggle(picked, o.id))}
      {...tip(optionTip(o))}
    >
      <span className="sim-icon">{o.icon && <img src={iconURL(o.icon)} alt="" loading="lazy" />}</span>
      <span className="small">{o.name}</span>
    </button>
  )
  return (
    <section className="panel sim-enhancements">
      <h2>
        Enchants and gems{' '}
        {picks > 0 && <span className="muted small">{picks} picked</span>}
      </h2>
      <p className="muted small">
        Pick what to try on your worn gear: each enchant on every worn item its slot fits, each gem in the first socket
        of every worn item that has one. Nothing picked keeps what you wear.
      </p>
      {slots.map((slot) => (
        <div key={slot} className="sim-consumable-kind">
          <h3>{slot.startsWith('main_hand') ? 'Weapon' : groupLabel(slot.replace(/d$/, ''))} enchants</h3>
          <div className="sim-omnium-node">
            {reference.enchants.filter((e) => e.slots[0] === slot).map((e) => chip(e, enchants, onEnchants))}
          </div>
        </div>
      ))}
      <div className="sim-consumable-kind">
        <h3>Gems</h3>
        <div className="sim-omnium-node">{reference.gems.map((g) => chip(g, gems, onGems))}</div>
      </div>
      {picks > 0 && (
        <div className="actions">
          <button
            onClick={() => {
              onEnchants([])
              onGems([])
            }}
          >
            Back to what I wear
          </button>
        </div>
      )}
    </section>
  )
}
