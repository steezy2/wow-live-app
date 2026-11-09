import { useEffect, useMemo, useRef, useState } from 'react'
import {
  fetchDungeonMap,
  fetchTrack,
  raidMapURL,
  subscribe,
  TILES_PER_SIDE,
  tileURL,
  type DungeonFloor,
  type DungeonMap,
  type KeyMeter,
  type KeyTrack,
  type KeyView,
  type RaidMap,
  type TrackPoint,
} from './api'
import { classColors } from './classColors'
import { defense, offense } from './meterColumns'
import { MeterCard } from './MeterCards'
import { clock } from './format'
import { floorsOf, lastAt, meterBetween, useReplay, type SliderMark } from './replay'

/** The trail the live map draws. */
const LIVE_TRAIL_MS = 30_000
const SPEEDS = [1, 2, 4, 8, 16, 32]
const TRAILS = [10_000, 30_000, 120_000, Infinity]
/** Above this many yards a second between two samples is a teleport, so the line breaks. */
const JUMP_YARDS_PER_S = 15
/** A raid replay shows at least this share of the room's image, padded by this share of the fight on each side. */
const FIGHT_MIN_SHARE = 0.2
const FIGHT_PAD = 0.15
/** A player seen this long after their death is alive again. */
const REVIVED_AFTER_MS = 2_000
/** A boss unit not seen for this long (gone, or submerged) isn't drawn. */
const BOSS_GONE_MS = 5_000

/** The replay's controls: play, the slider with its marks, the time, the speed and the trail. */
export function ReplayControls(props: {
  replay: ReturnType<typeof useReplay>
  end: number
  marks: SliderMark[]
  trail: number
  setTrail: (t: number) => void
}) {
  const { replay, end, marks, trail, setTrail } = props
  return (
    <>
      <button onClick={replay.toggle}>{replay.playing ? 'Pause' : 'Play'}</button>
      <div className="replay-slider">
        <input
          type="range"
          min={0}
          max={end}
          step={1000}
          value={replay.at}
          onChange={(e) => replay.setAt(Number(e.target.value))}
          aria-label="replay time"
        />
        <div className="map-marks">
          {marks.map((m, i) => (
            <span key={i} className={m.className} title={m.title} style={{ left: `${(m.clockMS / end) * 100}%` }} />
          ))}
        </div>
      </div>
      <span>{clock(replay.at)}</span>
      <select value={replay.speed} onChange={(e) => replay.setSpeed(Number(e.target.value))} aria-label="speed">
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
  )
}

/** The party on the dungeon's map: live, the last 30 s of trails; for a finished key, a replay with controls. Floors
 * Keystone.guru maps are drawn on its tiles; any other floor's shape is where parties have walked in its saved runs. */
export function KeyMap({ keyView: key, clockMS }: { keyView: KeyView; clockMS: number }) {
  const live = key.status === 'running'
  const [track, setTrack] = useState<KeyTrack | null>(null)
  const [dungeon, setDungeon] = useState<DungeonMap>({ floors: [], walked: [] })
  const replay = useReplay(clockMS, clockMS)
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

  const now = live ? clockMS : replay.at
  const floors = useMemo(() => floorsOf(track), [track])
  const followed = follow ? track?.players[follow] : undefined
  const current = (followed ? lastAt(followed, now)?.map : floor) ?? floors[0]
  const tiled = dungeon.floors.find((f) => f.uiMapID === current)
  const backdrop = useMemo<Backdrop>(
    () => (tiled ? { kind: 'tiles', floor: tiled, mapID: key.mapID } : { kind: 'walked', walked: dungeon.walked }),
    [tiled, key.mapID, dungeon.walked],
  )
  const marks: SliderMark[] = [
    ...key.bosses
      .filter((b) => b.kill && b.endMS !== undefined)
      .map((b) => ({ clockMS: b.endMS ?? 0, className: 'boss', title: `${b.name} killed at ${clock(b.endMS ?? 0)}` })),
    ...key.deaths.map((d) => ({
      clockMS: d.clockMS,
      className: 'death',
      title: `${d.player} died at ${clock(d.clockMS)}`,
    })),
  ]

  if (!track || floors.length === 0) return <p className="muted">No positions yet (needs Advanced Combat Logging).</p>
  return (
    <section className="map">
      <div className="map-controls">
        {!live && <ReplayControls replay={replay} end={clockMS} marks={marks} trail={trail} setTrail={setTrail} />}
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
          backdrop={backdrop}
          floor={current}
          now={now}
          trailMS={live ? LIVE_TRAIL_MS : trail}
          deaths={key.deaths}
        />
        <ReplayMeters track={track} now={now} />
      </div>
      {tiled && (
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

/** The replay's meters at its moment, like the live cards: DPS and HPS for the pull it's in (or the last one before
 * it) and for the key so far. Keys saved before pulls were tracked show the key so far only. */
function ReplayMeters({ track, now }: { track: KeyTrack; now: number }) {
  const started = (track.pulls ?? []).filter((p) => p.startMS <= now)
  const pull = started.length > 0 ? started[started.length - 1] : undefined
  const meters: [string, KeyMeter][] = [['Whole key', meterBetween(track, 0, now)]]
  if (pull) {
    const end = pull.endMS > 0 && pull.endMS < now ? pull.endMS : now
    meters.unshift([`Pull ${started.length}`, meterBetween(track, pull.startMS, end)])
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

/** Map images and tiles already loaded, by URL, so a redraw doesn't fetch them again. */
const images = new Map<string, HTMLImageElement>()

/** What a replay draws under the players: a dungeon floor's Keystone.guru tiles, a raid room's map image, or (with
 * neither) the walked area faintly. */
export type Backdrop =
  | { kind: 'tiles'; floor: DungeonFloor; mapID: number }
  | { kind: 'image'; map: RaidMap }
  | { kind: 'walked'; walked: TrackPoint[] }

/** The canvas size for a backdrop: tiles and a room's image at their own shape, the walked area at 3:2. */
function canvasSize(backdrop: Backdrop): [number, number] {
  switch (backdrop.kind) {
    case 'tiles':
      return [1536, 1024]
    case 'image': {
      const m = backdrop.map
      return [1536, Math.round((1536 * (m.maxX - m.minX)) / (m.maxY - m.minY))]
    }
    case 'walked':
      return [960, 640]
  }
}

/** The map itself: the backdrop, each player's trail and dot in their class colour (a cross where they died), a raid
 * boss's units, and mob deaths so far. */
export function MapCanvas(props: {
  track: KeyTrack
  backdrop: Backdrop
  floor: number
  now: number
  trailMS: number
  deaths: { player: string; clockMS: number }[]
}) {
  const { track, backdrop, floor, now, trailMS, deaths } = props
  const ref = useRef<HTMLCanvasElement>(null)
  const [loaded, setLoaded] = useState(0)
  const [width, height] = canvasSize(backdrop)
  const area = useMemo<[number, number, number]>(
    () => (backdrop.kind === 'image' ? fightArea(backdrop.map, track, floor) : [0, 0, 1]),
    [backdrop, track, floor],
  )
  useEffect(() => {
    const canvas = ref.current
    const ctx = canvas?.getContext('2d')
    if (!canvas || !ctx) return
    const at =
      backdrop.kind === 'tiles'
        ? onTiles(backdrop.floor, canvas)
        : backdrop.kind === 'image'
          ? onImage(backdrop.map, area, canvas)
          : onWalked(backdrop.walked, track, floor, canvas)
    if (!at) return
    const image = (url: string) => {
      let img = images.get(url)
      if (!img) {
        img = new Image()
        img.onload = () => setLoaded((n) => n + 1)
        img.src = url
        images.set(url, img)
      }
      return img.complete && img.naturalWidth > 0 ? img : undefined
    }
    const style = getComputedStyle(canvas)
    ctx.clearRect(0, 0, canvas.width, canvas.height)
    if (backdrop.kind === 'tiles') {
      const [w, h] = [canvas.width / TILES_PER_SIDE, canvas.height / TILES_PER_SIDE]
      for (let x = 0; x < TILES_PER_SIDE; x++) {
        for (let y = 0; y < TILES_PER_SIDE; y++) {
          const img = image(tileURL(backdrop.mapID, floor, x, y))
          if (img) ctx.drawImage(img, x * w, y * h, w, h)
        }
      }
    } else if (backdrop.kind === 'image') {
      const img = image(raidMapURL(backdrop.map.uiMapID))
      const [left, top, share] = area
      const [w, h] = [img?.naturalWidth ?? 0, img?.naturalHeight ?? 0]
      if (img) ctx.drawImage(img, left * w, top * h, share * w, share * h, 0, 0, canvas.width, canvas.height)
    } else {
      ctx.fillStyle = style.getPropertyValue('--muted')
      ctx.globalAlpha = 0.18
      backdrop.walked
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
    Object.values(track.bosses ?? {}).forEach((points) => {
      const last = lastAt(points, now)
      if (!last || last.map !== floor || now - last.t > BOSS_GONE_MS) return
      const [x, y] = at(last)
      ctx.fillStyle = '#000'
      ctx.strokeStyle = style.getPropertyValue('--warn')
      ctx.lineWidth = dot / 2
      ctx.beginPath()
      ctx.arc(x, y, dot * 2.4, 0, 2 * Math.PI)
      ctx.fill()
      ctx.stroke()
      ctx.fillStyle = style.getPropertyValue('--text')
      ctx.font = `${Math.round(dot * 2.6)}px system-ui, sans-serif`
      ctx.textAlign = 'center'
      ctx.fillText(last.name ?? '', x, y - dot * 3.4)
    })
    const died = new Map<string, number>() // each player's last death so far
    deaths
      .filter((d) => d.clockMS <= now)
      .forEach((d) => died.set(d.player, Math.max(died.get(d.player) ?? 0, d.clockMS)))
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
      // Where they are: their last place even past the trail, and before their first sample where they were first seen.
      // Dead until they're seen again after their last death (a release and run back, or a battle res).
      const last = lastAt(points, now) ?? points[0]
      const dead = died.has(player) && (!last || last.t <= (died.get(player) ?? 0) + REVIVED_AFTER_MS)
      if (!last || last.map !== floor) return
      const [x, y] = at(last)
      if (dead) {
        ctx.strokeStyle = style.getPropertyValue('--bad')
        ctx.lineWidth = dot / 2
        ctx.beginPath()
        ctx.moveTo(x - dot, y - dot)
        ctx.lineTo(x + dot, y + dot)
        ctx.moveTo(x + dot, y - dot)
        ctx.lineTo(x - dot, y + dot)
        ctx.stroke()
        return
      }
      ctx.fillStyle = colour
      ctx.strokeStyle = '#000'
      ctx.lineWidth = dot / 4
      ctx.beginPath()
      ctx.arc(x, y, dot * 1.2, 0, 2 * Math.PI)
      ctx.fill()
      ctx.stroke()
    })
  }, [track, backdrop, area, floor, now, trailMS, deaths, loaded])
  return (
    <canvas ref={ref} width={width} height={height} className="map-canvas" role="img" aria-label="players on the map" />
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

/** Where a log position lands on a raid room's whole image, as shares of its width and height: across from its maxY
 * (west, the left edge), down from its maxX (north, the top), as the game's world map draws it. */
function onRoom(map: RaidMap, p: TrackPoint): [number, number] {
  return [(map.maxY - p.y) / (map.maxY - map.minY), (map.maxX - p.x) / (map.maxX - map.minX)]
}

/** The part of a raid room's image the pull used, so a fight in a corner of a big map fills the canvas: around every
 * position of the raid and the boss on it, padded, at least FIGHT_MIN_SHARE of the image, as its left and top edges
 * and its share of the image's width and height (one share, so the canvas keeps the image's shape). */
function fightArea(map: RaidMap, track: KeyTrack, floor: number): [number, number, number] {
  let [l, t, r, b] = [1, 1, 0, 0]
  for (const points of [...Object.values(track.players), ...Object.values(track.bosses ?? {})]) {
    for (const p of points) {
      if (p.map !== floor) continue
      const [u, v] = onRoom(map, p)
      ;[l, t, r, b] = [Math.min(l, u), Math.min(t, v), Math.max(r, u), Math.max(b, v)]
    }
  }
  if (l > r) return [0, 0, 1]
  const share = Math.min(1, Math.max(FIGHT_MIN_SHARE, Math.max(r - l, b - t) * (1 + 2 * FIGHT_PAD)))
  const clamp = (v: number) => Math.min(1 - share, Math.max(0, v))
  return [clamp((l + r - share) / 2), clamp((t + b - share) / 2), share]
}

/** Where a log position lands on the canvas showing a part of a raid room's image (fightArea). */
function onImage(map: RaidMap, [left, top, share]: [number, number, number], canvas: HTMLCanvasElement) {
  return (p: TrackPoint): [number, number] => {
    const [u, v] = onRoom(map, p)
    return [((u - left) / share) * canvas.width, ((v - top) / share) * canvas.height]
  }
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
