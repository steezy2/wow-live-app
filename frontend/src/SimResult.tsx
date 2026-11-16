import { useState } from 'react'
import type { Sim, SimCombo, SimItem, SimSwap } from './api'
import { amount, clock } from './format'
import { Spell } from './Spell'

/** An item as the page names it: its name (or ID) and item level. */
const itemText = (item: SimItem) => `${item.name ?? `item ${item.id}`}${item.level ? ` (${item.level})` : ''}`

/** What a swap does: "Ring A (300) → Ring B (310)", or the slot emptied. */
function swapText(s: SimSwap): string {
  return `${s.out ? itemText(s.out) : 'empty'} → ${s.in ? itemText(s.in) : 'empty'}`
}

/** The gear combinations or drops of a Top Gear or Droptimizer sim, best first: the gain over the equipped gear,
 * the DPS, what changes and where a drop comes from. Drops filter by source. */
function Combos({ combos, drops }: { combos: SimCombo[]; drops: boolean }) {
  const [source, setSource] = useState('')
  const sources = [...new Set(combos.flatMap((c) => (c.source ? [c.source] : [])))]
  const shown = source ? combos.filter((c) => c.source === source) : combos
  return (
    <section className="panel">
      <h2>{drops ? 'Upgrades from drops' : 'Gear combinations'}</h2>
      {drops && sources.length > 1 && (
        <label className="inline">
          Source{' '}
          <select value={source} onChange={(e) => setSource(e.target.value)}>
            <option value="">All</option>
            {sources.map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
        </label>
      )}
      <div className="table-scroll">
        <table className="review-table">
          <thead>
            <tr>
              <th>Gain</th>
              <th>DPS</th>
              <th>Change</th>
              {drops && <th>From</th>}
            </tr>
          </thead>
          <tbody>
            {shown.map((c, i) => (
              <tr key={i}>
                <td className={c.gain >= 0 ? 'good' : 'warning'}>
                  {c.gain >= 0 ? '+' : ''}
                  {amount(c.gain)}
                </td>
                <td>{amount(c.dps)}</td>
                <td style={{ textAlign: 'left' }}>
                  {c.swaps.map((s) => (
                    <div key={s.slot}>
                      <span className="muted">{s.slot.replace('_', ' ')}</span> {swapText(s)}
                    </div>
                  ))}
                </td>
                {drops && (
                  <td style={{ textAlign: 'left' }}>
                    {c.boss}
                    <span className="muted"> · {c.source}</span>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}

/** A finished sim: the DPS, a Top Gear or Droptimizer's ranked rows, each ability's share of the damage and the buffs
 * that were up. */
export function SimResult({ sim }: { sim: Sim }) {
  const result = sim.result
  if (!result) return null
  const top = Math.max(1, ...result.abilities.map((a) => a.dps))
  const share = (dps: number) => (dps / top) * 100
  return (
    <>
      <section className="panel">
        <h2>
          {amount(result.dps)} DPS <span className="muted small">± {amount(result.dpsError)}</span>
        </h2>
        <p className="muted small">
          {sim.options.fightStyle}, {sim.options.length} s, {sim.options.targets}{' '}
          target{sim.options.targets > 1 ? 's' : ''}
          {sim.tookMS ? ` · took ${clock(sim.tookMS)}` : ''}
        </p>
      </section>
      {result.combos && <Combos combos={result.combos} drops={sim.kind === 'drops'} />}
      <section className="panel">
        <h2>Abilities</h2>
        <div className="table-scroll">
          <table className="review-table">
            <thead>
              <tr>
                <th>Ability</th>
                <th>DPS</th>
                <th>Share</th>
                <th>Casts</th>
                <th>Per cast</th>
              </tr>
            </thead>
            <tbody>
              {result.abilities.map((a) => (
                <tr key={a.id || a.name}>
                  <td
                    style={{
                      textAlign: 'left',
                      backgroundImage: `linear-gradient(to right, var(--border) ${share(a.dps)}%, transparent 0)`,
                    }}
                  >
                    <Spell name={a.name} />
                  </td>
                  <td>{amount(a.dps)}</td>
                  <td>{Math.round(a.share * 100)}%</td>
                  <td>{a.casts.toFixed(1)}</td>
                  <td>{amount(a.perCast)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>
      {result.buffs.length > 0 && (
        <section className="panel">
          <h2>Buffs</h2>
          <ul className="list">
            {result.buffs.map((b) => (
              <li key={b.id || b.name}>
                <Spell name={b.name} /> <span className="muted">{Math.round(b.uptime)}%</span>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}
