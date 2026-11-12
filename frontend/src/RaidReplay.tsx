import { useMemo, useRef, useState, type CSSProperties, type MouseEvent } from 'react'
import type { KeyTrack, RaidLane, RaidPull } from './api'
import { clock } from './format'
import { MapCanvas, ReplayControls, type Backdrop } from './KeyMap'
import { abilitiesUpTo, floorsOf, tickStep, type SliderMark, type useReplay } from './replay'
import { MeterGraph } from './DamageGraph'
import { BossBox, FocusBoxes } from './ReplayBoxes'

/** A raid pull's replay: the mechanics' lanes and the DPS and HPS graph, the play bar, then the raid and the boss on
 * the room's map, all on the page's one clock: a click on the lanes or the graph moves the map there. A click on a
 * raider (their dot or their line) focuses them: a ring and their name on the map, what they're casting and what hit
 * them over it, and their line alone on the graph; a click on an enemy shows its casts; a right click lets go of
 * both. The track is the page's (useRaidTrack): undefined while it loads, null for a pull saved without one. */
export function RaidReplay(props: {
  pull: RaidPull
  track: KeyTrack | null | undefined
  error: string | null
  replay: ReturnType<typeof useReplay>
  focus: string
  onFocus: (player: string) => void
}) {
  const { pull, track, error, replay, focus, onFocus } = props
  const end = pull.durationMS
  const [trail, setTrail] = useState(0)
  const [floor, setFloor] = useState<number | null>(null)
  const [enemy, setEnemy] = useState('')
  const floors = useMemo(() => floorsOf(track ?? null), [track])
  const current = floor ?? floors[0]
  const room = pull.maps.find((m) => m.uiMapID === current)
  const backdrop = useMemo<Backdrop>(
    () => (room ? { kind: 'image', map: room } : { kind: 'walked', walked: [] }),
    [room],
  )
  const marks: SliderMark[] = pull.deaths.map((d) => ({
    clockMS: d.clockMS,
    className: 'death',
    title: `${d.player} died at ${clock(d.clockMS)}`,
  }))
  if (error) return <p className="panel warning">Couldn't read the replay: {error}</p>
  if (track === null) {
    return <p className="panel muted">No replay for this pull: it was saved before replays were kept.</p>
  }
  if (!track) return <p className="panel muted">Loading the replay…</p>
  const now = replay.at
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
        <>
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
              enemy={enemy}
              onPickEnemy={setEnemy}
            />
            <BossBox track={track} now={now} />
            <FocusBoxes track={track} player={focus} enemy={enemy} taken={upTo?.taken} now={now} />
          </div>
          <p className="muted small">
            Scroll to zoom, drag to move, double click for the whole room · click a raider to focus them or an enemy for
            its casts, right click for neither
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
        </>
      )}
    </section>
  )
}

/** The pull's timeline: a lane per mechanic with its marks (a stretch for a phase, an add's life or how long a buff
 * lasted), the ones that went wrong in red, over a ruler with a tick every tickStep. One line through every lane is
 * the replay's moment, with its time on the ruler. A mouse over a mark shows its lane, time and text in a box at once;
 * a click or a drag anywhere on the lanes moves the replay there, and a click on a mark to its start. */
export function RaidLanes(props: { lanes: RaidLane[]; end: number; now: number; onSeek: (clockMS: number) => void }) {
  const { lanes, end, now, onSeek } = props
  const ruler = useRef<HTMLDivElement>(null)
  const root = useRef<HTMLDivElement>(null)
  const [tip, setTip] = useState<{ x: number; y: number; lane: string; when: string; text: string } | null>(null)
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
      ref={root}
      role="list"
      aria-label="the pull's mechanics over time"
      onMouseDown={(e) => e.button === 0 && seek(e)}
      onMouseMove={(e) => e.buttons === 1 && seek(e)}
      onMouseLeave={() => setTip(null)}
    >
      {lanes.map((l) => (
        <div key={`${l.kind}-${l.name}`} className="lane" role="listitem">
          <span className="lane-name" title={l.name}>
            {l.name} <span className="muted">{l.kind === 'phases' ? '' : l.marks.length}</span>
          </span>
          <div className={`lane-track lane-${l.kind}`}>
            {l.marks.map((m, i) => {
              const when = `${clock(m.clockMS)}${m.endMS ? `–${clock(m.endMS)}` : ''}`
              return (
                <button
                  key={i}
                  className={`${m.endMS ? 'lane-span' : 'lane-mark'}${m.bad ? ' bad-mark' : ''}`}
                  style={m.endMS ? { left: at(m.clockMS), width: at(m.endMS - m.clockMS) } : { left: at(m.clockMS) }}
                  aria-label={`${when} ${m.text}`}
                  onMouseEnter={(e) => {
                    const mark = e.currentTarget.getBoundingClientRect()
                    const box = root.current?.getBoundingClientRect()
                    if (!box) return
                    const x = mark.left + mark.width / 2 - box.left
                    setTip({ x, y: mark.top - box.top, lane: l.name, when, text: m.text })
                  }}
                  onMouseLeave={() => setTip(null)}
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
      {tip && (
        <div className="lane-tip" style={{ left: tip.x, top: tip.y }} role="tooltip">
          <span className="muted">
            {tip.lane} · {tip.when}
          </span>
          {tip.text}
        </div>
      )}
    </div>
  )
}
