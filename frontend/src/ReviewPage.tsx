import { type ReactNode, useCallback, useMemo, useState } from 'react'
import { FixNext } from './FixNext'
import { SavedBefore } from './SavedBefore'
import { Tiles } from './Tiles'
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
  type FightKind,
} from './api'
import { clock, counted, progressText, resultText } from './format'
import { DamageGraph } from './DamageGraph'
import { EventFeed } from './EventFeed'
import { FightMeters } from './FightMeters'
import { KeyMap } from './KeyMap'
import { KeyReview } from './KeyReview'
import { fightTabs, pullFightData, type FightData } from './KeyTabs'
import { offense, raidOffense } from './meterColumns'
import { RaidLanes, RaidReplay } from './RaidReplay'
import { useRaidTrack, useReplay, useTrack } from './replay'
import { Name, When, Who, type Seeker } from './Seek'
import { ComparePlayer } from './ComparePlayer'
import { PlayerReview } from './PlayerReview'
import { Skeleton } from './Skeleton'
import { Tabs } from './Tabs'
import { useFetched } from './useFetched'

/** The review page's tabs, by their name in the address (?tab=). */
const TABS = ['overview', 'player', 'deaths', 'interrupts', 'events', 'timeline', 'compare'] as const
type TabName = (typeof TABS)[number]

/** The first page of WoWAnalyzer: nothing is uploaded, so there is no report to open there. */
const WOWANALYZER = 'https://wowanalyzer.com'

type MeterProps = Parameters<typeof FightMeters>[0]

/** What the shell shows of a key or a raid pull; the two pages build it and share the tabs. */
interface Fight {
  kind: FightKind
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
export function ReviewPage({ kind, start }: { kind: FightKind; start: number }) {
  const { data: saved, error } = useFetched<KeyView | RaidPull>(`${kind}:${start}`, (signal) =>
    kind === 'mplus' ? fetchSavedKey(start, signal) : fetchRaidPull(start, signal),
  )
  if (error) return <p className="warning">{error}</p>
  if (!saved) return <Skeleton height={480} />
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

/** What a tab shows for a fight saved without its abilities. */
const NO_DATA = <SavedBefore what="abilities" />

/** The select that picks the player a tab is about. */
function PlayerPicker({ players, focus, onFocus }: { players: string[]; focus: string; onFocus: (player: string) => void }) {
  return (
    <label>
      Player{' '}
      <select value={focus} onChange={(e) => onFocus(e.target.value)}>
        <option value="">Pick a player</option>
        {players.map((p) => (
          <option key={p}>{p}</option>
        ))}
      </select>
    </label>
  )
}

/** The page itself: header, then the tabs on one replay clock and one focused player. A time in any tab moves the
 * replay and opens the Timeline. */
function ReviewShell({ fight }: { fight: Fight }) {
  const group = fight.kind === 'mplus' ? 'party' : 'raid'
  const query = useMemo(() => new URLSearchParams(location.search), [])
  const [tab, setTab] = useState<TabName>(TABS.find((t) => t === query.get('tab')) ?? 'overview')
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
  // Built once, not on each tick of the replay's clock: the tabs don't depend on it, so their panels stay as they are.
  const shared = useMemo(
    () => (data ? fightTabs(data, 'Deaths', group, seek, fight.track) : []),
    [data, group, seek, fight.track],
  )
  const feed = useMemo(() => data && <EventFeed data={data} seek={seek} />, [data, seek])
  const meters = (to: number, atEnd: boolean) => (
    <FightMeters
      windows={[{ title: 'Whole fight', from: 0, to, saved: meter }]}
      track={fight.track ?? null}
      atEnd={atEnd}
      fight={fight.meters.fight}
      offense={fight.meters.offense}
      avoidableColumn={group === 'raid'}
      group={group}
      classes={classes}
      focus={focus}
      onFocus={setFocus}
    />
  )
  // The tabs that don't follow the replay's clock, built when what they show changes, so a tick leaves them alone.
  const still = useMemo(() => {
    const part = (id: string) => shared.find((t) => t.id === id)?.body
    return {
      deaths: data ? (
        <>
          {part('deaths')}
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
        NO_DATA
      ),
      interrupts: data ? (
        <>
          {part('interrupts')}
          <h2>Must-stop casts</h2>
          {part('must-stops')}
        </>
      ) : (
        NO_DATA
      ),
      events: data ? (
        <>
          {feed}
          <h2>Defensives and potions</h2>
          {part('defensives')}
        </>
      ) : (
        NO_DATA
      ),
      compare: (
        <>
          <PlayerPicker players={fight.players} focus={focus} onFocus={setFocus} />
          {focus ? (
            <ComparePlayer key={focus} kind={fight.kind} start={fight.start} player={focus} />
          ) : (
            <p className="muted">Pick a player to compare with the top logs of their spec.</p>
          )}
        </>
      ),
    }
  }, [data, shared, feed, seek, fight, focus])
  const bodies: Record<TabName, ReactNode> = {
    ...still,
    overview: (
      <>
        <Tiles tiles={tiles} />
        <FixNext title={fight.suggestionsTitle} items={fight.suggestions} />
        {fight.review}
        {data ? meters(replay.at, replay.at >= fight.durationMS) : NO_DATA}
      </>
    ),
    player: data ? (
      <>
        <PlayerPicker players={fight.players} focus={focus} onFocus={setFocus} />
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
            {meters(fight.durationMS, true)}
          </>
        ) : (
          <p className="muted">Pick a player to see their fight.</p>
        )}
      </>
    ) : (
      NO_DATA
    ),
    timeline: <section className="panel replay">{fight.timeline(replay, focus, setFocus)}</section>,
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
              }}
            >
              Logs
            </button>{' '}
          </span>
        ))}
      </p>
      <Tabs
        open={tab}
        onOpen={(id) => open(id as TabName)}
        tabs={TABS.map((id) => ({ id, name: id[0].toUpperCase() + id.slice(1), body: bodies[id] }))}
      />
    </main>
  )
}
