import { useEffect, useState } from 'react'
import { fetchTrack, type KeyTrack, type KeyView, type MeterPoint } from './api'
import { classColors } from './classColors'
import { amount, clock, valueAt } from './format'

/** The graph's drawing size (it scales to the panel) and its margins for the axis labels. */
const W = 1000
const H = 240
const LEFT = 56
const BOTTOM = 22
/** How far apart the graph samples, and the window each sample's rate is over. */
const STEP_MS = 5_000
const WINDOW_MS = 15_000

/** A finished key's damage (or healing) per second over the key, a line per player in class colours, with boss kills
 * and deaths marked: where the damage went up and down, from the same track the replay draws. Healing adds the party's
 * healing and damage taken per second, to see where more healing was needed. */
export function DamageGraph({ keyView: key }: { keyView: KeyView }) {
  const [track, setTrack] = useState<KeyTrack | null>(null)
  const [kind, setKind] = useState<'d' | 'h'>('d')
  useEffect(() => {
    const controller = new AbortController()
    fetchTrack(key.start, controller.signal).then(setTrack, () => undefined)
    return () => controller.abort()
  }, [key.start])
  const meters = Object.entries(track?.meters ?? {})
  if (!track || meters.length === 0) return null
  const end = key.clockMS
  const times: number[] = []
  for (let t = STEP_MS; t <= end; t += STEP_MS) times.push(t)
  const rates = (points: MeterPoint[], of: 'd' | 'h' | 'tk') =>
    times.map((t) => {
      const from = Math.max(0, t - WINDOW_MS)
      return ((valueAt(points, t, of) - valueAt(points, from, of)) * 1000) / (t - from)
    })
  const partyRates = (of: 'h' | 'tk') => {
    const each = meters.map(([, points]) => rates(points, of))
    return times.map((_, i) => each.reduce((sum, r) => sum + r[i], 0))
  }
  const lines = meters.map(([player, points]) => ({ player, rates: rates(points, kind) }))
  const taken = kind === 'h' ? partyRates('tk') : []
  const party = [
    ...(kind === 'h' ? [{ name: 'party healing', className: 'graph-healing', rates: partyRates('h') }] : []),
    ...(taken.some((v) => v > 0) ? [{ name: 'party damage taken', className: 'graph-taken', rates: taken }] : []),
  ]
  const top = Math.max(1, ...lines.flatMap((l) => l.rates), ...party.flatMap((l) => l.rates))
  const x = (t: number) => LEFT + ((W - LEFT) * t) / Math.max(1, end)
  const y = (v: number) => H - BOTTOM - ((H - BOTTOM - 8) * v) / top
  return (
    <section className="panel">
      <h2>
        {kind === 'd' ? 'Damage' : 'Healing'} per second over the key{' '}
        <span className="muted small">({WINDOW_MS / 1000} s average)</span>
      </h2>
      <div className="map-controls">
        <button aria-pressed={kind === 'd'} onClick={() => setKind('d')}>
          DPS
        </button>
        <button aria-pressed={kind === 'h'} onClick={() => setKind('h')}>
          HPS
        </button>
      </div>
      <svg
        className="graph"
        viewBox={`0 0 ${W} ${H}`}
        role="img"
        aria-label={`${kind === 'd' ? 'DPS' : 'HPS'} by player`}
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
          <text key={f} className="graph-axis" x={x(end * f)} y={H - 4} textAnchor="middle">
            {clock(end * f)}
          </text>
        ))}
        {key.bosses
          .filter((b) => b.kill && b.endMS !== undefined)
          .map((b) => (
            <line key={b.name} className="graph-boss" x1={x(b.endMS ?? 0)} x2={x(b.endMS ?? 0)} y1={0} y2={H - BOTTOM}>
              <title>{`${b.name} killed at ${clock(b.endMS ?? 0)}`}</title>
            </line>
          ))}
        {key.deaths.map((d, i) => (
          <line
            key={i}
            className="graph-death"
            x1={x(d.clockMS)}
            x2={x(d.clockMS)}
            y1={H - BOTTOM - 10}
            y2={H - BOTTOM}
          >
            <title>{`${d.player} died at ${clock(d.clockMS)}`}</title>
          </line>
        ))}
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
        {party.map((l) => (
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
      </svg>
      <p className="small">
        {lines.map((l) => (
          <span key={l.player} style={{ color: classColors[track.classes[l.player]] ?? 'var(--accent)' }}>
            ■ {l.player}{' '}
          </span>
        ))}
        {party.map((l) => (
          <span key={l.name} className={l.className}>
            ■ {l.name}{' '}
          </span>
        ))}
        <span className="muted">· boss kills in green, deaths in red</span>
      </p>
    </section>
  )
}
