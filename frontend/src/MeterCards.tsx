import type { CSSProperties, KeyboardEvent, ReactNode } from 'react'
import type { KeyMeter, KeyView } from './api'
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

/** One card: a row per player, sorted by the first column, with a class-coloured bar across the row against the
 * top value. With onPick, a click on a row picks its player. */
export function MeterCard({
  title,
  meter,
  columns,
  onPick,
}: {
  title: string
  meter: KeyMeter
  columns: Column[]
  onPick?: (player: string) => void
}) {
  const [first] = columns
  const rows = [...meter.rows].sort((a, b) => first.value(b) - first.value(a))
  const top = Math.max(1, ...rows.map(first.value))
  const grid = { gridTemplateColumns: `minmax(120px, 1.6fr) repeat(${columns.length}, minmax(56px, 1fr))` }
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
      <div className="card-row card-head muted" style={grid}>
        <span>Player</span>
        {columns.map((c) => (
          <span key={c.label}>{c.label}</span>
        ))}
      </div>
      {rows.map((r) => (
        <BarRow
          key={r.player}
          share={first.value(r) / top}
          color={classColors[r.class] ?? 'var(--accent)'}
          grid={grid}
          onPick={onPick && (() => onPick(r.player))}
        >
          <span className="name">{r.player}</span>
          {columns.map((c) => (
            <span key={c.label} title={c.title?.(r)}>
              {c.show ? c.show(r) : amount(c.value(r))}
            </span>
          ))}
        </BarRow>
      ))}
      {rows.length === 0 && <p className="muted">No pull yet</p>}
    </div>
  )
}

/** A card's row with a coloured bar across it, share (0 to 1) of the row wide; with onPick, a button. */
export function BarRow({
  share,
  color,
  grid,
  onPick,
  children,
}: {
  share: number
  color: string
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
        style={{ width: `${share * 100}%`, '--bar': color } as CSSProperties}
      />
      {children}
    </div>
  )
}
