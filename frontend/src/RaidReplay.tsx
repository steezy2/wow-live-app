import { useEffect, useMemo, useRef, useState, type CSSProperties, type MouseEvent } from 'react'
import { fetchRaidTrack, type KeyTrack, type RaidLane, type RaidPull } from './api'
import { amount, clock } from './format'
import { BOSS_GONE_MS, MapCanvas, ReplayControls, type Backdrop } from './KeyMap'
import {
  abilitiesUpTo,
  floorsOf,
  lastAt,
  meterBetween,
  tickStep,
  topMeter,
  type AbilitiesUpTo,
  type SliderMark,
  type useReplay,
} from './replay'
import { MeterGraph } from './DamageGraph'
import { MeterCard } from './MeterCards'
import { defense, raidOffense } from './meterColumns'
import { RaidFocus } from './RaidFocus'
import { classColors } from './classColors'

/** How many raiders the replay's meters list, top first. */
const REPLAY_METER_ROWS = 10

/** At most this many units in the boss box: the boss's own, then the adds to burst. */
const BOSS_BOX_UNITS = 4

/** How many of the focused raider's last casts the cast box lists. */
const CAST_BOX_CASTS = 5

/** How many abilities the damage taken box lists, the most damage first. */
const TAKEN_BOX_ROWS = 5

/** A raid pull's replay: the mechanics' lanes and the DPS and HPS graph, the play bar, then the raid and the boss on
 * the room's map with the meters at that moment, all on the page's one clock: a click on the lanes or the graph moves
 * the map there. A click on a raider (their dot, a meter row, their line) focuses them: a ring and their name on the map,
 * their focus card in place of the meters, and their line alone on the graph; a right click goes back. */
export function RaidReplay(props: {
  pull: RaidPull
  replay: ReturnType<typeof useReplay>
  focus: string
  onFocus: (player: string) => void
}) {
  const { pull, replay, focus, onFocus } = props
  const [track, setTrack] = useState<KeyTrack | null | undefined>(undefined) // null: no replay kept
  const [error, setError] = useState<string | null>(null)
  const end = pull.durationMS
  const [trail, setTrail] = useState(10_000)
  const [floor, setFloor] = useState<number | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchRaidTrack(pull.start, controller.signal).then(setTrack, (err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
    })
    return () => controller.abort()
  }, [pull.start])
  const floors = useMemo(() => floorsOf(track ?? null), [track])
  const current = floor ?? floors[0]
  const room = pull.maps.find((m) => m.uiMapID === current)
  const backdrop = useMemo<Backdrop>(() => (room ? { kind: 'image', map: room } : { kind: 'walked', walked: [] }), [room])
  const marks: SliderMark[] = pull.deaths.map((d) => ({
    clockMS: d.clockMS,
    className: 'death',
    title: `${d.player} died at ${clock(d.clockMS)}`,
  }))
  if (error) return <p className="panel warning">Couldn't read the replay: {error}</p>
  if (track === null) return <p className="panel muted">No replay for this pull: it was saved before replays were kept.</p>
  if (!track) return <p className="panel muted">Loading the replay…</p>
  const now = replay.at
  const sofar = meterBetween(track, 0, now)
  const upTo = focus ? abilitiesUpTo(track, pull, focus, now) : undefined
  return (
    <section className="panel replay">
      <RaidLanes lanes={pull.lanes} end={end} now={now} onSeek={replay.setAt} />
      <MeterGraph
        track={track}
        end={end}
        marks={marks}
        group="raid"
        over="pull"
        now={now}
        onSeek={replay.setAt}
        focus={focus}
        onFocus={onFocus}
      />
      <div className="map-controls">
        <ReplayControls replay={replay} end={end} marks={marks} trail={trail} setTrail={setTrail} />
        {floors.length > 1 &&
          floors.map((m, i) => (
            <button key={m} aria-pressed={m === current} onClick={() => setFloor(m)}>
              Room {i + 1}
            </button>
          ))}
      </div>
      {floors.length === 0 ? (
        <p className="muted">No positions in this pull (needs Advanced Combat Logging).</p>
      ) : (
        <div className="map-body">
          <div>
            <div className="map-stage">
              <MapCanvas
              key={current}
              track={track}
              backdrop={backdrop}
              floor={current}
              now={now}
              clock={replay.playing ? replay.now : undefined}
              trailMS={trail}
              deaths={pull.deaths}
              focus={focus}
              onPick={onFocus}
              />
              <BossBox track={track} now={now} />
              {focus && <CastBox track={track} player={focus} now={now} />}
              {upTo && <TakenBox taken={upTo.taken} />}
            </div>
            <p className="muted small">
              Scroll to zoom, drag to move, double click for the whole room · click a raider to focus them, right click
              for the whole raid
              {room && (
                <>
                  {' '}
                  · map:{' '}
                  <a href="https://www.warcraftlogs.com" target="_blank" rel="noreferrer">
                    Warcraft Logs
                  </a>
                </>
              )}
            </p>
          </div>
          {focus ? (
            <RaidFocus
              pull={pull}
              track={track}
              player={focus}
              upTo={upTo}
              now={now}
              onSeek={replay.setAt}
              onBack={() => onFocus('')}
            />
          ) : (
            <div className="replay-meters raid">
              <MeterCard
                title="DPS so far"
                meter={topMeter(sofar, 'dps', REPLAY_METER_ROWS)}
                columns={raidOffense.slice(0, 2)}
                onPick={onFocus}
              />
              <MeterCard
                title="HPS so far"
                meter={topMeter(sofar, 'hps', REPLAY_METER_ROWS)}
                columns={defense(false).slice(0, 2)}
                onPick={onFocus}
              />
            </div>
          )}
        </div>
      )}
    </section>
  )
}

/** The pull's timeline: a lane per mechanic with its marks (a stretch for a phase, an add's life or how long a buff
 * lasted), the ones that went wrong in red, over a ruler with a tick every tickStep. One line through every lane is
 * the replay's moment, with its time on the ruler. A click or a drag anywhere on the lanes moves the replay there; a
 * click on a mark, to its start. */
export function RaidLanes(props: { lanes: RaidLane[]; end: number; now: number; onSeek: (clockMS: number) => void }) {
  const { lanes, end, now, onSeek } = props
  const ruler = useRef<HTMLDivElement>(null)
  const share = (t: number) => Math.min(t, end) / Math.max(1, end)
  const at = (t: number) => `${share(t) * 100}%`
  if (lanes.length === 0) return null
  const ticks: number[] = []
  for (let t = 0; t <= end; t += tickStep(end)) ticks.push(t)
  const seek = (e: MouseEvent) => {
    const box = ruler.current?.getBoundingClientRect() // every lane's track is the ruler's width
    if (!box || e.clientX < box.left) return
    onSeek(Math.round(Math.min(1, (e.clientX - box.left) / box.width) * end))
  }
  return (
    <div
      className="lanes"
      role="list"
      aria-label="the pull's mechanics over time"
      onMouseDown={(e) => e.button === 0 && seek(e)}
      onMouseMove={(e) => e.buttons === 1 && seek(e)}
    >
      {lanes.map((l) => (
        <div key={`${l.kind}-${l.name}`} className="lane" role="listitem">
          <span className="lane-name" title={l.name}>
            {l.name} <span className="muted">{l.kind === 'phases' ? '' : l.marks.length}</span>
          </span>
          <div className={`lane-track lane-${l.kind}`}>
            {l.marks.map((m, i) => {
              const title = `${clock(m.clockMS)}${m.endMS ? `–${clock(m.endMS)}` : ''} ${m.text}`
              return (
                <button
                  key={i}
                  className={`${m.endMS ? 'lane-span' : 'lane-mark'}${m.bad ? ' bad-mark' : ''}`}
                  style={m.endMS ? { left: at(m.clockMS), width: at(m.endMS - m.clockMS) } : { left: at(m.clockMS) }}
                  title={title}
                  aria-label={title}
                  onMouseDown={(e) => e.stopPropagation()}
                  onClick={() => onSeek(m.clockMS)}
                >
                  {l.kind === 'phases' ? m.text : ''}
                </button>
              )
            })}
          </div>
        </div>
      ))}
      <div className="lane" aria-hidden="true">
        <span className="lane-name muted">Time</span>
        <div className="lane-track lane-ruler" ref={ruler}>
          {ticks.map((t) => (
            <span key={t} className="lane-tick" style={{ left: at(t) }}>
              {clock(t)}
            </span>
          ))}
        </div>
      </div>
      <div className="lanes-now" style={{ '--at': share(now) } as CSSProperties}>
        <span>{clock(now)}</span>
      </div>
    </div>
  )
}

/** The boss's health at the replay's moment, over the map's top right: the units the track says to show, in its
 * order (the boss's own, then the adds to burst), seen in the last BOSS_GONE_MS, with their health as a bar. */
function BossBox({ track, now }: { track: KeyTrack; now: number }) {
  const units = (track.health ?? [])
    .map((guid) => lastAt(track.bosses?.[guid] ?? [], now))
    .filter((p) => p !== undefined && p.hp !== undefined && now - p.t <= BOSS_GONE_MS)
    .slice(0, BOSS_BOX_UNITS)
  if (units.length === 0) return null
  return (
    <div className="map-box boss-box" aria-label="boss health">
      {units.map((p, i) => (
        <div key={i} className="boss-hp">
          <span>{p?.name}</span> <b>{p?.hp?.toFixed(1)}%</b>
          <div className="bar">
            <div className="fill" style={{ width: `${p?.hp ?? 0}%` }} />
          </div>
        </div>
      ))}
    </div>
  )
}

/** The focused raider at the replay's moment, over the map's bottom left: what they're casting (with how far along)
 * and their last few casts, newest first. Nothing for a track without casts. */
function CastBox({ track, player, now }: { track: KeyTrack; player: string; now: number }) {
  const casts = track.casts?.[player]
  if (!casts) return null
  const started = casts.filter((c) => c.t <= now)
  const current = started.filter((c) => c.e !== undefined && c.e > now).pop()
  const recent = started
    .filter((c) => c !== current)
    .slice(-CAST_BOX_CASTS)
    .reverse()
  const color = classColors[track.classes[player]] ?? 'var(--accent)'
  return (
    <div className="map-box cast-box" aria-label={`${player}'s casts`}>
      <b style={{ color }}>{player}</b>
      {current && current.e !== undefined && (
        <div>
          Casting {current.s}
          <div className="bar">
            <div className="fill" style={{ width: `${(100 * (now - current.t)) / (current.e - current.t)}%` }} />
          </div>
        </div>
      )}
      {recent.map((c, i) => (
        <div key={i} className="muted">
          {c.s}
        </div>
      ))}
    </div>
  )
}

/** The focused raider's damage taken up to the replay's moment, over the map's bottom right: their TAKEN_BOX_ROWS
 * abilities that took the most. Nothing before they took any. */
function TakenBox({ taken }: { taken: AbilitiesUpTo['taken'] }) {
  const top = [...taken].sort((a, b) => b.damage - a.damage).slice(0, TAKEN_BOX_ROWS)
  if (top.length === 0) return null
  return (
    <div className="map-box taken-box" aria-label="damage taken">
      <b>Damage taken</b>
      {top.map((h) => (
        <div key={h.ability} className="muted">
          {h.ability} <b>{amount(h.damage)}</b>
        </div>
      ))}
    </div>
  )
}
