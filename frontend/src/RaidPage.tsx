import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { fetchRaidNights, fetchRaidPull, reviewURL, subscribe, type KeyTrack, type RaidNight, type RaidPull, type RaidPullSummary } from './api'
import { clock, counted, groupBy, progressText } from './format'
import { defense, raidOffense } from './meterColumns'
import { MeterCard } from './MeterCards'
import { ReportLinks, ReportTag } from './ReportLinks'
import { fightTabs, pullFightData } from './KeyTabs'
import { PlayerTable } from './PlayerTable'
import { BuffGaps } from './BuffGaps'
import { EarlyDeaths, Mechanics } from './RaidCards'
import { FightMeters, NoAbilities } from './FightMeters'
import { Name, type Seeker } from './Seek'
import { SideGroups } from './SideGroups'
import { RaidReplay } from './RaidReplay'
import { topMeter, useRaidTrack, useReplay } from './replay'
import { Tabs } from './Tabs'
import { useNow, useRaidLive } from './useLiveKey'
import { Skeleton } from './Skeleton'
import { tip } from './tipAttrs'
import { WaitingCard } from './WaitingCard'
import { localDay, raidTonight } from './waiting'

/** How many deaths, things to fix, and top damage and healing rows the overlay's card shows. */
const OVERLAY_DEATHS = 3
const OVERLAY_FIXES = 3
const OVERLAY_DPS_ROWS = 5
const OVERLAY_HPS_ROWS = 3

/** A pull's title: "Ula'tek Mythic · pull 23". */
function pullName(p: RaidPullSummary): string {
  return `${p.boss} ${p.difficulty} · pull ${p.number}`
}

/** The Raid tab: tonight's and earlier nights' pulls on the side by night (by week past the last 7 days), newest first
 * by boss (the top bar's character's, '' for all), and the last pull (or the one picked, first initial) as the
 * overview, read again as pulls end. */
export function RaidPage({ initial = null, character }: { initial?: number | null; character: string }) {
  const live = useRaidLive()
  const now = useNow(60_000)
  const [selected, setSelected] = useState<number | null>(initial)
  const [nights, setNights] = useState<RaidNight[]>([])
  const lastStart = live?.last?.start
  useEffect(() => {
    const controller = new AbortController()
    fetchRaidNights(character, controller.signal).then(setNights, () => undefined)
    return () => controller.abort()
  }, [lastStart, character])
  return (
    <div className="layout">
      <aside className="side">
        <button className={selected === null ? 'item active' : 'item'} onClick={() => setSelected(null)}>
          Last pull
        </button>
        <SideGroups
          items={nights}
          time={nightTime}
          count={(g) => counted(g.reduce((sum, n) => sum + n.pulls.length, 0), 'pull')}
          isSelected={(n) => n.pulls.some((p) => p.start === selected)}
        >
          {(group, label) =>
            group.map((n) => (
              <section key={n.night}>
                {label.startsWith('Week of') && <h2>{new Date(nightTime(n)).toLocaleDateString()}</h2>}
                {[...groupBy(n.pulls, (p) => `${p.boss} ${p.difficulty}`)].map(([boss, pulls]) => (
                  <div key={boss} className="boss-pulls">
                    <h3 className="muted small">
                      {boss} <Imported pulls={pulls} />
                    </h3>
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
            ))
          }
        </SideGroups>
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
          <RaidPullView key={live.last.start} pull={live.last} />
        ) : (
          <WaitingCard
            title="Waiting for the first raid pull"
            hint="Each pull shows here when it ends."
            rows={raidTonight(nights, localDay(now), setSelected)}
          />
        )}
      </main>
    </div>
  )
}

/** The Warcraft Logs tag of a boss's pulls on a night, when any is a known report fight: it opens the first one's. */
function Imported({ pulls }: { pulls: RaidPullSummary[] }) {
  const p = pulls.find((p) => p.report)
  return p?.report ? <ReportTag link={p.report} imported={p.imported} /> : null
}

/** Midday of a raid night ("2026-11-10"), so a night is grouped by its date whatever the time zone. */
function nightTime(n: RaidNight): number {
  return new Date(`${n.night}T12:00`).getTime()
}

/** A saved pull, read again when the saved keys and pulls change (a report linked to it, say). */
function SavedPull({ start }: { start: number }) {
  const [pull, setPull] = useState<RaidPull | null>(null) // the previous pull stays up, dimmed, while this one is read
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    const read = () =>
      fetchRaidPull(start, controller.signal)
        .then(setPull)
        .catch((err: unknown) => {
          if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
        })
    const off = subscribe(() => void read())
    return () => {
      controller.abort()
      off()
    }
  }, [start])
  if (error) return <p className="warning">{error}</p>
  if (!pull) return <Skeleton height={480} />
  return (
    <div className={pull.start === start ? undefined : 'stale'}>
      <RaidPullView key={pull.start} pull={pull} />
    </div>
  )
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
 * wrong, the replay with its mechanic lanes and graphs, the meters and tabs, and last the buffs missing through the
 * pull. The replay's clock and the focused raider are the page's: a time in a card moves the replay there and
 * brings it into view, and a raider picked anywhere (the map, a meter, the graph, a name) is focused everywhere. Shown
 * with the pull's start as its key, so both start over with each pull. */
export function RaidPullView({ pull }: { pull: RaidPull }) {
  const replay = useReplay(pull.durationMS, pull.durationMS)
  const { track, error } = useRaidTrack(pull.start)
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
        <ReportLinks fight={pull.start} link={pull.report} />
        <p className="links">
          <a href={reviewURL('raid', pull.start)}>Open full review</a>
        </p>
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
        <EarlyDeaths pull={pull} track={track ?? null} classes={classes} onSeek={show} onFocus={setFocus} />
        <Mechanics pull={pull} classes={classes} onSeek={show} onFocus={setFocus} />
      </div>

      <div ref={replayBox} className="replay-box">
        <RaidReplay pull={pull} track={track} error={error} replay={replay} focus={focus} onFocus={setFocus} />
      </div>

      <FightMeters
        windows={[
          {
            title: 'Whole pull',
            from: 0,
            to: replay.at,
            saved: pull.meter,
          },
        ]}
        track={track ?? null}
        atEnd={replay.at >= pull.durationMS}
        fight={pull}
        offense={raidOffense}
        avoidableColumn
        group="raid"
        classes={classes}
        focus={focus}
        onFocus={setFocus}
      />
      <PullTabs pull={pull} seek={{ onSeek: show, classes, onFocus: setFocus }} track={track ?? null} />
    </>
  )
}

/** The overlay's card between pulls: how far the last pull got, the boss's health as a bar with tonight's best pull
 * marked, the first deaths, the top things to fix, and the top DPS and HPS (rows players each when set, else 5 and 3);
 * made very short, the title and bar only. */
export function RaidOverlayCard({ pull, rows = 0 }: { pull: RaidPull; rows?: number }) {
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
            <div className="mark" style={{ left: `${100 - best.bossPct}%` }} {...tip("tonight's best pull")} />
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
          <MeterCard
            title="Damage"
            meter={topMeter(pull.meter, 'dps', rows || OVERLAY_DPS_ROWS)}
            columns={raidOffense.slice(0, 1)}
          />
          <MeterCard
            title="Healing"
            meter={topMeter(pull.meter, 'hps', rows || OVERLAY_HPS_ROWS)}
            columns={defense(false).slice(0, 1)}
          />
        </div>
      </div>
    </>
  )
}

/** Under the meters: every raider's pull in one table, and the tabs a key has (deaths, interrupts, must-stops, spikes,
 * big hits, avoidable, defensives and potions, CC) with the buffs missing through the pull; a pull saved before they were kept
 * says how to fill them in. */
function PullTabs({ pull, seek, track }: { pull: RaidPull; seek: Seeker; track: KeyTrack | null }) {
  const data = pullFightData(pull)
  if (!data) return <NoAbilities />
  const tabs = fightTabs(data, `Deaths ${pull.deaths.length}`, 'raid', seek, track)
  const missing = new Set(pull.buffGaps?.map((g) => g.buff)).size
  if (missing > 0) {
    tabs.push({
      name: `Buffs ${missing}`,
      body: <BuffGaps gaps={pull.buffGaps ?? []} endMS={pull.durationMS} players={pull.players.length} seek={seek} />,
    })
  }
  return (
    <>
      <PlayerTable
        title="Raiders"
        meter={pull.meter}
        casts={data.casts}
        activity={pull.activity}
        deaths={pull.deaths}
        onPick={seek.onFocus}
      />
      <div className="tall">
        <Tabs tabs={tabs} />
      </div>
    </>
  )
}
