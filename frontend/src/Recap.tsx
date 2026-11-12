import { useEffect, useState } from 'react'
import { fetchRecap, fetchRecapPeriods, type Recap as RecapData, type RecapPeriod } from './api'
import { CharacterParses } from './CharacterParses'
import { amount, clock, counted } from './format'

/** The Recap tab: a period picked with chips (this month by default) read as a story: tiles, the dungeons run most,
 * your character, who you ran with, facts that open their key or pull (onOpen), and the raid by boss. */
export function Recap({
  character,
  onOpen,
}: {
  character: string
  onOpen: (kind: 'mplus' | 'raid', start: number) => void
}) {
  const [periods, setPeriods] = useState<RecapPeriod[]>([])
  const [label, setLabel] = useState('')
  // The last answer, for the period it was asked for: a recap or why it failed.
  const [answer, setAnswer] = useState<{ period?: RecapPeriod; recap?: RecapData; error?: string }>({})
  const period = periods.find((p) => p.label === label) ?? periods[0]
  useEffect(() => {
    const controller = new AbortController()
    fetchRecapPeriods(controller.signal).then(setPeriods, (err: unknown) => {
      if (!controller.signal.aborted) setAnswer({ error: err instanceof Error ? err.message : String(err) })
    })
    return () => controller.abort()
  }, [])
  useEffect(() => {
    if (!period) return
    const controller = new AbortController()
    fetchRecap(period, controller.signal).then(
      (recap) => setAnswer({ period, recap }),
      (err: unknown) => {
        if (!controller.signal.aborted) setAnswer({ period, error: err instanceof Error ? err.message : String(err) })
      },
    )
    return () => controller.abort()
  }, [period])
  const current = answer.period === period ? answer : {}
  return (
    <>
      <header className="title">
        <h1>Recap</h1>
      </header>
      <div className="map-controls" role="group" aria-label="Period">
        {periods.map((p) => (
          <button key={p.label} aria-pressed={p === period} onClick={() => setLabel(p.label)}>
            {p.label}
          </button>
        ))}
      </div>
      {answer.error && (!answer.period || current.error) ? (
        <p className="warning">{answer.error}</p>
      ) : current.recap ? (
        <RecapBody recap={current.recap} character={character} onOpen={onOpen} />
      ) : (
        <p className="muted">Loading…</p>
      )}
    </>
  )
}

function RecapBody({
  recap: r,
  character,
  onOpen,
}: {
  recap: RecapData
  character: string
  onOpen: (kind: 'mplus' | 'raid', start: number) => void
}) {
  if (r.keys === 0 && r.raid.length === 0) return <p className="muted">No keys or raid pulls in this period</p>
  return (
    <>
      {r.keys > 0 && (
        <section className="panel">
          <p className="recap-tiles">
            <span>
              <b>{r.keys}</b> keys
            </span>
            <span>
              <b>{Math.round((100 * r.timed) / r.keys)}%</b> timed
            </span>
            <span>
              highest timed <b>{r.highestTimed ? `+${r.highestTimed}` : '–'}</b>
            </span>
            <span>
              <b>{(r.timeMS / 3_600_000).toFixed(1)} h</b> in keys
            </span>
            <span>
              <b>{(r.deaths / r.keys).toFixed(1)}</b> deaths a key
            </span>
          </p>
        </section>
      )}
      {r.facts.length > 0 && (
        <section className="panel">
          <h2>Facts</h2>
          <ul className="list">
            {r.facts.map((f) => (
              <li key={f.text}>
                <button className="link" onClick={() => onOpen(f.kind, f.start)}>
                  {f.text}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
      {r.dungeons.length > 0 && (
        <section className="panel">
          <h2>Most run dungeons</h2>
          <ul className="list">
            {r.dungeons.map((d) => (
              <li key={d.dungeon}>
                <span>
                  {d.dungeon}{' '}
                  <span className="muted small">
                    {counted(d.runs, 'run')}, {d.timed} timed
                  </span>
                </span>
                <span>{d.bestLevel ? `best +${d.bestLevel} in ${clock(d.bestMS)}` : 'none timed'}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {r.you && (
        <section className="panel">
          <h2>{r.you.character}</h2>
          <p className="muted">
            {r.you.specs.join(', ')} · {counted(r.you.deaths, 'death')}
            {r.you.killers.length > 0 &&
              ` · killed most by ${r.you.killers.map((k) => `${k.name} (${k.count})`).join(', ')}`}
          </p>
          <ul className="list">
            {r.you.dungeons.map((d) => (
              <li key={d.dungeon + d.metric}>
                <span>{d.dungeon}</span>
                <span>
                  median {amount(d.median)} {d.metric.toUpperCase()}
                </span>
              </li>
            ))}
          </ul>
          <CharacterParses character={character} />
        </section>
      )}
      {r.party.length > 0 && (
        <section className="panel">
          <h2>Who you ran with</h2>
          <ul className="list">
            {r.party.map((m) => (
              <li key={m.player}>
                <span>{m.player}</span>
                <span>
                  {counted(m.keys, 'key')} together, {Math.round((100 * m.timed) / m.keys)}% timed
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {r.raid.length > 0 && (
        <section className="panel">
          <h2>Raid</h2>
          <ul className="list">
            {r.raid.map((b) => (
              <li key={b.boss + b.difficulty}>
                <span>
                  {b.boss} {b.difficulty}{' '}
                  <span className="muted small">
                    {counted(b.pulls, 'pull')}, {counted(b.kills, 'kill')}
                    {b.yourDeaths > 0 && `, you died ${counted(b.yourDeaths, 'time')}`}
                    {b.mostHit && ` · hit hardest by ${b.mostHit}`}
                  </span>
                </span>
                {b.firstKill ? (
                  <button className="link" onClick={() => onOpen('raid', b.firstKill)}>
                    first kill on pull {b.firstKillPull}, {new Date(b.firstKill).toLocaleDateString()}
                  </button>
                ) : (
                  <span>{b.bestWipePct >= 0 ? `best wipe ${b.bestWipePct.toFixed(1)}%` : 'no kill'}</span>
                )}
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}
