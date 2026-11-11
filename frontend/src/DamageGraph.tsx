import { useState, type MouseEvent } from 'react'
import { type KeyTrack, type KeyView, type MeterPoint } from './api'
import { classColors } from './classColors'
import { amount, clock, valueAt } from './format'
import type { SliderMark } from './replay'

/** The graph's drawing size (it scales to the panel) and its margins for the axis labels. */
const W = 1000
const H = 240
const LEFT = 56
const BOTTOM = 22
/** How far apart the graph samples, and the window each sample's rate is over. */
const STEP_MS = 5_000
const WINDOW_MS = 15_000
/** The graph's line choice for the group's total, not one player. */
const TOTAL = ' total'
/** A line this close to the pointer (in the drawing's units) is the one under it. */
const HOVER_REACH = 14

/** A finished key's damage (or healing) graph over its track, with boss kills and deaths marked; a click moves the
 * replay there (onSeek) and now is drawn as a line. */
export function DamageGraph(props: {
  keyView: KeyView
  track: KeyTrack
  now: number
  onSeek: (clockMS: number) => void
}) {
  const { keyView: key, track, now, onSeek } = props
  const marks: SliderMark[] = [
    ...key.bosses
      .filter((b) => b.kill && b.endMS !== undefined)
      .map((b) => ({ clockMS: b.endMS ?? 0, className: 'boss', title: `${b.name} killed at ${clock(b.endMS ?? 0)}` })),
    ...key.deaths.map((d) => ({
      clockMS: d.clockMS,
      className: 'death',
      title: `${d.player} died at ${clock(d.clockMS)}`,
    })),
  ]
  return (
    <MeterGraph track={track} end={key.clockMS} marks={marks} group="party" over="key" now={now} onSeek={onSeek} />
  )
}

/** Damage (or healing) per second over a key or a raid pull, a line per player in class colours, or the group's total,
 * or one player, with boss kills and deaths marked: where the damage went up and down. Healing adds the group's
 * healing and damage taken per second, to see where more healing was needed. The line under the pointer comes to the
 * front with its player's name and number; a click on it (or on a name) shows just that player, and a right click
 * everyone again. With onSeek, a click anywhere else moves the replay there, and now is drawn as a line. With
 * onFocus, the page keeps the one player shown (focus, '' for everyone). */
export function MeterGraph(props: {
  track: KeyTrack
  end: number
  marks: SliderMark[]
  group: 'party' | 'raid'
  over: 'key' | 'pull'
  now?: number
  onSeek?: (clockMS: number) => void
  focus?: string
  onFocus?: (player: string) => void
}) {
  const { track, end, marks, group, over, now, onSeek, focus, onFocus } = props
  const [kind, setKind] = useState<'d' | 'h'>('d')
  const [own, setOwn] = useState('')
  const [total, setTotal] = useState(false)
  const [hover, setHover] = useState<{ player: string; i: number } | null>(null)
  const player = onFocus ? (focus ?? '') : own
  const setPlayer = onFocus ?? setOwn
  const only = player || (total ? TOTAL : '')
  const meters = Object.entries(track.meters ?? {})
  if (meters.length === 0) return null
  const times: number[] = []
  for (let t = STEP_MS; t <= end; t += STEP_MS) times.push(t)
  const rates = (points: MeterPoint[], of: 'd' | 'h' | 'tk') =>
    times.map((t) => {
      const from = Math.max(0, t - WINDOW_MS)
      return ((valueAt(points, t, of) - valueAt(points, from, of)) * 1000) / (t - from)
    })
  const groupRates = (of: 'd' | 'h' | 'tk') => {
    const each = meters.map(([, points]) => rates(points, of))
    return times.map((_, i) => each.reduce((sum, r) => sum + r[i], 0))
  }
  const lines = (only === TOTAL ? [] : meters.filter(([p]) => !only || p === only)).map(([name, points]) => ({
    player: name,
    color: classColors[track.classes[name]] ?? 'var(--accent)',
    rates: rates(points, kind),
  }))
  const totals: { name: string; className: string; rates: number[] }[] = []
  const whole = only === '' || only === TOTAL
  if (only === TOTAL || (kind === 'h' && whole)) {
    const what = kind === 'd' ? 'damage' : 'healing'
    totals.push({ name: `${group} ${what}`, className: `graph-${what}`, rates: groupRates(kind) })
  }
  const taken = kind === 'h' && whole ? groupRates('tk') : []
  if (taken.some((v) => v > 0)) totals.push({ name: `${group} damage taken`, className: 'graph-taken', rates: taken })
  const top = Math.max(1, ...lines.flatMap((l) => l.rates), ...totals.flatMap((l) => l.rates))
  const x = (t: number) => LEFT + ((W - LEFT) * t) / Math.max(1, end)
  const y = (v: number) => H - BOTTOM - ((H - BOTTOM - 8) * v) / top
  /** Where the pointer is on the drawing: the time under it, and how far down. */
  const point = (e: MouseEvent<SVGSVGElement>): [number, number] => {
    const box = e.currentTarget.getBoundingClientRect()
    const t = (((e.clientX - box.left) * W) / box.width - LEFT) / (W - LEFT)
    return [Math.min(1, Math.max(0, t)) * end, ((e.clientY - box.top) * H) / box.height]
  }
  /** The line under the pointer, and which of its samples. */
  const under = (e: MouseEvent<SVGSVGElement>) => {
    const [t, down] = point(e)
    const i = Math.min(times.length - 1, Math.max(0, Math.round(t / STEP_MS) - 1))
    const far = (l: (typeof lines)[number]) => Math.abs(y(l.rates[i]) - down)
    const [nearest] = lines.filter((l) => far(l) <= HOVER_REACH).sort((a, b) => far(a) - far(b))
    return nearest ? { player: nearest.player, i } : null
  }
  const click = (e: MouseEvent<SVGSVGElement>) => {
    const line = under(e)
    if (line && lines.length > 1) setPlayer(line.player)
    else onSeek?.(Math.round(point(e)[0]))
  }
  const everyone = () => {
    setPlayer('')
    setTotal(false)
  }
  const hovered = hover && lines.find((l) => l.player === hover.player)
  const pointing = hovered && lines.length > 1
  return (
    <section className="panel">
      <h2>
        {kind === 'd' ? 'Damage' : 'Healing'} per second over the {over}{' '}
        <span className="muted small">({WINDOW_MS / 1000} s average)</span>
      </h2>
      <div className="map-controls">
        <button aria-pressed={kind === 'd'} onClick={() => setKind('d')}>
          DPS
        </button>
        <button aria-pressed={kind === 'h'} onClick={() => setKind('h')}>
          HPS
        </button>
        <button
          aria-pressed={only === TOTAL}
          onClick={() => {
            setTotal(only !== TOTAL)
            setPlayer('')
          }}
        >
          {group === 'raid' ? 'Raid' : 'Party'} total
        </button>
        {player && <button onClick={everyone}>Everyone</button>}
      </div>
      <svg
        className={`graph${onSeek ? ' seekable' : ''}${pointing ? ' pointing' : ''}`}
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={`${kind === 'd' ? 'DPS' : 'HPS'} by player`}
        onClick={click}
        onMouseMove={(e) => {
          const line = under(e)
          setHover((h) => (h?.player === line?.player && h?.i === line?.i ? h : line))
        }}
        onMouseLeave={() => setHover(null)}
        onContextMenu={(e) => {
          e.preventDefault()
          everyone()
        }}
      >
        {[0, 0.5, 1].map((f) => (
          <g key={f} className="graph-axis">
            <line x1={LEFT} x2={W} y1={y(top * f)} y2={y(top * f)} />
            <text x={LEFT - 6} y={y(top * f) + 4} textAnchor="end">
              {amount(top * f)}
            </text>
          </g>
        ))}
        {[0, 0.25, 0.5, 0.75, 1].map((f) => (
          <text key={f} className="graph-axis" x={x(end * f)} y={H - 4} textAnchor={f === 1 ? 'end' : 'middle'}>
            {clock(end * f)}
          </text>
        ))}
        {marks.map((m, i) =>
          m.className === 'boss' ? (
            <line key={i} className="graph-boss" x1={x(m.clockMS)} x2={x(m.clockMS)} y1={0} y2={H - BOTTOM}>
              <title>{m.title}</title>
            </line>
          ) : (
            <line
              key={i}
              className="graph-death"
              x1={x(m.clockMS)}
              x2={x(m.clockMS)}
              y1={H - BOTTOM - 10}
              y2={H - BOTTOM}
            >
              <title>{m.title}</title>
            </line>
          ),
        )}
        {[...lines]
          .sort((a, b) => Number(a === hovered) - Number(b === hovered))
          .map((l) => (
            <polyline
              key={l.player}
              fill="none"
              stroke={l.color}
              strokeWidth={l === hovered ? 3.5 : 2}
              opacity={pointing && l !== hovered ? 0.3 : 1}
              points={l.rates.map((v, i) => `${x(times[i])},${y(v)}`).join(' ')}
            />
          ))}
        {totals.map((l) => (
          <polyline
            key={l.name}
            className={l.className}
            fill="none"
            strokeWidth={3}
            points={l.rates.map((v, i) => `${x(times[i])},${y(v)}`).join(' ')}
          >
            <title>{l.name}</title>
          </polyline>
        ))}
        {now !== undefined && <line className="graph-now" x1={x(now)} x2={x(now)} y1={0} y2={H - BOTTOM} />}
        {hover && hovered && (
          <g className="graph-hover" fill={hovered.color}>
            <circle cx={x(times[hover.i])} cy={y(hovered.rates[hover.i])} r={4} />
            <text x={W - 4} y={14} textAnchor="end">
              {hovered.player} · {amount(hovered.rates[hover.i])} {kind === 'd' ? 'DPS' : 'HPS'} at{' '}
              {clock(times[hover.i])}
            </text>
          </g>
        )}
      </svg>
      <p className="small graph-legend">
        {meters.map(([name]) => (
          <button
            key={name}
            aria-pressed={player === name}
            style={{ color: classColors[track.classes[name]] ?? 'var(--accent)' }}
            onClick={() => setPlayer(player === name ? '' : name)}
          >
            ■ {name}
          </button>
        ))}
        {totals.map((l) => (
          <span key={l.name} className={l.className}>
            ■ {l.name}{' '}
          </span>
        ))}
        <span className="muted">
          · point at a line for its name · click it, or a name, for just them · right click for everyone · boss kills
          in green, deaths in red
        </span>
      </p>
    </section>
  )
}
