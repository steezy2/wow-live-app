import { type ReactNode, useCallback, useEffect, useMemo, useState } from 'react'
import {
  fetchRaidPull,
  fetchSavedKey,
  popOutReview,
  reviewURL,
  type BuffGap,
  type KeyMeter,
  type Resurrection,
  type KeyTrack,
  type KeyView,
  type RaidPull,
} from './api'
import { clock, counted, progressText, resultText } from './format'
import { DamageGraph } from './DamageGraph'
import { EventFeed } from './EventFeed'
import { FightMeters, NoAbilities } from './FightMeters'
import { KeyMap } from './KeyMap'
import { KeyReview } from './KeyReview'
import { fightTabs, pullFightData, type FightData } from './KeyTabs'
import { offense, raidOffense } from './meterColumns'
import { RaidLanes, RaidReplay } from './RaidReplay'
import { useRaidTrack, useReplay, useTrack } from './replay'
import { Name, When, Who, type Seeker } from './Seek'
import { ComparePlayer } from './ComparePlayer'
import { PlayerReview } from './PlayerReview'
import { Tabs } from './Tabs'

/** The review page's tabs, by their name in the address (?tab=). */
const TABS = ['overview', 'player', 'deaths', 'interrupts', 'events', 'timeline', 'compare'] as const
type TabName = (typeof TABS)[number]

/** The first page of WoWAnalyzer: nothing is uploaded, so there is no report to open there. */
const WOWANALYZER = 'https://wowanalyzer.com'

type MeterProps = Parameters<typeof FightMeters>[0]

/** What the shell shows of a key or a raid pull; the two pages build it and share the tabs. */
interface Fight {
  kind: 'mplus' | 'raid'
  start: number
  title: string
  result: string
  durationMS: number
  players: string[]
  meter: KeyMeter
  data: FightData | null
  track: KeyTrack | null | undefined
  suggestions: string[]
  suggestionsTitle: string
  buffGaps: BuffGap[]
  lusts: number[]
  resurrections: Resurrection[]
  /** What its meters read: the fight's abilities and its columns. */
  meters: { fight: MeterProps['fight']; offense: MeterProps['offense'] }
  /** Under the overview's tiles (an M+ key's review). */
  review?: ReactNode
  /** The Timeline tab's body, on the page's clock. */
  timeline: (replay: ReturnType<typeof useReplay>, focus: string, onFocus: (p: string) => void) => ReactNode
}

const noop = () => undefined

/** A finished key's (kind mplus) or raid pull's (raid) full review, read from its start: the long version of what the
 * key and pull pages show, in tabs. The tab and player are the address's query. */
export function ReviewPage({ kind, start }: { kind: 'mplus' | 'raid'; start: number }) {
  const [saved, setSaved] = useState<KeyView | RaidPull | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    const read = kind === 'mplus' ? fetchSavedKey : fetchRaidPull
    read(start, controller.signal).then(setSaved, (err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
    })
    return () => controller.abort()
  }, [kind, start])
  if (error) return <p className="warning">{error}</p>
  if (saved?.start !== start) return <p className="muted">Loading…</p>
  return kind === 'mplus' ? <KeyFight view={saved as KeyView} /> : <PullFight pull={saved as RaidPull} />
}

function KeyFight({ view: key }: { view: KeyView }) {
  const track = useTrack(key.start, false)
  const timer = key.timerMS ?? 0
  return (
    <ReviewShell
      fight={{
        kind: 'mplus',
        start: key.start,
        title: `${key.dungeon} +${key.level}`,
        result: resultText(key.status, key.chests, key.clockMS, timer),
        durationMS: key.clockMS,
        players: key.players,
        meter: key.overall,
        data: { ...key, lanes: key.lanes ?? [] },
        track,
        suggestions: key.suggestions ?? [],
        suggestionsTitle: 'Fix next key',
        buffGaps: key.buffGaps ?? [],
        lusts: key.lusts ?? [],
        resurrections: key.resurrections ?? [],
        meters: { fight: key, offense },
        review: <KeyReview keyView={key} justEnded={false} onReview={noop} />,
        timeline: (replay, focus, onFocus) => (
          <>
            {track && (
              <DamageGraph
                keyView={key}
                track={track}
                now={replay.at}
                onSeek={replay.setAt}
                focus={focus}
                onFocus={onFocus}
              />
            )}
            {key.lanes && key.lanes.length > 0 && (
              <RaidLanes lanes={key.lanes} end={key.clockMS} now={replay.at} onSeek={replay.setAt} />
            )}
            <KeyMap keyView={key} clockMS={key.clockMS} track={track} replay={replay} focus={focus} onFocus={onFocus} />
          </>
        ),
      }}
    />
  )
}

function PullFight({ pull }: { pull: RaidPull }) {
  const { track, error } = useRaidTrack(pull.start)
  return (
    <ReviewShell
      fight={{
        kind: 'raid',
        start: pull.start,
        title: `${pull.boss} ${pull.difficulty} · pull ${pull.number}`,
        result: `${progressText(pull)} · ${counted(pull.deaths.length, 'death')}`,
        durationMS: pull.durationMS,
        players: pull.players,
        meter: pull.meter,
        data: pullFightData(pull),
        track,
        suggestions: pull.suggestions,
        suggestionsTitle: 'Fix next pull',
        buffGaps: pull.buffGaps ?? [],
        lusts: pull.lusts,
        resurrections: pull.resurrections ?? [],
        meters: { fight: pull, offense: raidOffense },
        timeline: (replay, focus, onFocus) => (
          <RaidReplay pull={pull} track={track} error={error} replay={replay} focus={focus} onFocus={onFocus} />
        ),
      }}
    />
  )
}

/** The page itself: header, then the tabs on one replay clock and one focused player. A time in any tab moves the
 * replay and opens the Timeline. */
function ReviewShell({ fight }: { fight: Fight }) {
  const group = fight.kind === 'mplus' ? 'party' : 'raid'
  const query = useMemo(() => new URLSearchParams(location.search), [])
  const [tab, setTab] = useState<TabName>(TABS.find((t) => t === query.get('tab')) ?? 'overview')
  const [opened, setOpened] = useState(0) // bumped to open a tab the page picked, since Tabs keeps its own state
  const [focus, setFocus] = useState(query.get('player') ?? '')
  const replay = useReplay(fight.durationMS, fight.durationMS)
  const classes = useMemo(() => Object.fromEntries(fight.meter.rows.map((r) => [r.player, r.class])), [fight.meter])
  const setAt = replay.setAt
  const open = useCallback((next: TabName) => {
    setTab(next)
    const url = new URL(location.href)
    url.searchParams.set('tab', next)
    history.replaceState(null, '', url)
  }, [])
  const seek = useMemo<Seeker>(
    () => ({
      onSeek: (ms) => {
        setAt(ms)
        open('timeline')
        setOpened((n) => n + 1)
      },
      classes,
      onFocus: setFocus,
    }),
    [setAt, open, classes],
  )
  const { data, meter } = fight
  const stopped = data?.stops.filter((s) => s.outcome !== 'went off').length ?? 0
  const tiles: [string, string][] = [
    [String(Math.round(meter.rows.reduce((sum, r) => sum + r.dps, 0))), 'party damage per second'],
    [String(Math.round(meter.rows.reduce((sum, r) => sum + r.hps, 0))), 'party healing per second'],
    [String(data?.deaths.length ?? 0), 'deaths'],
  ]
  if (data?.reference) tiles.push([`${stopped}/${data.stops.length}`, 'must-stop casts stopped'])
  const tabsOf = (prefix: string) =>
    data ? fightTabs(data, 'Deaths', group, seek, fight.track).filter((t) => t.name.startsWith(prefix)) : []
  const bodies: Record<TabName, ReactNode> = {
    overview: (
      <>
        <div className="recap-tiles">
          {tiles.map(([value, label]) => (
            <div key={label} className="card small">
              <b>{value}</b>
              <span className="muted small">{label}</span>
            </div>
          ))}
        </div>
        {fight.suggestions.length > 0 && (
          <section className="panel attention">
            <h2>{fight.suggestionsTitle}</h2>
            <ol className="fix-next">
              {fight.suggestions.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
          </section>
        )}
        {fight.review}
        {data ? (
          <FightMeters
            windows={[{ title: 'Whole fight', from: 0, to: replay.at, saved: meter }]}
            track={fight.track ?? null}
            atEnd={replay.at >= fight.durationMS}
            fight={fight.meters.fight}
            offense={fight.meters.offense}
            avoidableColumn={group === 'raid'}
            group={group}
            classes={classes}
            focus={focus}
            onFocus={setFocus}
          />
        ) : (
          <NoAbilities />
        )}
      </>
    ),
    player: data ? (
      <>
        <label>
          Player{' '}
          <select value={focus} onChange={(e) => setFocus(e.target.value)}>
            <option value="">Pick a player</option>
            {fight.players.map((p) => (
              <option key={p}>{p}</option>
            ))}
          </select>
        </label>
        {focus ? (
          <>
            <PlayerReview
              player={focus}
              row={meter.rows.find((r) => r.player === focus)}
              data={data}
              track={fight.track}
              buffGaps={fight.buffGaps}
              lusts={fight.lusts}
              durationMS={fight.durationMS}
              seek={seek}
              now={replay.at}
            />
            <FightMeters
              windows={[{ title: 'Whole fight', from: 0, to: fight.durationMS, saved: meter }]}
              track={fight.track ?? null}
              atEnd
              fight={fight.meters.fight}
              offense={fight.meters.offense}
              avoidableColumn={group === 'raid'}
              group={group}
              classes={classes}
              focus={focus}
              onFocus={setFocus}
            />
          </>
        ) : (
          <p className="muted">Pick a player to see their fight.</p>
        )}
      </>
    ) : (
      <NoAbilities />
    ),
    deaths: data ? (
      <>
        {tabsOf('Deaths').map((t) => (
          <div key={t.name}>{t.body}</div>
        ))}
        <h2>Resurrections</h2>
        <ul className="list">
          {fight.resurrections.map((r, i) => (
            <li key={i}>
              <span>
                <Who player={r.by} seek={seek} /> brought back <Who player={r.player} seek={seek} />
              </span>
              <When ms={r.clockMS} seek={seek} />
            </li>
          ))}
          {fight.resurrections.length === 0 && <li className="muted">None, or not recorded before Nov 13</li>}
        </ul>
      </>
    ) : (
      <NoAbilities />
    ),
    interrupts: data ? (
      <>
        {tabsOf('Interrupts').map((t) => (
          <div key={t.name}>{t.body}</div>
        ))}
        {tabsOf('Must-stops').map((t) => (
          <div key={t.name}>
            <h2>Must-stop casts</h2>
            {t.body}
          </div>
        ))}
      </>
    ) : (
      <NoAbilities />
    ),
    events: data ? (
      <>
        <EventFeed data={data} seek={seek} />
        {tabsOf('Defensives and potions').map((t) => (
          <div key={t.name}>
            <h2>Defensives and potions</h2>
            {t.body}
          </div>
        ))}
      </>
    ) : (
      <NoAbilities />
    ),
    timeline: <section className="panel replay">{fight.timeline(replay, focus, setFocus)}</section>,
    compare: (
      <>
        <label>
          Player{' '}
          <select value={focus} onChange={(e) => setFocus(e.target.value)}>
            <option value="">Pick a player</option>
            {fight.players.map((p) => (
              <option key={p}>{p}</option>
            ))}
          </select>
        </label>
        {focus ? (
          <ComparePlayer key={focus} kind={fight.kind} start={fight.start} player={focus} />
        ) : (
          <p className="muted">Pick a player to compare with the top logs of their spec.</p>
        )}
      </>
    ),
  }
  return (
    <main className="page review">
      <header className="title">
        <button className="back" onClick={() => (history.length > 1 ? history.back() : location.assign('/'))}>
          ← Back
        </button>
        <h1>{fight.title}</h1>
        <button onClick={() => popOutReview(reviewURL(fight.kind, fight.start, tab, focus))}>Pop out</button>
      </header>
      <p>
        {fight.result} · {clock(fight.durationMS)} ·{' '}
        <a href={WOWANALYZER} target="_blank" rel="noreferrer">
          WoWAnalyzer
        </a>
      </p>
      <p className="small">
        {fight.players.map((p) => (
          <span key={p}>
            <Name player={p} classes={classes} onPick={setFocus} />{' '}
            <button
              className="link"
              onClick={() => {
                setFocus(p)
                open('compare')
                setOpened((n) => n + 1)
              }}
            >
              Logs
            </button>{' '}
          </span>
        ))}
      </p>
      <Tabs
        key={opened}
        initial={TABS.indexOf(tab)}
        onOpen={(i) => open(TABS[i])}
        tabs={TABS.map((name) => ({ name: name[0].toUpperCase() + name.slice(1), body: bodies[name] }))}
      />
    </main>
  )
}
