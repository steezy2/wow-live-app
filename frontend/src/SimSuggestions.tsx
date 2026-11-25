import type { SimItem, SimSuggestReport, Suggestion } from './api'
import { full } from './format'
import { Gain, SwapRows } from './SimRows'
import { ItemLine } from './SimItem'
import { useItemInfo } from './simItems'
import { tip } from './tipAttrs'

/** A suggestion's gain: a bar and its DPS (with the sim's error on hover) when the sim tested it, the DPS the stat
 * weights expect when it is a stat, nothing for a talent. */
function SuggestionGain({ s, best }: { s: Suggestion; best: number }) {
  if (s.expected) {
    return (
      <span className="good" {...tip('From the stat weights, if the gap closed')}>
        +{full(s.expected)}
      </span>
    )
  }
  if (!s.gain) return <span />
  return (
    <span {...tip(`The sim's gain, ± ${full(s.dpsError ?? 0)} DPS`)}>
      <Gain gain={s.gain} best={best} />
    </span>
  )
}

/** What a suggestion changes: the worn item an enchant or gem goes on, or the swaps of gear and a drop (with where it
 * drops), under the words that say it. */
function Change(props: { s: Suggestion; worn?: SimItem; info: ReturnType<typeof useItemInfo> }) {
  const { s, worn, info } = props
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
    </div>
  )
}

/** A Suggest sim's report: the changes best first, each with what it changes, why and its gain, then the gain of all of
 * them together, what was left out and the note that they are a start, not an order. `equipped` is the gear the sim
 * started from (an enchant's or gem's item); items are asked `chunk` at a time. */
export function SimSuggestions(props: { report: SimSuggestReport; equipped: SimItem[]; chunk: number }) {
  const { report, equipped, chunk } = props
  const { suggestions, combined } = report
  const shown = suggestions.map((s) => ({
    s,
    worn: s.variant && s.slot ? equipped.find((i) => i.slot === s.slot) : undefined,
  }))
  const items = shown.flatMap(({ s, worn }) => [worn, ...(s.swaps ?? []).flatMap((w) => [w.out, w.in])])
  const info = useItemInfo(items.flatMap((i) => (i ? [i] : [])), chunk)
  const best = Math.max(0, combined?.gain ?? 0, ...suggestions.map((s) => s.gain ?? 0))
  return (
    <section className="panel">
      <h2>Suggestions</h2>
      {report.baseline && <p className="muted small">Compared with {report.baseline}.</p>}
      {suggestions.length === 0 && <p className="muted">No change the sim confirms.</p>}
      <ol className="sim-combos">
        {shown.map(({ s, worn }, i) => (
          <li key={i}>
            <span className="sim-rank">{i + 1}</span>
            <SuggestionGain s={s} best={best} />
            <Change s={s} worn={worn} info={info} />
          </li>
        ))}
        {combined && (
          <li className="sim-equipped-row">
            <span className="sim-rank">Σ</span>
            <Gain gain={combined.gain} dps={combined.dps} best={best} />
            <div className="sim-swaps">
              <strong>Combined</strong>
              <span className="muted small">
                {suggestions.filter((s) => s.combined).length} changes together, not the sum of the rows
              </span>
            </div>
          </li>
        )}
      </ol>
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
