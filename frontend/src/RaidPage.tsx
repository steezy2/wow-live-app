import { useEffect, useState } from 'react'
import { fetchRaidNights, fetchRaidPull, type RaidNight, type RaidPull, type RaidPullSummary } from './api'
import { amount, clock, coverText, progressText } from './format'
import { defense, raidOffense } from './meterColumns'
import { MeterCard } from './MeterCards'
import { useRaidLive } from './useLiveKey'

/** A pull's title: "Ula'tek Mythic · pull 23". */
function pullName(p: RaidPullSummary): string {
  return `${p.boss} ${p.difficulty} · pull ${p.number}`
}

/** The Raid tab: tonight's and earlier nights' pulls on the side, and the last pull (or the one picked) as the
 * overview, read again as pulls end. */
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
            {n.pulls.map((p) => (
              <div key={p.start} className={selected === p.start ? 'item active' : 'item'}>
                <button className="open" onClick={() => setSelected(p.start)}>
                  <b>{pullName(p)}</b>
                  <span className="muted small">
                    {clock(p.durationMS)} · {progressText(p)}
                  </span>
                </button>
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

/** One pull's overview: how far it got against the best pulls, the deaths, what went wrong, the cooldowns and the
 * meters. */
export function RaidPullView({ pull }: { pull: RaidPull }) {
  const early = pull.deaths.filter((d) => d.early)
  return (
    <>
      <section className="raid-head">
        <h1>
          {pullName(pull)} <span className={pull.kill ? 'good' : 'bad'}>{progressText(pull)}</span>
        </h1>
        <p>
          {clock(pull.durationMS)} · {pull.deaths.length} deaths · {pull.players.length} raiders
        </p>
        <Comparisons pull={pull} />
      </section>

      <section className="panel">
        <h2>
          Early deaths {early.length} <span className="muted small">of {pull.deaths.length}</span>
        </h2>
        <ul className="list">
          {early.map((d, i) => (
            <li key={`${d.player}-${d.clockMS}-${i}`} className="stack">
              <span>
                <b>{d.player}</b> {d.ability || 'unknown'} <span className="muted">from {d.killer || 'unknown'}</span>
              </span>
              <span className="muted small">
                {clock(d.clockMS)} · {coverText(d.defensive, d.ready ?? [])}
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

      <section className="panel">
        <h2>Cooldowns</h2>
        <p>
          {pull.lusts.length > 0 ? `Bloodlust at ${pull.lusts.map(clock).join(', ')}` : 'No Bloodlust'}
        </p>
        <p className="muted small">
          {pull.cooldowns.map((c) => `${clock(c.clockMS)} ${c.player} ${c.ability}`).join(' · ') ||
            'No raid cooldowns or combat potions'}
        </p>
      </section>

      <section className="cards">
        <MeterCard title="Pull" meter={pull.meter} columns={raidOffense} />
        <MeterCard title="Pull" meter={pull.meter} columns={defense(true)} />
      </section>
    </>
  )
}

/** What went wrong by kind of mechanic; a kind with nothing to show is left out. */
function Mechanics({ pull }: { pull: RaidPull }) {
  const lines: [string, string][] = [
    ...pull.avoidable.map((a): [string, string] => [
      `${a.ability} hits`,
      a.players.map((p) => `${p.player} ${p.count}`).join(', '),
    ]),
    ...pull.soaks.map((s): [string, string] => [
      `${s.ability} soaks`,
      `${s.casts} soaks, ${s.fewest}–${s.most} raiders each${s.died.length ? ` · died: ${s.died.join(', ')}` : ''}`,
    ]),
    ...pull.stacks.map((s): [string, string] => [
      `${s.ability} stacks${s.limit ? ` (${s.limit} too many)` : ''}`,
      s.players
        .filter((p) => !s.limit || p.count >= s.limit)
        .slice(0, 5)
        .map((p) => `${p.player} ${p.count}`)
        .join(', ') || 'nobody at the limit',
    ]),
    ...pull.dispels.map((d): [string, string] => [
      `${d.spell} on ${d.player}`,
      `${d.by ? `dispelled by ${d.by}` : 'ran out'} after ${(d.afterMS / 1000).toFixed(1)} s`,
    ]),
    ...pull.wentOff.map((s): [string, string] => [`${s.spell} went off`, `${s.mob} at ${clock(s.clockMS)}`]),
    ...pull.adds
      .filter((a) => a.aliveEnd > 0)
      .map((a): [string, string] => [
        `${a.name} adds`,
        `${a.aliveEnd} of ${a.spawned} still up at the end, one for ${clock(a.longestMS)}`,
      ]),
    ...pull.priority.map((p): [string, string] => [`Damage into ${p.target}`, amount(p.damage)]),
  ]
  return (
    <section className="panel">
      <h2>Mechanics</h2>
      <ul className="list">
        {lines.map(([what, detail]) => (
          <li key={what} className="stack">
            <b>{what}</b>
            <span className="muted small">{detail}</span>
          </li>
        ))}
        {lines.length === 0 && <li className="muted">Nothing flagged on this boss</li>}
      </ul>
    </section>
  )
}

/** The overlay's card between pulls: how far the last pull got, against tonight's best, the first deaths, the
 * worst mechanic and the top DPS and HPS. */
export function RaidOverlayCard({ pull }: { pull: RaidPull }) {
  const deaths = pull.deaths.slice(0, 4)
  const worst = pull.avoidable[0]
  const top = (by: 'dps' | 'hps') =>
    [...pull.meter.rows]
      .sort((a, b) => b[by] - a[by])
      .slice(0, 3)
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
      {worst && (
        <p className="small warning">
          {worst.ability}: {worst.players.slice(0, 4).map((p) => `${p.player} ${p.count}`).join(', ')}
        </p>
      )}
      <p className="small overlay-extra">DPS {top('dps')}</p>
      <p className="small overlay-extra">HPS {top('hps')}</p>
    </>
  )
}
