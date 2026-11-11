import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchRaidNights, fetchRaidPull, type RaidNight, type RaidPull, type RaidPullSummary } from './api'
import { clock, counted, progressText } from './format'
import { defense, raidOffense } from './meterColumns'
import { MeterCard } from './MeterCards'
import { fightTabs, type FightData } from './KeyTabs'
import { PlayerTable } from './PlayerTable'
import { EarlyDeaths, Mechanics, Name, NoAbilities, PullMeters } from './RaidCards'
import { RaidReplay } from './RaidReplay'
import { topMeter, useReplay } from './replay'
import { Tabs } from './Tabs'
import { ReadyPanel } from './ReadyPanel'
import { useRaidLive } from './useLiveKey'

/** How many deaths, things to fix, and top damage and healing rows the overlay's card shows. */
const OVERLAY_DEATHS = 3
const OVERLAY_FIXES = 3
const OVERLAY_DPS_ROWS = 5
const OVERLAY_HPS_ROWS = 3

/** A pull's title: "Ula'tek Mythic · pull 23". */
function pullName(p: RaidPullSummary): string {
  return `${p.boss} ${p.difficulty} · pull ${p.number}`
}

/** The Raid tab: tonight's and earlier nights' pulls on the side, newest first by boss, and the last pull (or the one
 * picked) as the overview, read again as pulls end. */
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
                    <span className="muted small">
                      {new Date(p.start).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })}
                    </span>
                  </button>
                ))}
              </div>
            ))}
          </section>
        ))}
        {nights.length === 0 && <p className="muted small">No raid pulls yet: each one is saved here as it ends.</p>}
      </aside>
      <main className="page">
        {live?.pulling ? (
          <p className="panel">
            Pulling <b>{live.boss}</b> now · the overview of this pull shows when it ends.
          </p>
        ) : (
          live?.ready && <ReadyPanel ready={live.ready} raid />
        )}
        {selected !== null ? (
          <SavedPull start={selected} />
        ) : live?.last ? (
          <RaidPullView key={live.last.start} pull={live.last} />
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

/** A night's pulls by boss and difficulty, in the order the list gives them (the boss pulled last first). */
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
  return pull?.start === start ? <RaidPullView key={start} pull={pull} /> : <p className="muted">Loading…</p>
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

/** Each raider's class by name, from a pull's meter. */
function classesOf(pull: RaidPull): Record<string, string> {
  return Object.fromEntries(pull.meter.rows.map((r) => [r.player, r.class]))
}

/** One pull's overview: how far it got against the best pulls, what to fix next, the early deaths and what went
 * wrong, the replay with its mechanic lanes and graphs, and the meters. The replay's clock and the focused raider
 * are the page's: a time in a card moves the replay there and brings it into view, and a raider picked anywhere
 * (the map, a meter, the graph, a name) is focused everywhere. Shown with the pull's start as its key, so both start
 * over with each pull. */
export function RaidPullView({ pull }: { pull: RaidPull }) {
  const replay = useReplay(pull.durationMS, 0, 4)
  const [focus, setFocus] = useState('')
  const replayBox = useRef<HTMLDivElement>(null)
  const classes = useMemo(() => classesOf(pull), [pull])
  const setAt = replay.setAt
  const show = useCallback(
    (clockMS: number) => {
      setAt(clockMS)
      replayBox.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    },
    [setAt],
  )
  return (
    <>
      <section className="raid-head">
        <h1>
          {pullName(pull)} <span className={pull.kill ? 'good' : 'bad'}>{progressText(pull)}</span>
        </h1>
        <p>
          {clock(pull.durationMS)} · {counted(pull.deaths.length, 'death')} · {pull.players.length} raiders
          {pull.lusts.length > 0 ? ` · Bloodlust at ${pull.lusts.map(clock).join(', ')}` : ' · no Bloodlust'}
        </p>
        <Comparisons pull={pull} />
        {pull.missing && pull.missing.length > 0 && (
          <p className="warning small">
            Missing at the pull:{' '}
            {pull.missing.map((m, i) => (
              <Fragment key={m.player}>
                {i > 0 && ' · '}
                <Name player={m.player} classes={classes} /> {m.missing.join(', ')}
              </Fragment>
            ))}
          </p>
        )}
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

      <div className="raid-columns">
        <EarlyDeaths pull={pull} classes={classes} onSeek={show} onFocus={setFocus} />
        <Mechanics pull={pull} classes={classes} onSeek={show} onFocus={setFocus} />
      </div>

      <div ref={replayBox} className="replay-box">
        <RaidReplay pull={pull} replay={replay} focus={focus} onFocus={setFocus} />
      </div>

      <PullMeters pull={pull} classes={classes} focus={focus} onFocus={setFocus} />
      <PullTabs pull={pull} onFocus={setFocus} />
    </>
  )
}

/** The overlay's card between pulls: how far the last pull got, the boss's health as a bar with tonight's best pull
 * marked, the first deaths, the top things to fix, and the top DPS and HPS; made very short, the title and bar only. */
export function RaidOverlayCard({ pull }: { pull: RaidPull }) {
  const classes = classesOf(pull)
  const best = pull.comparisons?.tonight
  const more = pull.deaths.length - OVERLAY_DEATHS
  return (
    <>
      <div className="overlay-clock">
        <span className={pull.kill ? 'big good' : 'big bad'}>{progressText(pull)}</span> <b>{pullName(pull)}</b>{' '}
        <span className="muted small">
          {clock(pull.durationMS)} · {counted(pull.deaths.length, 'death')}
        </span>
      </div>
      {pull.bossPct >= 0 && (
        <div className="bar" role="img" aria-label={`the boss at ${Math.round(pull.bossPct)}%`}>
          <div className={pull.kill ? 'fill ahead' : 'fill'} style={{ width: `${100 - pull.bossPct}%` }} />
          {best && best.bossPct >= 0 && (
            <div className="mark" style={{ left: `${100 - best.bossPct}%` }} title="tonight's best pull" />
          )}
        </div>
      )}
      <p className="small muted">
        {best ? `Best tonight: pull ${best.number}, ${progressText(best)}` : 'The best pull tonight'}
      </p>
      <div className="overlay-extra overlay-rows">
        {pull.deaths.slice(0, OVERLAY_DEATHS).map((d, i) => (
          <p key={`${d.player}-${i}`} className="small">
            <span className="muted">{clock(d.clockMS)}</span> <Name player={d.player} classes={classes} />{' '}
            {d.ability || 'unknown'}
          </p>
        ))}
        {more > 0 && <p className="small muted">and {counted(more, 'more death')}</p>}
        {pull.suggestions.length > 0 && (
          <ol className="overlay-fix small warning">
            {pull.suggestions.slice(0, OVERLAY_FIXES).map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        )}
        <div className="overlay-meters">
          <MeterCard title="Damage" meter={topMeter(pull.meter, 'dps', OVERLAY_DPS_ROWS)} columns={raidOffense.slice(0, 1)} />
          <MeterCard
            title="Healing"
            meter={topMeter(pull.meter, 'hps', OVERLAY_HPS_ROWS)}
            columns={defense(false).slice(0, 1)}
          />
        </div>
      </div>
    </>
  )
}

/** Under the meters: every raider's pull in one table, and the tabs a key has (deaths, interrupts, must-stops, spikes,
 * defensives and potions, CC); a pull saved before they were kept says how to fill them in. */
function PullTabs({ pull, onFocus }: { pull: RaidPull; onFocus: (player: string) => void }) {
  if (!pull.casts) return <NoAbilities />
  const data: FightData = {
    deaths: pull.deaths,
    interrupts: pull.interrupts ?? [],
    casts: pull.casts,
    stops: pull.stops ?? [],
    wastedKicks: pull.wastedKicks ?? [],
    spikes: pull.spikes ?? [],
    debuffs: pull.debuffs ?? [],
    reference: true,
  }
  return (
    <>
      <PlayerTable
        title="Raiders"
        meter={pull.meter}
        casts={pull.casts}
        activity={pull.activity}
        deaths={pull.deaths}
        avoidableListed
        onPick={onFocus}
      />
      <Tabs tabs={fightTabs(data, `Deaths ${pull.deaths.length}`, 'raid')} />
    </>
  )
}
