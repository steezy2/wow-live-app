import { useState } from 'react'
import type { SimItem, SimSuggestReport, SimVariant, Suggestion } from './api'
import { full } from './format'
import { Gain, SwapRows } from './SimRows'
import { ItemLine } from './SimItem'
import { useItemInfo } from './simItems'
import { type RankBy, rankSuggestions } from './simOptions'
import { tip } from './tipAttrs'

const RANKS: [RankBy, string][] = [
  ['best', 'Best'],
  ['single', '1 target'],
  ['aoe', 'AoE'],
]

/** The fights' gains of a suggestion or the combined row: on one target and on several, each with the sim's error. */
type Gains = Pick<Suggestion, 'gain' | 'dpsError' | 'aoeGain' | 'aoeError'>

/** A change's gain on one target and on AoE side by side, each a bar and its DPS (with the sim's error on hover, or
 * that the sim shows none); `dps` is the combined row's. */
function GainPair({ g, best, dps }: { g: Gains; best: number; dps?: number }) {
  const fights = [
    ['1 target', g.gain, g.dpsError],
    ['AoE', g.aoeGain, g.aoeError],
  ] as const
  return (
    <div className="sim-gains">
      {fights.map(([label, gain, error], i) => (
        <span
          key={label}
          {...tip(
            gain ? `The sim's gain on ${label}, ± ${full(error ?? 0)} DPS` : `No gain on ${label} over the sim's error`,
          )}
        >
          <span className="muted small">{label}</span>
          <Gain gain={gain ?? 0} dps={i === 0 ? dps : undefined} best={best} />
        </span>
      ))}
    </div>
  )
}

/** A suggestion's gains when the sim tested it, the DPS the stat weights expect when it is a stat, nothing for a
 * talent swap with no DPS change. */
function SuggestionGain({ s, best }: { s: Suggestion; best: number }) {
  if (s.expected) {
    return (
      <span className="good" {...tip('From the stat weights, if the gap closed')}>
        +{full(s.expected)}
      </span>
    )
  }
  return s.gain || s.aoeGain ? <GainPair g={s} best={best} /> : <span />
}

/** What a suggestion changes: the worn item an enchant or gem goes on, or the swaps of gear and a drop (with where it
 * drops), under the words that say it. */
function Change(props: {
  s: Suggestion
  worn?: SimItem
  info: ReturnType<typeof useItemInfo>
  onTry?: (v: SimVariant) => void
}) {
  const { s, worn, info, onTry } = props
  return (
    <div className="sim-swaps">
      <span>{s.what}</span>
      {worn && <ItemLine item={worn} info={info.get(worn.id)} />}
      {s.swaps && <SwapRows swaps={s.swaps} info={info} />}
      {s.boss && (
        <span className="muted small">
          {s.boss} · {s.source}
        </span>
      )}
      <span className="muted small">{s.evidence}</span>
      {s.variant && onTry && (
        <button className="link" onClick={() => onTry(s.variant!)}>
          Try it in Top Gear
        </button>
      )}
    </div>
  )
}

/** A Suggest sim's report: the changes best first, each with what it changes, why and its gain (and, for an enchant or
 * gem, a button that has `onTry` open it in Top Gear), then the gain of all of them together, the talents top players
 * take that add no DPS in a closed group (a loss shown), what was left out and the note that they are a start, not an
 * order. `equipped` is the gear the sim started from (an enchant's or gem's item); items are asked `chunk` at a
 * time. */
export function SimSuggestions(props: {
  report: SimSuggestReport
  equipped: SimItem[]
  chunk: number
  onTry?: (v: SimVariant) => void
}) {
  const { report, equipped, chunk, onTry } = props
  const { suggestions, combined } = report
  const [by, setBy] = useState<RankBy>('best')
  const shown = rankSuggestions(suggestions, by).map((s, key) => ({
    s,
    key,
    worn: s.variant && s.slot ? equipped.find((i) => i.slot === s.slot) : undefined,
  }))
  const items = shown.flatMap(({ s, worn }) => [worn, ...(s.swaps ?? []).flatMap((w) => [w.out, w.in])])
  const info = useItemInfo(items.flatMap((i) => (i ? [i] : [])), chunk)
  const best = Math.max(0, ...[combined, ...suggestions].flatMap((g) => [g?.gain ?? 0, g?.aoeGain ?? 0]))
  const main = shown.filter(({ s }) => s.kind !== 'talentOther')
  const other = shown.filter(({ s }) => s.kind === 'talentOther')
  const row = ({ s, key, worn }: (typeof shown)[number], rank: number | string) => (
    <li key={key}>
      <span className="sim-rank">{rank}</span>
      <SuggestionGain s={s} best={best} />
      <Change s={s} worn={worn} info={info} onTry={onTry} />
    </li>
  )
  return (
    <section className="panel">
      <h2>Suggestions</h2>
      {report.baseline && <p className="muted small">Compared with {report.baseline}.</p>}
      {main.length === 0 && <p className="muted">No change the sim confirms.</p>}
      <div className="sim-tabs" role="group" aria-label="Rank the changes by">
        {RANKS.map(([k, label]) => (
          <button
            key={k}
            aria-pressed={by === k}
            onClick={() => setBy(k)}
            {...tip(k === 'best' ? 'The larger of the two gains' : `Rank by the gain on ${label}`)}
          >
            {label}
          </button>
        ))}
      </div>
      <ol className="sim-combos sim-suggest">
        {main.map((r, i) => row(r, i + 1))}
        {combined && (
          <li className="sim-equipped-row">
            <span className="sim-rank">Σ</span>
            <GainPair g={combined} best={best} dps={combined.dps} />
            <div className="sim-swaps">
              <strong>Combined</strong>
              <span className="muted small">
                {suggestions.filter((s) => s.combined).length} changes together, not the sum of the rows
              </span>
            </div>
          </li>
        )}
      </ol>
      {other.length > 0 && (
        <details>
          <summary>Other talents ({other.length})</summary>
          <p className="muted small">No DPS gain in the sim: these are what top players take for utility.</p>
          <ol className="sim-combos sim-suggest">{other.map((r) => row(r, '·'))}</ol>
        </details>
      )}
      {report.dropped > 0 && (
        <p className="muted small">
          {report.dropped} tested {report.dropped > 1 ? 'changes were' : 'change was'} left out: no gain over the sim's
          error.
        </p>
      )}
      {(report.notes ?? []).map((n) => (
        <p key={n} className="muted small">
          {n}
        </p>
      ))}
      <p className="muted small">
        Suggestions are what top players do plus what the sim confirms; check it fits your playstyle.
      </p>
    </section>
  )
}
