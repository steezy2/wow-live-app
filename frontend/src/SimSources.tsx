import { iconURL, type SimCombo } from './api'
import { useItemInfo } from './simItems'
import { tip } from './tipAttrs'

/** A drop's gain, as a share of the equipped DPS, that makes it worth showing in colour (Raidbots' 0.05%). */
const NOTABLE = 0.05

/** A source's priority from its best drop's gain (% of the equipped DPS): 1 when it is a clear upgrade, 2 when it is
 * any, none when no drop beats the equipped gear. */
const priority = (best: number) => (best >= 0.2 ? '1' : best > 0 ? '2' : '–')

/** One boss or dungeon's drops: each item once (at its best gain across compared consumables) and what they come to. */
interface Source {
  boss: string
  source: string
  drops: { combo: SimCombo; gain: number }[]
  best: number
  expected: number
}

/** The drops grouped by the boss that drops them, with the best gain of each and the expected one if each of the
 * boss's drops were as likely as the others. Best source first. */
function bySource(combos: SimCombo[]): Source[] {
  const groups = new Map<string, Map<string, SimCombo>>()
  for (const c of combos) {
    const swap = c.swaps[0]
    if (!c.boss || !swap?.in) continue
    const key = `${c.source}\n${c.boss}`
    const items = groups.get(key) ?? new Map<string, SimCombo>()
    const id = `${swap.in.id}-${swap.slot}`
    if (!items.has(id) || c.gain > (items.get(id)?.gain ?? -Infinity)) items.set(id, c)
    groups.set(key, items)
  }
  return [...groups.values()]
    .map((items): Source => {
      const drops = [...items.values()].map((combo) => ({ combo, gain: combo.gain })).sort((a, b) => b.gain - a.gain)
      const positive = drops.map((d) => Math.max(0, d.gain))
      return {
        boss: drops[0].combo.boss ?? '',
        source: drops[0].combo.source ?? '',
        drops,
        best: positive[0],
        expected: positive.reduce((sum, g) => sum + g, 0) / drops.length,
      }
    })
    .sort((a, b) => b.best - a.best || b.expected - a.expected)
}

/** A Droptimizer's bosses and dungeons ranked by the best upgrade they drop, each with all its drops as icons under
 * their gain (% of the equipped DPS), the gain if a drop were picked at random from it and its best. */
export function SourceRanking(props: { combos: SimCombo[]; equipped: number; chunk: number; unit?: string }) {
  const { combos, equipped, chunk, unit = 'DPS' } = props
  const sources = bySource(combos)
  const info = useItemInfo(
    sources.flatMap((s) => s.drops.flatMap((d) => (d.combo.swaps[0].in ? [d.combo.swaps[0].in] : []))),
    chunk,
  )
  if (sources.length < 2) return null
  const pct = (gain: number) => (equipped > 0 ? (gain / equipped) * 100 : 0)
  const num = (n: number) => `${n > 0 ? '+' : ''}${Math.round(n).toLocaleString('en-US')}`
  return (
    <section className="panel sim-sources">
      <h2>Best places to get upgrades</h2>
      <p className="muted small">
        Each boss and dungeon with what it drops, ranked by its best upgrade. Gains are against your current gear;
        coloured items are a 0.05% {unit} gain or better. Expected is the gain of one drop picked at random.
      </p>
      <ol>
        {sources.map((s) => (
          <li key={`${s.source}${s.boss}`} className="sim-source">
            <div className="sim-source-name">
              <strong>{s.boss}</strong>
              {s.source !== s.boss && <span className="muted small">{s.source}</span>}
            </div>
            <div className="sim-source-drops">
              {s.drops.map(({ combo, gain }) => {
                const item = combo.swaps[0].in
                const known = item ? info.get(item.id) : undefined
                const p = pct(gain)
                return (
                  <span
                    key={`${item?.id}-${combo.swaps[0].slot}`}
                    className={`sim-source-drop${p >= NOTABLE ? ' good' : ''}`}
                    {...tip(`${known?.name ?? item?.name ?? ''} ${item?.level ?? ''}`.trim(), `${num(gain)} ${unit}`)}
                  >
                    <span className="sim-icon">
                      {known?.icon && <img src={iconURL(known.icon)} alt="" loading="lazy" />}
                    </span>
                    <span className="small">{`${p > 0 ? '' : p < 0 ? '-' : ''}${Math.abs(p).toFixed(1)}%`}</span>
                  </span>
                )
              })}
            </div>
            <div className="sim-source-num">
              <strong className={s.expected > 0 ? 'good' : 'muted'}>{num(s.expected)}</strong>
              <span className="muted small">Expected</span>
            </div>
            <div className="sim-source-num">
              <strong className={s.best > 0 ? 'good' : 'muted'}>{num(s.best)}</strong>
              <span className="muted small">Best</span>
            </div>
            <div className="sim-source-num">
              <strong>{priority(pct(s.best))}</strong>
              <span className="muted small">Priority</span>
            </div>
          </li>
        ))}
      </ol>
    </section>
  )
}
