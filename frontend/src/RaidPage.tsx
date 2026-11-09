import { useEffect, useState } from 'react'
import {
  fetchRaidNights,
  fetchRaidPull,
  type KeyMeter,
  type RaidNight,
  type RaidPull,
  type RaidPullSummary,
} from './api'
import { classColors } from './classColors'
import { amount, clock, coverText, progressText } from './format'
import { defense, raidOffense, type Column } from './meterColumns'
import { MeterCard } from './MeterCards'
import { RaidReplay } from './RaidReplay'
import { topRows } from './replay'
import { useRaidLive } from './useLiveKey'

/** A pull's title: "Ula'tek Mythic · pull 23". */
function pullName(p: RaidPullSummary): string {
  return `${p.boss} ${p.difficulty} · pull ${p.number}`
}

/** The Raid tab: tonight's and earlier nights' pulls on the side, by boss, and the last pull (or the one picked) as
 * the overview, read again as pulls end. */
export function RaidPage() {
  const live = useRaidLive()
  const [selected, setSelected] = useState<number | null>(null)
  const [nights, setNights] = useState<RaidNight[]>([])
  const lastStart = live?.last?.start
  useEffect(() => {
    const controller = new AbortController()
    fetchRaidNights(controller.signal).then(setNights, () => undefined)
    return () => controller.abort()
  }, [lastStart])
  return (
    <div className="layout">
      <aside className="side">
        <button className={selected === null ? 'item active' : 'item'} onClick={() => setSelected(null)}>
          Last pull
        </button>
        {nights.map((n) => (
          <section key={n.night}>
            <h2>{new Date(`${n.night}T12:00`).toLocaleDateString()}</h2>
            {byBoss(n.pulls).map(([boss, pulls]) => (
              <div key={boss} className="boss-pulls">
                <h3 className="muted small">{boss}</h3>
                {pulls.map((p) => (
                  <button
                    key={p.start}
                    className={selected === p.start ? 'item pull-row active' : 'item pull-row'}
                    onClick={() => setSelected(p.start)}
                  >
                    <span>#{p.number}</span>
                    <span className="muted">{clock(p.durationMS)}</span>
                    <span className={p.kill ? 'good' : ''}>{progressText(p)}</span>
                  </button>
                ))}
              </div>
            ))}
          </section>
        ))}
        {nights.length === 0 && <p className="muted small">No raid pulls yet: each one is saved here as it ends.</p>}
      </aside>
      <main className="page">
        {live?.pulling && (
          <p className="panel">
            Pulling <b>{live.boss}</b> now · the overview of this pull shows when it ends.
          </p>
        )}
        {selected !== null ? (
          <SavedPull start={selected} />
        ) : live?.last ? (
          <RaidPullView pull={live.last} />
        ) : (
          <section className="waiting">
            <h1>Waiting for the first raid pull</h1>
            <p className="muted">
              Each pull shows here when it ends, with combat logging on (Advanced Combat Logging, then /combatlog).
            </p>
          </section>
        )}
      </main>
    </div>
  )
}

/** A night's pulls by boss and difficulty, in the order each was first pulled. */
function byBoss(pulls: RaidPullSummary[]): [string, RaidPullSummary[]][] {
  const groups = new Map<string, RaidPullSummary[]>()
  for (const p of pulls) {
    const name = `${p.boss} ${p.difficulty}`
    groups.set(name, [...(groups.get(name) ?? []), p])
  }
  return [...groups]
}

/** A saved pull, read once. */
function SavedPull({ start }: { start: number }) {
  const [pull, setPull] = useState<RaidPull | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchRaidPull(start, controller.signal)
      .then(setPull)
      .catch((err: unknown) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
      })
    return () => controller.abort()
  }, [start])
  if (error) return <p className="warning">{error}</p>
  return pull ? <RaidPullView pull={pull} /> : <p className="muted">Loading…</p>
}

/** The comparison line: tonight's best and the best ever, the same pull named once. */
function Comparisons({ pull }: { pull: RaidPull }) {
  const { tonight, ever } = pull.comparisons ?? {}
  if (!tonight && !ever) return <p className="muted">The best pull on this boss so far.</p>
  const same = tonight && ever && tonight.start === ever.start
  return (
    <p className="muted">
      {tonight && `Best tonight: pull ${tonight.number}, ${progressText(tonight)}`}
      {tonight && ever && !same && ' · '}
      {ever && !same && `best ever: ${progressText(ever)} (${new Date(ever.start).toLocaleDateString()})`}
      {same && ', the best ever too'}
    </p>
  )
}

/** A raider's name in their class colour. */
function Name({ player, classes }: { player: string; classes: Record<string, string> }) {
  return (
    <b className="name" style={{ color: classColors[classes[player]] }}>
      {player}
    </b>
  )
}

/** One pull's overview: how far it got against the best pulls, what to fix next, the replay with its mechanic lanes
 * and graphs, the early deaths, what went wrong, and the meters. */
export function RaidPullView({ pull }: { pull: RaidPull }) {
  const early = pull.deaths.filter((d) => d.early)
  const classes = Object.fromEntries(pull.meter.rows.map((r) => [r.player, r.class]))
  return (
    <>
      <section className="raid-head">
        <h1>
          {pullName(pull)} <span className={pull.kill ? 'good' : 'bad'}>{progressText(pull)}</span>
        </h1>
        <p>
          {clock(pull.durationMS)} · {pull.deaths.length} deaths · {pull.players.length} raiders
          {pull.lusts.length > 0 ? ` · Bloodlust at ${pull.lusts.map(clock).join(', ')}` : ' · no Bloodlust'}
        </p>
        <Comparisons pull={pull} />
      </section>

      {pull.suggestions.length > 0 && (
        <section className="panel attention">
          <h2>Fix next pull</h2>
          <ol className="fix-next">
            {pull.suggestions.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        </section>
      )}

      <RaidReplay key={pull.start} pull={pull} />

      <div className="raid-columns">
        <section className="panel">
          <h2>
            Early deaths {early.length} <span className="muted small">of {pull.deaths.length}</span>
          </h2>
          <ul className="list">
            {early.map((d, i) => (
              <li key={`${d.player}-${d.clockMS}-${i}`} className="stack">
                <span>
                  <span className="muted">{clock(d.clockMS)}</span> <Name player={d.player} classes={classes} />{' '}
                  {d.ability || 'unknown'}
                  {d.killer && <span className="muted"> from {d.killer}</span>}
                </span>
                <span className="muted small">
                  {coverText(d.defensive, d.ready ?? [])}
                  {d.recap.length > 0 &&
                    ` · before: ${d.recap.map((h) => `${h.ability} ${amount(h.amount)}`).join(', ')}`}
                </span>
              </li>
            ))}
            {early.length === 0 && <li className="muted">None</li>}
          </ul>
          {pull.massKills.map((m) => (
            <p key={`${m.ability}-${m.clockMS}`} className="warning">
              {m.ability} killed {m.killed} at {clock(m.clockMS)}
            </p>
          ))}
        </section>
        <Mechanics pull={pull} />
      </div>

      <section className="cards">
        <MeterCard title="Pull" meter={pull.meter} columns={nonZero(raidOffense, pull.meter)} />
        <MeterCard title="Pull" meter={pull.meter} columns={nonZero(defense(true), pull.meter)} />
      </section>
    </>
  )
}

/** A card's columns without those every raider has 0 in, the first (the one it sorts by) always kept. */
function nonZero(columns: Column[], meter: KeyMeter): Column[] {
  return columns.filter((c, i) => i === 0 || meter.rows.some((r) => c.value(r) !== 0))
}

/** What went wrong by kind of mechanic, one row each: what, then who and how much; a kind with nothing to show is
 * left out. */
function Mechanics({ pull }: { pull: RaidPull }) {
  const counts = (players: { player: string; count: number }[]) =>
    players
      .slice(0, 6)
      .map((p) => `${p.player} ${p.count}`)
      .join(', ')
  const lines: [string, string][] = [
    ...pull.avoidable.map((a): [string, string] => [`${a.ability} hits`, counts(a.players)]),
    ...pull.soaks.map((s): [string, string] => [
      `${s.ability} soaks`,
      `${s.casts} soaks, ${s.fewest}–${s.most} raiders each${s.died.length ? ` · died: ${s.died.join(', ')}` : ''}`,
    ]),
    ...pull.stacks.map((s): [string, string] => [
      `${s.ability} stacks${s.limit ? ` (over ${s.limit} is too many)` : ''}`,
      counts(s.players.filter((p) => !s.limit || p.count > s.limit)) || 'nobody over the limit',
    ]),
    ...pull.dispels.map((d): [string, string] => [
      `${d.spell} on ${d.player}`,
      `${d.by ? `dispelled by ${d.by}` : 'ran out'} after ${(d.afterMS / 1000).toFixed(1)} s`,
    ]),
    ...pull.wentOff.map((s): [string, string] => [`${s.spell} went off`, `${s.mob} at ${clock(s.clockMS)}`]),
    ...pull.adds.map((a): [string, string] => [
      a.name,
      `${a.killed} of ${a.spawned} killed${a.aliveEnd ? `, ${a.aliveEnd} still up at the end` : ''} · longest ` +
        clock(a.longestMS),
    ]),
    ...pull.priority.map((p): [string, string] => [`Damage into ${p.target}`, amount(p.damage)]),
  ]
  return (
    <section className="panel">
      <h2>Mechanics</h2>
      <ul className="list">
        {lines.map(([what, detail], i) => (
          <li key={i} className="stack">
            <b>{what}</b>
            <span className="muted small">{detail}</span>
          </li>
        ))}
        {lines.length === 0 && <li className="muted">Nothing flagged on this boss</li>}
      </ul>
    </section>
  )
}

/** The overlay's card between pulls: how far the last pull got, against tonight's best, the first deaths, the top
 * two things to fix, and the top DPS and HPS. */
export function RaidOverlayCard({ pull }: { pull: RaidPull }) {
  const deaths = pull.deaths.slice(0, 4)
  const top = (by: 'dps' | 'hps') =>
    topRows(pull.meter.rows, by, 3)
      .map((r) => `${r.player} ${amount(r[by])}`)
      .join(', ')
  const best = pull.comparisons?.tonight
  return (
    <>
      <div className="overlay-clock">
        <b>{pullName(pull)}</b> <span className={pull.kill ? 'good' : 'bad'}>{progressText(pull)}</span>{' '}
        <span className="muted small">
          {clock(pull.durationMS)}
          {best && ` · best tonight ${progressText(best)}`}
        </span>
      </div>
      {deaths.map((d, i) => (
        <p key={`${d.player}-${i}`} className="small">
          {i === 0 ? 'First: ' : ''}
          <b>{d.player}</b> {d.ability} at {clock(d.clockMS)}
        </p>
      ))}
      {pull.suggestions.slice(0, 2).map((s) => (
        <p key={s} className="small warning">
          {s}
        </p>
      ))}
      <p className="small overlay-extra">DPS {top('dps')}</p>
      <p className="small overlay-extra">HPS {top('hps')}</p>
    </>
  )
}
