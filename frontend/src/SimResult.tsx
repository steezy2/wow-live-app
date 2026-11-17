import { useState } from 'react'
import type { Sim, SimCombo } from './api'
import { amount, clock } from './format'
import { Spell } from './Spell'
import { ItemLine } from './SimItem'
import { useItemInfo } from './simItems'
import { gainShare } from './simOptions'

/** The gear combinations or drops of a Top Gear or Droptimizer sim, best first: each a row with its rank, a bar for
 * its gain over the equipped gear, its DPS, and what changes (icons and item levels) and where a drop comes from.
 * Drops filter by source. */
function Combos({ combos, drops }: { combos: SimCombo[]; drops: boolean }) {
  const [source, setSource] = useState('')
  const swapped = combos.flatMap((c) => c.swaps.flatMap((s) => [s.out, s.in]))
  const info = useItemInfo(swapped.flatMap((i) => (i ? [i] : [])))
  const sources = [...new Set(combos.flatMap((c) => (c.source ? [c.source] : [])))]
  const shown = source ? combos.filter((c) => c.source === source) : combos
  const best = Math.max(0, ...shown.map((c) => c.gain))
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
      <ol className="sim-combos">
        {shown.map((c, i) => (
          <li key={i}>
            <span className="sim-rank">{i + 1}</span>
            <div className="sim-gain">
              <span className={c.gain >= 0 ? 'good' : 'warning'}>
                {c.gain >= 0 ? '+' : ''}
                {amount(c.gain)}
              </span>
              <span className="muted small">{amount(c.dps)} DPS</span>
              <span className="sim-gain-bar">
                <span style={{ width: `${gainShare(c.gain, best)}%` }} />
              </span>
            </div>
            <div className="sim-swaps">
              {c.swaps.map((s) => (
                <div key={s.slot} className="sim-swap">
                  <span className="muted small sim-swap-slot">{s.slot.replace('_', ' ')}</span>
                  {s.out ? <ItemLine item={s.out} info={info.get(s.out.id)} /> : <span className="muted">empty</span>}
                  <span className="muted">→</span>
                  {s.in ? <ItemLine item={s.in} info={info.get(s.in.id)} /> : <span className="muted">empty</span>}
                </div>
              ))}
              {drops && (
                <span className="muted small">
                  {c.boss} · {c.source}
                </span>
              )}
            </div>
          </li>
        ))}
      </ol>
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
        <h2 className="sim-dps">
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
