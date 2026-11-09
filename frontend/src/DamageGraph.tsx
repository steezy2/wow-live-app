import { useEffect, useState, type MouseEvent } from 'react'
import { fetchTrack, type KeyTrack, type KeyView, type MeterPoint } from './api'
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

/** A finished key's damage (or healing) graph, from its track. */
export function DamageGraph({ keyView: key }: { keyView: KeyView }) {
  const [track, setTrack] = useState<KeyTrack | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchTrack(key.start, controller.signal).then(setTrack, () => undefined)
    return () => controller.abort()
  }, [key.start])
  if (!track) return null
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
  return <MeterGraph track={track} end={key.clockMS} marks={marks} group="party" over="key" />
}

/** Damage (or healing) per second over a key or a raid pull, a line per player in class colours, or the group's total,
 * or one player (click a name), with boss kills and deaths marked: where the damage went up and down. Healing adds the
 * group's healing and damage taken per second, to see where more healing was needed. With onSeek, a click on the
 * graph moves the replay there, and now is drawn as a line. */
export function MeterGraph(props: {
  track: KeyTrack
  end: number
  marks: SliderMark[]
  group: 'party' | 'raid'
  over: 'key' | 'pull'
  now?: number
  onSeek?: (clockMS: number) => void
}) {
  const { track, end, marks, group, over, now, onSeek } = props
  const [kind, setKind] = useState<'d' | 'h'>('d')
  const [only, setOnly] = useState('')
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
  const lines = (only === TOTAL ? [] : meters.filter(([p]) => !only || p === only)).map(([player, points]) => ({
    player,
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
  const seek = (e: MouseEvent<SVGSVGElement>) => {
    const box = e.currentTarget.getBoundingClientRect()
    const t = (((e.clientX - box.left) * W) / box.width - LEFT) / (W - LEFT)
    onSeek?.(Math.round(Math.min(1, Math.max(0, t)) * end))
  }
  const pick = (name: string) => setOnly(only === name ? '' : name)
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
        <button aria-pressed={only === TOTAL} onClick={() => pick(TOTAL)}>
          {group === 'raid' ? 'Raid' : 'Party'} total
        </button>
        {only && only !== TOTAL && <button onClick={() => setOnly('')}>Everyone</button>}
      </div>
      <svg
        className={onSeek ? 'graph seekable' : 'graph'}
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={`${kind === 'd' ? 'DPS' : 'HPS'} by player`}
        onClick={onSeek && seek}
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
        {lines.map((l) => (
          <polyline
            key={l.player}
            fill="none"
            stroke={classColors[track.classes[l.player]] ?? 'var(--accent)'}
            strokeWidth={2}
            points={l.rates.map((v, i) => `${x(times[i])},${y(v)}`).join(' ')}
          >
            <title>{l.player}</title>
          </polyline>
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
      </svg>
      <p className="small graph-legend">
        {meters.map(([player]) => (
          <button
            key={player}
            aria-pressed={only === player}
            style={{ color: classColors[track.classes[player]] ?? 'var(--accent)' }}
            onClick={() => pick(player)}
          >
            ■ {player}
          </button>
        ))}
        {totals.map((l) => (
          <span key={l.name} className={l.className}>
            ■ {l.name}{' '}
          </span>
        ))}
        <span className="muted">· click a name for just them · boss kills in green, deaths in red</span>
      </p>
    </section>
  )
}
