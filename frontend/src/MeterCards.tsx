import type { CSSProperties, ReactNode } from 'react'
import type { KeyMeter, KeyMeterRow, KeyView } from './api'
import { type Column, defense, offense } from './meterColumns'
import { classColors } from './classColors'
import { amount, clock, pullTitle } from './format'

/** The four meter cards: offense and defense, for the current (or last) pull and the whole key. */
export function MeterCards({ keyView: key, clockMS }: { keyView: KeyView; clockMS: number }) {
  const pull = key.lastPull
  const title = pullTitle(key, clockMS)
  return (
    <section className="cards">
      <MeterCard title={title} meter={pull} columns={offense} />
      <MeterCard title="Whole key" meter={key.overall} columns={offense} />
      <MeterCard title={title} meter={pull} columns={defense(key.reference)} />
      <MeterCard title="Whole key" meter={key.overall} columns={defense(key.reference)} />
    </section>
  )
}

const NAME_PX = 96
const NUMBER_PX = 64
const GAP_PX = 8 // --s2
/** Rows from which a card is striped. */
const ZEBRA_ROWS = 8

/** One card: a row per player, sorted by the first column, with a class-coloured bar across the row against the
 * top value. */
export function MeterCard({ title, meter, columns }: { title: string; meter: KeyMeter; columns: Column[] }) {
  const [first] = columns
  const rows = [...meter.rows].sort((a, b) => first.value(b) - first.value(a))
  const top = Math.max(1, ...rows.map(first.value))
  const grid = { gridTemplateColumns: `minmax(${NAME_PX}px, 1fr) repeat(${columns.length}, ${NUMBER_PX}px)` }
  const minWidth = NAME_PX + columns.length * (NUMBER_PX + GAP_PX)
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
            {columns.map((c) => (
              <span key={c.label} className={c.group ? 'group' : undefined}>
                {c.label}
              </span>
            ))}
          </div>
          {rows.map((r) => {
            const color = classColors[r.class] ?? 'var(--accent)'
            return (
              <BarRow key={r.player} share={first.value(r) / top} color={color} grid={grid}>
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
 * background) for a bar of several colours. */
export function BarRow({
  share,
  color,
  fill,
  grid,
  children,
}: {
  share: number
  color: string
  fill?: string
  grid: CSSProperties
  children: ReactNode
}) {
  return (
    <div className="card-row" style={grid}>
      <span
        className="meter-bar"
        aria-hidden="true"
        style={{ width: `${share * 100}%`, '--bar': color, '--bar-fill': fill } as CSSProperties}
      />
      {children}
    </div>
  )
}
