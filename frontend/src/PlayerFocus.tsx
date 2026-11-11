import { useState, type ReactNode } from 'react'
import { abilityViews, type AbilityRow } from './abilities'
import type { KeyView } from './api'
import { classColors } from './classColors'
import { amount } from './format'
import { BarRow } from './MeterCards'
import { Tabs } from './Tabs'

/** One player's key up close, picked from the party: their damage and healing by ability with casts and hits, what
 * they took damage from, and what they cast, each as bars against the biggest. */
export function PlayerFocus({ keyView: key }: { keyView: KeyView }) {
  const [player, setPlayer] = useState(key.players[0] ?? '')
  const row = key.overall.rows.find((r) => r.player === player)
  const color = classColors[row?.class ?? ''] ?? 'var(--accent)'
  const views = abilityViews(player, key.abilities ?? [], key.taken ?? [])
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
          <AbilityTabs views={views} color={color} />
        </>
      ) : (
        <p className="muted">This key was saved before abilities were kept: keys played from now on have them.</p>
      )}
    </div>
  )
}

/** A player's breakdowns as tabs: Damage, Healing, Damage taken and Casts, then any more the page adds. */
export function AbilityTabs(props: {
  views: ReturnType<typeof abilityViews>
  color: string
  more?: { name: string; body: ReactNode }[]
}) {
  const { views, color, more = [] } = props
  return (
    <Tabs
      tabs={[
        { name: 'Damage', body: <AbilityBars rows={views.damage} color={color} /> },
        { name: 'Healing', body: <AbilityBars rows={views.healing} color={color} /> },
        { name: 'Damage taken', body: <AbilityBars rows={views.taken} color={color} /> },
        { name: 'Casts', body: <AbilityBars rows={views.casts} color={color} count /> },
        ...more,
      ]}
    />
  )
}

/** Abilities (or whatever label names) as bars, biggest first: the number, its share of the total, and casts and hits
 * where the rows have them. Counts show whole; everything else shortened. */
export function AbilityBars({
  rows,
  color,
  count = false,
  label = 'Ability',
}: {
  rows: AbilityRow[]
  color: string
  count?: boolean
  label?: string
}) {
  const sorted = [...rows].sort((a, b) => b.value - a.value)
  const total = sorted.reduce((sum, r) => sum + r.value, 0)
  const withCasts = sorted.some((r) => r.casts !== undefined)
  const grid = { gridTemplateColumns: `minmax(120px, 2fr) repeat(${withCasts ? 4 : 2}, minmax(48px, 1fr))` }
  if (sorted.length === 0) return <p className="muted">None</p>
  return (
    <div className="scroll">
      <div className="card-row card-head muted" style={grid}>
        <span>{label}</span>
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
