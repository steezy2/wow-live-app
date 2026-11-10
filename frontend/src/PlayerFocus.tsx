import { useState } from 'react'
import type { KeyView } from './api'
import { classColors } from './classColors'
import { amount } from './format'
import { BarRow } from './MeterCards'
import { Tabs } from './Tabs'

/** One line of a player's breakdown: an ability and its number, with how often it was cast and hit where that's
 * known. */
interface AbilityRow {
  name: string
  value: number
  casts?: number
  hits?: number
}

/** One player's key up close, picked from the party: their damage and healing by ability with casts and hits, what
 * they took damage from, and what they cast, each as bars against the biggest. */
export function PlayerFocus({ keyView: key }: { keyView: KeyView }) {
  const [player, setPlayer] = useState(key.players[0] ?? '')
  const row = key.overall.rows.find((r) => r.player === player)
  const color = classColors[row?.class ?? ''] ?? 'var(--accent)'
  const mine = (key.abilities ?? []).filter((a) => a.player === player)
  const done = (pick: 'damage' | 'healing') =>
    mine.filter((a) => a[pick] > 0).map((a) => ({ name: a.ability, value: a[pick], casts: a.casts, hits: a.hits }))
  const views: { name: string; rows: AbilityRow[]; count?: boolean }[] = [
    { name: 'Damage', rows: done('damage') },
    { name: 'Healing', rows: done('healing') },
    {
      name: 'Damage taken',
      rows: (key.taken ?? []).filter((h) => h.player === player).map((h) => ({ name: h.ability, value: h.damage })),
    },
    {
      name: 'Casts',
      rows: mine.filter((a) => a.casts > 0).map((a) => ({ name: a.ability, value: a.casts })),
      count: true,
    },
  ]
  return (
    <div className="card focus">
      <h2>
        <label>
          Player focus{' '}
          <select value={player} onChange={(e) => setPlayer(e.target.value)}>
            {key.players.map((p) => (
              <option key={p}>{p}</option>
            ))}
          </select>
        </label>
      </h2>
      {key.abilities ? (
        <>
          {row && (
            <p className="muted small">
              {amount(row.dps)} DPS · {amount(row.hps)} HPS · {amount(row.damageTaken)} taken ·{' '}
              {key.deaths.filter((d) => d.player === player).length} deaths · {row.interrupts} kicks
            </p>
          )}
          <Tabs
            tabs={views.map((v) => ({
              name: v.name,
              body: <AbilityBars rows={v.rows} color={color} count={v.count} />,
            }))}
          />
        </>
      ) : (
        <p className="muted">This key was saved before abilities were kept: keys played from now on have them.</p>
      )}
    </div>
  )
}

/** Abilities as bars, biggest first: the number, its share of the total, and casts and hits where the rows have them.
 * Counts show whole; everything else shortened. */
function AbilityBars({ rows, color, count = false }: { rows: AbilityRow[]; color: string; count?: boolean }) {
  const sorted = [...rows].sort((a, b) => b.value - a.value)
  const total = sorted.reduce((sum, r) => sum + r.value, 0)
  const withCasts = sorted.some((r) => r.casts !== undefined)
  const grid = { gridTemplateColumns: `minmax(120px, 2fr) repeat(${withCasts ? 4 : 2}, minmax(48px, 1fr))` }
  if (sorted.length === 0) return <p className="muted">None</p>
  return (
    <div className="scroll">
      <div className="card-row card-head muted" style={grid}>
        <span>Ability</span>
        <span>{count ? 'Casts' : 'Amount'}</span>
        <span>%</span>
        {withCasts && <span>Casts</span>}
        {withCasts && <span>Hits</span>}
      </div>
      {sorted.map((r) => (
        <BarRow key={r.name} share={r.value / sorted[0].value} color={color} grid={grid}>
          <span className="name">{r.name}</span>
          <span>{count ? r.value : amount(r.value)}</span>
          <span>{((100 * r.value) / total).toFixed(1)}</span>
          {withCasts && <span>{r.casts || '–'}</span>}
          {withCasts && <span>{r.hits}</span>}
        </BarRow>
      ))}
    </div>
  )
}
