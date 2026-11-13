import { useState, type CSSProperties, type KeyboardEvent, type ReactNode } from 'react'
import type { KeyMeter, KeyMeterRow } from './api'
import { cardGrid, type Column } from './meterColumns'
import { classColors } from './classColors'
import { amount, clock } from './format'

/** Rows from which a card is striped. */
const ZEBRA_ROWS = 8

/** One card: a row per player, sorted by the first column (with sortable, by the column whose heading was clicked),
 * with a class-coloured bar across the row against the top value. With onPick, a click on a row picks its player; with
 * rows, only that many players show; dense (the overlay) packs the columns closer. */
export function MeterCard({
  title,
  meter,
  columns,
  onPick,
  sortable = false,
  rows: limit = 0,
  dense = false,
}: {
  title: string
  meter: KeyMeter
  columns: Column[]
  onPick?: (player: string) => void
  sortable?: boolean
  /** Players to show, the top ones; 0 or missing for everyone. */
  rows?: number
  dense?: boolean
}) {
  const [by, setBy] = useState(columns[0].label)
  const first = columns.find((c) => c.label === by) ?? columns[0]
  const sorted = [...meter.rows].sort((a, b) => first.value(b) - first.value(a))
  const rows = limit > 0 ? sorted.slice(0, limit) : sorted
  const top = Math.max(1, ...rows.map(first.value))
  const { grid, minWidth } = cardGrid(columns.length, dense)
  return (
    <div className="card">
      <h2>
        {title}{' '}
        {rows.length > 0 && (
          <span className="muted">
            {clock(meter.startMS)} → {clock(meter.endMS)}
          </span>
        )}
      </h2>
      <div className="card-scroll">
        <div className={rows.length >= ZEBRA_ROWS ? 'card-table zebra' : 'card-table'} style={{ minWidth }}>
          <div className="card-row card-head muted" style={grid}>
            <span>Player</span>
            {columns.map((c) =>
              sortable ? (
                <button
                  key={c.label}
                  className={c.group ? 'sort group' : 'sort'}
                  aria-pressed={c === first}
                  onClick={() => setBy(c.label)}
                >
                  {c.label}
                </button>
              ) : (
                <span key={c.label} className={c.group ? 'group' : undefined}>
                  {c.label}
                </span>
              ),
            )}
          </div>
          {rows.map((r) => {
            const color = classColors[r.class] ?? 'var(--accent)'
            return (
              <BarRow
                key={r.player}
                share={first.value(r) / top}
                color={color}
                grid={grid}
                onPick={onPick && (() => onPick(r.player))}
              >
                <span className="name" style={{ color }}>
                  {r.player}
                </span>
                {columns.map((c) => (
                  <span key={c.label} className={cellClass(c, r, rows)} title={c.title?.(r)}>
                    {c.show ? c.show(r) : amount(c.value(r))}
                  </span>
                ))}
              </BarRow>
            )
          })}
        </div>
      </div>
      {rows.length === 0 && <p className="muted">No pull yet</p>}
    </div>
  )
}

/** A numbers cell's classes: a group rule and its tone. */
function cellClass(c: Column, r: KeyMeterRow, rows: KeyMeterRow[]): string {
  return ['num', c.group && 'group', c.tone?.(r, rows)].filter(Boolean).join(' ')
}

/** A card's row with a coloured bar across it, share (0 to 1) of the row wide: in color, or filled with fill (a CSS
 * background) for a bar of several colours; with onPick, a button. */
export function BarRow({
  share,
  color,
  fill,
  grid,
  onPick,
  children,
}: {
  share: number
  color: string
  fill?: string
  grid: CSSProperties
  onPick?: () => void
  children: ReactNode
}) {
  const button = onPick && {
    role: 'button',
    tabIndex: 0,
    onClick: onPick,
    onKeyDown: (e: KeyboardEvent) => e.key === 'Enter' && onPick(),
  }
  return (
    <div className={onPick ? 'card-row pick' : 'card-row'} style={grid} {...button}>
      <span
        className="meter-bar"
        aria-hidden="true"
        style={{ width: `${share * 100}%`, '--bar': color, '--bar-fill': fill } as CSSProperties}
      />
      {children}
    </div>
  )
}
