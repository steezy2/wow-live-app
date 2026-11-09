import { useEffect, useMemo, useState } from 'react'
import { fetchRaidTrack, type KeyMeter, type KeyTrack, type RaidLane, type RaidPull } from './api'
import { clock } from './format'
import { MapCanvas, ReplayControls, type Backdrop } from './KeyMap'
import { floorsOf, meterBetween, topRows, useReplay, type SliderMark } from './replay'
import { MeterGraph } from './DamageGraph'
import { MeterCard } from './MeterCards'
import { defense, raidOffense } from './meterColumns'

/** How many raiders the replay's meters list, top first. */
const REPLAY_METER_ROWS = 10

/** A raid pull's replay: the raid and the boss on the room's map with the meters at that moment, the mechanics'
 * lanes, and the DPS and HPS graph, all on one clock: a click on a lane's mark or on the graph moves the map there. */
export function RaidReplay({ pull }: { pull: RaidPull }) {
  const [track, setTrack] = useState<KeyTrack | null>(null)
  const [missing, setMissing] = useState(false)
  const end = pull.durationMS
  const replay = useReplay(end, 0, 4)
  const [trail, setTrail] = useState(10_000)
  const [floor, setFloor] = useState<number | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchRaidTrack(pull.start, controller.signal).then(setTrack, () => {
      if (!controller.signal.aborted) setMissing(true)
    })
    return () => controller.abort()
  }, [pull.start])
  const floors = useMemo(() => floorsOf(track), [track])
  const current = floor ?? floors[0]
  const room = pull.maps.find((m) => m.uiMapID === current)
  const backdrop = useMemo<Backdrop>(() => (room ? { kind: 'image', map: room } : { kind: 'walked', walked: [] }), [room])
  const marks: SliderMark[] = pull.deaths.map((d) => ({
    clockMS: d.clockMS,
    className: 'death',
    title: `${d.player} died at ${clock(d.clockMS)}`,
  }))
  if (missing) return <p className="panel muted">No replay for this pull: it was saved before replays were kept.</p>
  if (!track) return <p className="panel muted">Loading the replay…</p>
  const now = replay.at
  const sofar = meterBetween(track, 0, now)
  return (
    <section className="panel replay">
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
          <MapCanvas track={track} backdrop={backdrop} floor={current} now={now} trailMS={trail} deaths={pull.deaths} />
          <div className="replay-meters raid">
            <MeterCard title="So far" meter={top(sofar, 'dps')} columns={raidOffense.slice(0, 2)} />
            <MeterCard title="So far" meter={top(sofar, 'hps')} columns={defense(false).slice(0, 2)} />
          </div>
        </div>
      )}
      <RaidLanes lanes={pull.lanes} end={end} now={now} onSeek={replay.setAt} />
      <MeterGraph
        track={track}
        end={end}
        marks={marks}
        group="raid"
        over="pull"
        now={now}
        onSeek={replay.setAt}
      />
      {room && (
        <p className="muted small">
          Map:{' '}
          <a href="https://www.warcraftlogs.com" target="_blank" rel="noreferrer">
            Warcraft Logs
          </a>
        </p>
      )}
    </section>
  )
}

/** A meter with only its top rows by a column. */
function top(meter: KeyMeter, by: 'dps' | 'hps'): KeyMeter {
  return { ...meter, rows: topRows(meter.rows, by, REPLAY_METER_ROWS) }
}

/** The pull's timeline: a lane per mechanic with its marks (a stretch for a phase or an add's life), the ones that went
 * wrong in red, and the replay's moment as a line; a click on a mark moves the replay there. */
export function RaidLanes(props: { lanes: RaidLane[]; end: number; now: number; onSeek: (clockMS: number) => void }) {
  const { lanes, end, now, onSeek } = props
  const at = (t: number) => `${(Math.min(t, end) / Math.max(1, end)) * 100}%`
  if (lanes.length === 0) return null
  return (
    <div className="lanes" role="list" aria-label="the pull's mechanics over time">
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
                  onClick={() => onSeek(m.clockMS)}
                >
                  {l.kind === 'phases' ? m.text : ''}
                </button>
              )
            })}
            <span className="lane-now" style={{ left: at(now) }} />
          </div>
        </div>
      ))}
    </div>
  )
}
