import { useEffect, useState } from 'react'
import { fetchCompare, runCompare, type CompareRow, type PlayerCompare } from './api'
import { clock, rowValue } from './format'
import { Spell } from './Spell'

/** A player's review of a finished key or raid pull against the top Warcraft Logs logs of their spec and their own
 * similar runs: the verdict, then the key stats, abilities and buffs as "you" against three columns, and what to work
 * on. A stored review shows at once; the button makes one, which spends Warcraft Logs points. */
export function ComparePlayer({ kind, start, player }: { kind: 'mplus' | 'raid'; start: number; player: string }) {
  const [compare, setCompare] = useState<PlayerCompare | null | undefined>(undefined)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchCompare(kind, start, player, controller.signal).then(setCompare, (err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
    })
    return () => controller.abort()
  }, [kind, start, player])
  const run = () => {
    setBusy(true)
    setError(null)
    runCompare(kind, start, player)
      .then(setCompare, (err: unknown) => setError(err instanceof Error ? err.message : String(err)))
      .finally(() => setBusy(false))
  }
  if (compare === undefined && !error) return <p className="muted">Loading…</p>
  if (!compare) {
    return (
      <section className="panel">
        <p>
          Put {player} next to the top Warcraft Logs logs of their spec in a similar time and their own similar runs. It
          reads about 20 logs, which spends Warcraft Logs points.
        </p>
        <button disabled={busy} onClick={run}>
          {busy ? 'Reading the top logs…' : 'Run review'}
        </button>
        {busy && <div className="busy-bar" role="progressbar" aria-label="Reading the top logs" />}
        {error && <p className="warning">{error}</p>}
      </section>
    )
  }
  const earlier = compare.yourRuns === 1 ? '1 similar run' : `${compare.yourRuns} similar runs`
  return (
    <>
      <section className="panel attention">
        <h2>
          {compare.player} · {compare.spec}
        </h2>
        <p>{compare.verdict}</p>
        <p className="muted small">
          Top logs: {compare.baseline} · {compare.yourRuns > 0 ? `your usual is the median of ${earlier}` : 'no earlier similar runs'}
          {compare.points > 0 && ` · ${compare.points.toFixed(0)} Warcraft Logs points spent`}
        </p>
        {compare.parses &&
          (compare.parses.found ? (
            <p className="small">
              {compare.parse
                ? `Their Warcraft Logs parse here: best ${compare.parse.best.toFixed(0)}, median ${compare.parse.median.toFixed(0)} over ${compare.parse.runs} ranked. `
                : 'No ranked parse here. '}
              <a href={compare.parses.url} target="_blank" rel="noreferrer">
                Warcraft Logs page
              </a>
            </p>
          ) : (
            <p className="muted small">
              Not on Warcraft Logs.{' '}
              <a href={compare.parses.url} target="_blank" rel="noreferrer">
                Search there
              </a>
            </p>
          ))}
      </section>
      {compare.advice.length > 0 && (
        <section className="panel">
          <h2>What to work on</h2>
          <ol className="fix-next">
            {compare.advice.map((a) => (
              <li key={a}>{a}</li>
            ))}
          </ol>
        </section>
      )}
      <Rows title="Key stats" rows={compare.stats} />
      <Rows title="Abilities" rows={compare.abilities} spells />
      <Rows title="Buffs on themselves" rows={compare.buffs} spells />
      {(compare.onlyYou.length > 0 || compare.onlyTop.length > 0) && (
        <section className="panel">
          <h2>Only one side used (usually a talent difference)</h2>
          {compare.onlyYou.length > 0 && <p>Only {compare.player}: {compare.onlyYou.join(', ')}</p>}
          {compare.onlyTop.length > 0 && <p>Only the top logs: {compare.onlyTop.join(', ')}</p>}
        </section>
      )}
      <section className="panel">
        <h2>The {compare.logs.length} top logs</h2>
        <ul className="list">
          {compare.logs.map((l) => (
            <li key={`${l.code}-${l.fightID}`}>
              <a href={`https://www.warcraftlogs.com/reports/${l.code}#fight=${l.fightID}`} target="_blank" rel="noreferrer">
                {l.name} <span className="muted">{l.server}</span>
              </a>
              <span className="muted">
                {clock(l.durationMS)} · {Math.round(l.amount).toLocaleString()}/s
              </span>
            </li>
          ))}
        </ul>
      </section>
    </>
  )
}

/** Rows as "you" against your usual, the top logs' median and the best log. */
function Rows({ title, rows, spells = false }: { title: string; rows: CompareRow[]; spells?: boolean }) {
  if (rows.length === 0) return null
  return (
    <section className="panel">
      <h2>{title}</h2>
      <table className="compare">
        <thead>
          <tr>
            <th />
            <th>You</th>
            <th>Your usual</th>
            <th>Top median</th>
            <th>Best</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r) => (
            <tr key={`${r.id ?? 0}-${r.name}`}>
              <td>
                {spells ? <Spell name={r.name} /> : r.name}
                {r.share ? <span className="muted small"> {r.share.toFixed(0)}%</span> : null}
              </td>
              <td>{rowValue(r.you, r.unit)}</td>
              <td className="muted">{r.yours === undefined ? '–' : rowValue(r.yours, r.unit)}</td>
              <td>{rowValue(r.top, r.unit)}</td>
              <td className="muted">{rowValue(r.best, r.unit)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  )
}
