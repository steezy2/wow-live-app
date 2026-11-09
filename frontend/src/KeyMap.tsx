import { useEffect, useMemo, useRef, useState } from 'react'
import {
  fetchDungeonMap,
  fetchTrack,
  subscribe,
  TILES_PER_SIDE,
  tileURL,
  type DungeonFloor,
  type DungeonMap,
  type KeyMeter,
  type KeyMeterRow,
  type KeyTrack,
  type KeyView,
  type TrackPoint,
} from './api'
import { classColors } from './classColors'
import { defense, offense } from './meterColumns'
import { MeterCard } from './MeterCards'
import { clock, valueAt } from './format'

/** The trail the live map draws. */
const LIVE_TRAIL_MS = 30_000
const SPEEDS = [1, 2, 4, 8, 16, 32]
const TRAILS = [10_000, 30_000, 120_000, Infinity]
/** Above this many yards a second between two samples is a teleport, so the line breaks. */
const JUMP_YARDS_PER_S = 15

/** The party on the dungeon's map: live, the last 30 s of trails; for a finished key, a replay with controls. Floors
 * Keystone.guru maps are drawn on its tiles; any other floor's shape is where parties have walked in its saved runs. */
export function KeyMap({ keyView: key, clockMS }: { keyView: KeyView; clockMS: number }) {
  const live = key.status === 'running'
  const [track, setTrack] = useState<KeyTrack | null>(null)
  const [dungeon, setDungeon] = useState<DungeonMap>({ floors: [], walked: [] })
  const [at, setAt] = useState(clockMS)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(8)
  const [trail, setTrail] = useState(30_000)
  const [follow, setFollow] = useState('')
  const [floor, setFloor] = useState<number | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    const read = () => fetchTrack(key.start, controller.signal).then(setTrack, () => undefined)
    if (!live) {
      void read()
      return () => controller.abort()
    }
    const unsubscribe = subscribe(() => void read())
    return () => {
      controller.abort()
      unsubscribe()
    }
  }, [key.start, live])

  useEffect(() => {
    const controller = new AbortController()
    fetchDungeonMap(key.mapID, controller.signal).then(setDungeon, () => undefined)
    return () => controller.abort()
  }, [key.mapID])

  useEffect(() => {
    if (!playing) return
    const step = 100
    const timer = setInterval(() => setAt((t) => Math.min(t + step * speed, clockMS)), step)
    return () => clearInterval(timer)
  }, [playing, speed, clockMS])

  const now = live ? clockMS : at
  const shown = live ? LIVE_TRAIL_MS : trail
  const floors = useMemo(() => {
    const counts = new Map<number, number>()
    Object.values(track?.players ?? {}).forEach((ps) =>
      ps.forEach((p) => counts.set(p.map, (counts.get(p.map) ?? 0) + 1)),
    )
    return [...counts].sort((a, b) => b[1] - a[1]).map(([m]) => m)
  }, [track])
  const followed = follow ? track?.players[follow] : undefined
  const current = followed ? lastAt(followed, now)?.map : (floor ?? floors[0])

  if (!track || floors.length === 0) return <p className="muted">No positions yet (needs Advanced Combat Logging).</p>
  return (
    <section className="map">
      <div className="map-controls">
        {!live && (
          <>
            <button
              onClick={() => {
                if (!playing && at >= clockMS) setAt(0)
                setPlaying(!playing)
              }}
            >
              {playing ? 'Pause' : 'Play'}
            </button>
            <div className="replay-slider">
              <input
                type="range"
                min={0}
                max={clockMS}
                step={1000}
                value={at}
                onChange={(e) => setAt(Number(e.target.value))}
                aria-label="replay time"
              />
              <div className="map-marks">
                {key.bosses
                  .filter((b) => b.kill && b.endMS !== undefined)
                  .map((b) => (
                    <span
                      key={b.name}
                      className="boss"
                      title={`${b.name} killed at ${clock(b.endMS ?? 0)}`}
                      style={{ left: `${((b.endMS ?? 0) / clockMS) * 100}%` }}
                    />
                  ))}
                {key.deaths.map((d, i) => (
                  <span
                    key={i}
                    className="death"
                    title={`${d.player} died at ${clock(d.clockMS)}`}
                    style={{ left: `${(d.clockMS / clockMS) * 100}%` }}
                  />
                ))}
              </div>
            </div>
            <span>{clock(at)}</span>
            <select value={speed} onChange={(e) => setSpeed(Number(e.target.value))} aria-label="speed">
              {SPEEDS.map((s) => (
                <option key={s} value={s}>
                  {s}×
                </option>
              ))}
            </select>
            <select value={trail} onChange={(e) => setTrail(Number(e.target.value))} aria-label="trail">
              {TRAILS.map((t) => (
                <option key={t} value={t}>
                  {t === Infinity ? 'whole path' : `${t / 1000} s trail`}
                </option>
              ))}
            </select>
          </>
        )}
        <select value={follow} onChange={(e) => setFollow(e.target.value)} aria-label="follow">
          <option value="">Whole party</option>
          {Object.keys(track.players).map((p) => (
            <option key={p}>{p}</option>
          ))}
        </select>
        {floors.length > 1 &&
          floors.map((m, i) => (
            <button key={m} aria-pressed={m === current} onClick={() => setFloor(m)} disabled={!!followed}>
              Floor {i + 1}
            </button>
          ))}
      </div>
      <div className="map-body">
        <MapCanvas
          track={track}
          dungeon={dungeon}
          mapID={key.mapID}
          floor={current ?? floors[0]}
          now={now}
          trailMS={shown}
        />
        <ReplayMeters track={track} now={now} />
      </div>
      {dungeon.floors.some((f) => f.uiMapID === (current ?? floors[0])) && (
        <p className="muted small">
          Map:{' '}
          <a href="https://keystone.guru" target="_blank" rel="noreferrer">
            Keystone.guru
          </a>
        </p>
      )}
    </section>
  )
}

/** A meter row with nothing in it, for the fields the replay's track doesn't keep. */
const EMPTY_ROW: KeyMeterRow = {
  player: '',
  class: '',
  damage: 0,
  healing: 0,
  dps: 0,
  hps: 0,
  interrupts: 0,
  dispels: 0,
  purges: 0,
  damageTaken: 0,
  avoidable: 0,
  priorityDamage: 0,
}

/** The replay's meters at its moment, like the live cards: DPS and HPS for the pull it's in (or the last one before
 * it) and for the key so far. Keys saved before pulls were tracked show the key so far only. */
function ReplayMeters({ track, now }: { track: KeyTrack; now: number }) {
  const started = (track.pulls ?? []).filter((p) => p.startMS <= now)
  const pull = started.length > 0 ? started[started.length - 1] : undefined
  const meter = (from: number, to: number): KeyMeter => ({
    startMS: from,
    endMS: to,
    ongoing: false,
    rows: Object.entries(track.meters ?? {}).map(([player, points]) => {
      const seconds = Math.max(1, (to - from) / 1000)
      const damage = valueAt(points, to, 'd') - valueAt(points, from, 'd')
      const healing = valueAt(points, to, 'h') - valueAt(points, from, 'h')
      const role = { player, class: track.classes[player] ?? '', damage, healing }
      return { ...EMPTY_ROW, ...role, dps: damage / seconds, hps: healing / seconds }
    }),
  })
  const meters: [string, KeyMeter][] = [['Whole key', meter(0, now)]]
  if (pull) {
    const end = pull.endMS > 0 && pull.endMS < now ? pull.endMS : now
    meters.unshift([`Pull ${started.length}`, meter(pull.startMS, end)])
  }
  return (
    <div className="replay-meters">
      {[offense.slice(0, 2), defense(false).slice(0, 2)].map((columns) =>
        meters.map(([title, m]) => (
          <MeterCard key={columns[0].label + title} title={title} meter={m} columns={columns} />
        )),
      )}
    </div>
  )
}

/** Map tiles already loaded, by URL, so a redraw doesn't fetch them again. */
const tiles = new Map<string, HTMLImageElement>()

/** The map itself: the floor's tiles (or, without them, the walked area faintly), each player's trail and dot in their
 * class colour, and mob deaths so far. */
function MapCanvas(props: {
  track: KeyTrack
  dungeon: DungeonMap
  mapID: number
  floor: number
  now: number
  trailMS: number
}) {
  const { track, dungeon, mapID, floor, now, trailMS } = props
  const ref = useRef<HTMLCanvasElement>(null)
  const [loaded, setLoaded] = useState(0)
  const tiled = dungeon.floors.find((f) => f.uiMapID === floor)
  useEffect(() => {
    const canvas = ref.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    const at = tiled ? onTiles(tiled, canvas) : onWalked(dungeon.walked, track, floor, canvas)
    if (!at) return
    const style = getComputedStyle(canvas)
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (tiled) {
      const [w, h] = [canvas.width / TILES_PER_SIDE, canvas.height / TILES_PER_SIDE]
      for (let x = 0; x < TILES_PER_SIDE; x++) {
        for (let y = 0; y < TILES_PER_SIDE; y++) {
          const url = tileURL(mapID, floor, x, y)
          let img = tiles.get(url)
          if (!img) {
            img = new Image()
            img.onload = () => setLoaded((n) => n + 1)
            img.src = url
            tiles.set(url, img)
          }
          if (img.complete && img.naturalWidth > 0) ctx.drawImage(img, x * w, y * h, w, h)
        }
      }
    } else {
      ctx.fillStyle = style.getPropertyValue('--muted')
      ctx.globalAlpha = 0.18
      dungeon.walked
        .filter((p) => p.map === floor)
        .forEach((p) => {
          const [x, y] = at(p)
          ctx.fillRect(x - 2, y - 2, 4, 4)
        })
      ctx.globalAlpha = 1
    }
    const dot = canvas.width / 160
    ctx.fillStyle = style.getPropertyValue('--bad')
    track.deaths
      .filter((d) => d.map === floor && d.t <= now)
      .forEach((d) => {
        const [x, y] = at(d)
        ctx.fillRect(x - dot / 2, y - dot / 2, dot, dot)
      })
    Object.entries(track.players).forEach(([player, points]) => {
      const colour = classColors[track.classes[player]] ?? style.getPropertyValue('--accent')
      const seen = points.filter((p) => p.t <= now && p.t >= now - trailMS && p.map === floor)
      ctx.strokeStyle = colour
      ctx.lineWidth = dot / 2
      ctx.beginPath()
      seen.forEach((p, i) => {
        const [x, y] = at(p)
        const prev = seen[i - 1]
        const jump =
          prev && Math.hypot(p.x - prev.x, p.y - prev.y) / Math.max(1, (p.t - prev.t) / 1000) > JUMP_YARDS_PER_S
        if (i === 0 || jump) ctx.moveTo(x, y)
        else ctx.lineTo(x, y)
      })
      ctx.stroke()
      const last = seen[seen.length - 1]
      if (last) {
        const [x, y] = at(last)
        ctx.fillStyle = colour
        ctx.strokeStyle = '#000'
        ctx.lineWidth = dot / 4
        ctx.beginPath()
        ctx.arc(x, y, dot * 1.2, 0, 2 * Math.PI)
        ctx.fill()
        ctx.stroke()
      }
    })
  }, [track, dungeon, mapID, floor, now, trailMS, tiled, loaded])
  return (
    <canvas
      ref={ref}
      width={tiled ? 1536 : 960}
      height={tiled ? 1024 : 640}
      className="map-canvas"
      role="img"
      aria-label="party on the map"
    />
  )
}

/** Where a log position lands on a Keystone.guru floor: its x across is the log's -y within the floor's bounds,
 * its y down the log's x (both as Keystone.guru's CoordinatesService computes them). */
function onTiles(floor: DungeonFloor, canvas: HTMLCanvasElement) {
  return (p: TrackPoint): [number, number] => [
    ((floor.maxX + p.y) / (floor.maxX - floor.minX)) * canvas.width,
    ((floor.maxY - p.x) / (floor.maxY - floor.minY)) * canvas.height,
  ]
}

/** Where a log position lands on a floor without tiles: fitted to the walked area and the track, north up. */
function onWalked(walked: TrackPoint[], track: KeyTrack, floor: number, canvas: HTMLCanvasElement) {
  const all = [...walked, ...Object.values(track.players).flat()].filter((p) => p.map === floor)
  if (all.length === 0) return undefined
  // A loop, not Math.min(...points): a season of runs can pass the engine's limit on arguments.
  let [minX, maxX, minY, maxY] = [Infinity, -Infinity, Infinity, -Infinity]
  for (const p of all) {
    ;[minX, maxX, minY, maxY] = [Math.min(minX, -p.y), Math.max(maxX, -p.y), Math.min(minY, p.x), Math.max(maxY, p.x)]
  }
  const pad = 12
  const scale = Math.min((canvas.width - 2 * pad) / (maxX - minX || 1), (canvas.height - 2 * pad) / (maxY - minY || 1))
  return (p: TrackPoint): [number, number] => [pad + (-p.y - minX) * scale, canvas.height - pad - (p.x - minY) * scale]
}

/** A player's last position at or before a clock reading. */
function lastAt(points: TrackPoint[], now: number): TrackPoint | undefined {
  let last: TrackPoint | undefined
  for (const p of points) {
    if (p.t > now) break
    last = p
  }
  return last
}
