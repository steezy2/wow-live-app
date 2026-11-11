import type { ReactNode } from 'react'
import { abilityViews, type AbilityRow } from './abilities'
import type { KeyView } from './api'
import { classColors } from './classColors'
import { amount } from './format'
import { BarRow } from './MeterCards'
import { schoolBar, schoolName } from './schoolColors'
import { Tabs } from './Tabs'

/** One player's key up close, the page's focused player or else the one whose game wrote the log (a pick here
 * focuses them everywhere via onFocus): their damage and healing by ability with casts and hits, what they took
 * damage from, and what they cast, each as bars against the biggest in the ability's spell school colour (the
 * player's class colour in keys saved without schools). */
export function PlayerFocus(props: { keyView: KeyView; focus: string; onFocus: (player: string) => void }) {
  const { keyView: key, focus, onFocus: setPlayer } = props
  const player = focus || (key.logger ?? key.players[0] ?? '')
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

/** Abilities (or whatever label names) as bars, biggest first, each in its spell school's colour (color without one):
 * the number, its share of the total, and casts and hits where the rows have them. Counts show whole; everything else
 * shortened. */
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
  const withHits = sorted.some((r) => r.hits) // none up to a replay's moment, which keeps no hits
  const columns = 2 + Number(withCasts) + Number(withHits)
  const grid = { gridTemplateColumns: `minmax(120px, 2fr) repeat(${columns}, minmax(48px, 1fr))` }
  if (sorted.length === 0) return <p className="muted">None</p>
  return (
    <div className="scroll">
      <div className="card-row card-head muted" style={grid}>
        <span>{label}</span>
        <span>{count ? 'Casts' : 'Amount'}</span>
        <span>%</span>
        {withCasts && <span>Casts</span>}
        {withHits && <span>Hits</span>}
      </div>
      {sorted.map((r) => (
        <BarRow key={r.name} share={r.value / sorted[0].value} {...schoolBar(r.school, color)} grid={grid}>
          <span className="name" title={schoolName(r.school)}>
            {r.name}
          </span>
          <span>{count ? r.value : amount(r.value)}</span>
          <span>{((100 * r.value) / total).toFixed(1)}</span>
          {withCasts && <span>{r.casts || '–'}</span>}
          {withHits && <span>{r.hits || '–'}</span>}
        </BarRow>
      ))}
    </div>
  )
}
