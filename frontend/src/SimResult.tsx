import { useState } from 'react'
import type { Sim, SimCombo, SimItem, SimOptionRow, SimResult as Result, SimStatWeight, SimVariant } from './api'
import { clock, full } from './format'
import { SourceRanking } from './SimSources'
import { AbilityBars, AbilityLog, BuffBars } from './SimBreakdown'
import { ItemLine } from './SimItem'
import { useItemInfo, useSpellInfo } from './simItems'
import { simClassColor, simClassName } from './classColors'
import { groupLabel, kindLabel, unitOf } from './simOptions'
import { Gain, SwapRows } from './SimRows'
import { SimSuggestions } from './SimSuggestions'

/** The names the page gives simc's stats. */
const STAT_NAMES: Record<string, string> = {
  Int: 'Intellect',
  Str: 'Strength',
  Agi: 'Agility',
  Crit: 'Crit',
  Haste: 'Haste',
  Mastery: 'Mastery',
  Vers: 'Versatility',
}

/** The top of a report: what was simmed, the character, and the one number it comes to; a healer's says what it was
 * scored on and assumes. */
function ReportHead({ sim, result }: { sim: Sim; result: Result }) {
  const best = result.combos?.[0] ?? result.options?.[0]
  const who = [sim.spec && sim.spec[0].toUpperCase() + sim.spec.slice(1), simClassName(sim.class)]
    .filter(Boolean)
    .join(' ')
  const headline = Math.max(result.dps, best?.dps ?? 0)
  const ranked = sim.kind === 'topgear' || sim.kind === 'drops' || !!result.options
  const better = !!best && best.dps > result.dps
  const caption = !ranked
    ? result.metric === 'hps'
      ? 'From top logs'
      : `± ${full(result.dpsError)}`
    : !better
      ? 'Your current setup is the best'
      : sim.kind === 'drops'
        ? 'Best drop'
        : 'Best combination'
  return (
    <section
      className="panel sim-report-head"
      style={{ '--class': simClassColor(sim.class) ?? 'var(--accent)' } as React.CSSProperties}
    >
      <div>
        <h2>{kindLabel(sim.kind)}</h2>
        <p className="sim-who">
          <strong>{sim.character}</strong>{' '}
          <span className="muted">{who}</span>
        </p>
        {result.notes ? (
          result.notes.map((n, i) => (
            <p key={i} className="muted small">
              {n}
            </p>
          ))
        ) : (
          <p className="muted small">
            {result.suggest ? 'Stat weights on ' : ''}
            {fightText(sim.options)}
            {result.suggest?.fights && `; changes on ${result.suggest.fights.map((f) => f.name).join(' and ')}`}
          </p>
        )}
      </div>
      <div className="sim-headline">
        <span className="sim-dps">
          {full(headline)} {unitOf(result)}
        </span>
        <span className="muted">{caption}</span>
      </div>
    </section>
  )
}

/** The slots a report lays out, down the left, down the right and along the bottom, like the character panel. */
const DOLL: string[][] = [
  ['head', 'neck', 'shoulder', 'back', 'chest', 'wrist'],
  ['hands', 'waist', 'legs', 'feet', 'finger', 'trinket'],
  ['main_hand', 'off_hand'],
]

/** The gear of the best combination in full, in the character panel's layout: the equipped items with the best row's
 * swaps put on, or a line saying the equipped gear is the best. */
function BestGear({ equipped, best, chunk }: { equipped: SimItem[]; best?: SimCombo; chunk: number }) {
  const swaps = best && best.gain > 0 ? best.swaps : []
  const gear = equipped.flatMap((item) => {
    const swap = swaps.find((s) => s.slot === item.slot)
    const worn = swap ? swap.in : item
    return worn ? [worn] : []
  })
  const info = useItemInfo(gear, chunk)
  return (
    <section className="panel">
      <h2>{swaps.length > 0 ? 'Your top gear' : 'Your equipped gear is the top gear'}</h2>
      <div className="sim-doll">
        {DOLL.map((groups) => (
          <div key={groups[0]} className="sim-doll-column">
            {gear
              .filter((i) => groups.includes(i.group))
              .map((i) => (
                <div key={i.slot} className={swaps.some((s) => s.in === i) ? 'sim-doll-item changed' : 'sim-doll-item'}>
                  <span className="muted small sim-swap-slot">{groupLabel(i.group)}</span>
                  <ItemLine item={i} info={info.get(i.id)} />
                </div>
              ))}
          </div>
        ))}
      </div>
    </section>
  )
}

/** The gear combinations or drops of a Top Gear or Droptimizer sim, best first, the equipped gear a row among them at
 * its own DPS: each a row with its rank, its DPS, what it gains over the equipped gear with a bar, and what changes
 * (icons and item levels) and where a drop comes from. Drops filter by source. */
function Combos(props: { combos: SimCombo[]; equipped: number; drops: boolean; chunk: number; unit: string }) {
  const { combos, equipped, drops, chunk, unit } = props
  const [source, setSource] = useState('')
  const swapped = combos.flatMap((c) => c.swaps.flatMap((s) => [s.out, s.in]))
  const info = useItemInfo(swapped.flatMap((i) => (i ? [i] : [])), chunk)
  const sources = [...new Set(combos.flatMap((c) => (c.source ? [c.source] : [])))]
  const shown = source ? combos.filter((c) => c.source === source) : combos
  const above = shown.filter((c) => c.dps >= equipped).length
  const rows = [...shown.slice(0, above), undefined, ...shown.slice(above)] // undefined is the equipped gear's row
  const best = Math.max(0, ...shown.map((c) => c.gain))
  return (
    <section className="panel">
      <h2>{drops ? 'Upgrades from drops' : 'Gear combinations'}</h2>
      {drops && sources.length > 1 && (
        <label className="inline">
          Source{' '}
          <select value={source} onChange={(e) => setSource(e.target.value)}>
            <option value="">All</option>
            {sources.map((s) => (
              <option key={s}>{s}</option>
            ))}
          </select>
        </label>
      )}
      <ol className="sim-combos">
        {rows.map((c, i) =>
          c ? (
            <li key={i}>
              <span className="sim-rank">{i + 1}</span>
              <Gain gain={c.gain} dps={c.dps} best={best} unit={unit} />
              <div className="sim-swaps">
                {c.swaps.length === 0 && <span>Current gear</span>}
                <SwapRows swaps={c.swaps} info={info} />
                {drops && (
                  <span className="muted small">
                    {c.boss} · {c.source}
                  </span>
                )}
                {c.consumable && <span className="muted small">with {c.consumable}</span>}
              </div>
            </li>
          ) : (
            <li key="equipped" className="sim-equipped-row">
              <span className="sim-rank">{i + 1}</span>
              <Gain gain={0} dps={equipped} best={best} unit={unit} />
              <div className="sim-swaps">
                <strong>Equipped</strong> <span className="muted">Current gear</span>
              </div>
            </li>
          ),
        )}
      </ol>
    </section>
  )
}

/** A Quick Sim's compared values, best first: each a row with its rank, its DPS and a bar for its gain over the sim's
 * own setup. */
function OptionRows({ rows }: { rows: SimOptionRow[] }) {
  const best = Math.max(0, ...rows.map((r) => r.gain))
  return (
    <section className="panel">
      <h2>Compared</h2>
      <ol className="sim-combos">
        {rows.map((r, i) => (
          <li key={r.value}>
            <span className="sim-rank">{i + 1}</span>
            <Gain gain={r.gain} dps={r.dps} best={best} />
            <div className="sim-swaps">{r.name}</div>
          </li>
        ))}
      </ol>
    </section>
  )
}

/** A stat weights sim's weights, each stat a row with a bar against the main stat's, and the Pawn string to copy. */
function StatWeights({ weights, pawn, unit }: { weights: SimStatWeight[]; pawn?: string; unit: string }) {
  const [copied, setCopied] = useState(false)
  const top = Math.max(...weights.map((w) => w.weight), 1)
  const copy = () => void navigator.clipboard.writeText(pawn ?? '').then(() => setCopied(true), () => undefined)
  return (
    <section className="panel">
      <h2>Stat weights</h2>
      <p className="muted small">What a point of each rating is worth, against a point of the main stat (1.00).</p>
      <table className="review-table sim-weights">
        <thead>
          <tr>
            <th>Stat</th>
            <th>Weight</th>
            <th>{unit} per point</th>
          </tr>
        </thead>
        <tbody>
          {weights.map((w) => (
            <tr key={w.stat}>
              <td>{STAT_NAMES[w.stat] ?? w.stat}</td>
              <td
                style={{
                  backgroundImage: `linear-gradient(to right, var(--border) ${(w.weight / top) * 100}%, transparent 0)`,
                }}
              >
                {w.weight.toFixed(2)}
              </td>
              <td>{w.dps.toFixed(2)}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {pawn && (
        <div className="actions">
          <input readOnly value={pawn} aria-label="Pawn string" onFocus={(e) => e.target.select()} />
          <button onClick={copy}>{copied ? 'Copied' : 'Copy Pawn string'}</button>
        </div>
      )}
    </section>
  )
}

/** A fight in words: "Patchwerk, 300 s, 1 target". */
const fightText = (f: { fightStyle: string; length: number; targets: number }) =>
  `${f.fightStyle}, ${f.length} s, ${f.targets} target${f.targets > 1 ? 's' : ''}`

/** How the sim was run, one fact to a line: the fight (a Suggest sim's stat weights' fight, the fights it tested the
 * changes on and whose top logs), when, how long it took and how sure the number is. A healer's was scored on top
 * logs (its head says so), with no fight or error. */
function Details({ sim, result }: { sim: Sim; result: Result }) {
  const healer = result.metric === 'hps'
  const percent = result.dps ? (result.dpsError / result.dps) * 100 : 0
  const suggest = result.suggest
  const rows: [string, string][] = healer
    ? []
    : suggest
      ? [
          ['Stat weights', fightText(sim.options)],
          ...(suggest.fights ?? []).map((f): [string, string] => [`Changes on ${f.name}`, fightText(f)]),
          ...(suggest.target ? [['Top logs of', suggest.target] as [string, string]] : []),
        ]
      : [
          ['Fight style', sim.options.fightStyle],
          ['Fight length', `${sim.options.length} seconds`],
          ['Targets', String(sim.options.targets)],
        ]
  if (!healer) rows.push(['Accuracy', sim.options.accuracy])
  rows.push(['Sim run', new Date(sim.startedMS).toLocaleString()])
  if (!healer) {
    rows.push([
      suggest ? 'Margin of error (stat weights)' : 'Margin of error',
      `~${full(result.dpsError)} DPS (${percent.toFixed(2)}%)`,
    ])
  }
  if (result.stages) {
    rows.push(['Stages', result.stages.map((s) => `${s.combos} at ${s.error}%`).join(' → ')])
  }
  if (sim.tookMS) rows.push(['Processing time', clock(sim.tookMS)])
  return (
    <section className="panel">
      <h2>Simulation details</h2>
      <dl className="sim-details">
        {rows.map(([name, value]) => (
          <div key={name}>
            <dt className="muted">{name}</dt>
            <dd>{value}</dd>
          </div>
        ))}
      </dl>
    </section>
  )
}

/** A finished sim as a report: its head with the one number, the best gear (Top Gear), the ranked rows, the stat
 * weights, then the equipped setup's abilities and buffs and how the sim was run. Items are asked chunk at a time. */
export function SimResult({ sim, chunk, onTry }: { sim: Sim; chunk: number; onTry?: (v: SimVariant) => void }) {
  const result = sim.result
  const log = sim.kind === 'quick' ? (result?.log ?? []) : []
  const named = [...(result?.abilities ?? []), ...(result?.buffs ?? []), ...log, ...log.flatMap((e) => e.buffs ?? [])]
  const spells = useSpellInfo([...new Set(named.map((n) => n.id).filter(Boolean))], chunk)
  if (!result) return null
  return (
    <>
      <ReportHead sim={sim} result={result} />
      {sim.kind === 'topgear' && result.equipped && (
        <BestGear equipped={result.equipped} best={result.combos?.[0]} chunk={chunk} />
      )}
      {sim.kind === 'drops' && result.combos && (
        <SourceRanking combos={result.combos} equipped={result.dps} chunk={chunk} unit={unitOf(result)} />
      )}
      {result.combos && (
        <Combos
          combos={result.combos}
          equipped={result.dps}
          drops={sim.kind === 'drops'}
          chunk={chunk}
          unit={unitOf(result)}
        />
      )}
      {result.options && <OptionRows rows={result.options} />}
      {result.suggest && (
        <SimSuggestions report={result.suggest} equipped={result.equipped ?? []} chunk={chunk} onTry={onTry} />
      )}
      {result.weights && <StatWeights weights={result.weights} pawn={result.pawn} unit={unitOf(result)} />}
      <AbilityBars abilities={result.abilities} info={spells} unit={unitOf(result)} />
      {result.buffs.length > 0 && <BuffBars buffs={result.buffs} info={spells} />}
      {log.length > 0 && <AbilityLog log={log} info={spells} />}
      <Details sim={sim} result={result} />
    </>
  )
}
