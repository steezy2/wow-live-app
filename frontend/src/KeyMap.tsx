import { useEffect, useMemo, useRef, useState, type PointerEvent } from 'react'
import {
  fetchDungeonMap,
  raidMapURL,
  TILES_PER_SIDE,
  tileURL,
  type DungeonFloor,
  type DungeonMap,
  type KeyTrack,
  type KeyView,
  type RaidMap,
  type TrackPoint,
} from './api'
import { classColors } from './classColors'
import { clock } from './format'
import {
  abilitiesUpTo,
  BOSS_GONE_MS,
  enemyName,
  floorsOf,
  JUMP_YARDS_PER_S,
  indexAfter,
  lastAt,
  MOB_GONE_MS,
  positionAt,
  type SliderMark,
  type useReplay,
} from './replay'
import { BossBox, FocusBoxes } from './ReplayBoxes'

/** The trail the live map draws. */
const LIVE_TRAIL_MS = 30_000
const SPEEDS = [1, 2, 4, 8, 16, 32]
const TRAILS = [0, 10_000, 30_000, 120_000, Infinity]
/** How far the map zooms in, each wheel step's factor, and how far (CSS px) a press moves before it's a drag. */
const MAX_ZOOM = 8
const ZOOM_STEP = 1.25
const DRAG_PX = 4

/** The part of the canvas shown: zoomed k times (1 is all of it) from x and y, its left and top in canvas pixels. */
interface View {
  k: number
  x: number
  y: number
}
const WHOLE: View = { k: 1, x: 0, y: 0 }

/** A view kept inside the canvas and the zoom's range. */
function inside(v: View, canvas: HTMLCanvasElement): View {
  const k = Math.min(MAX_ZOOM, Math.max(1, v.k))
  const within = (at: number, size: number) => Math.min(size - size / k, Math.max(0, at))
  return { k, x: within(v.x, canvas.width), y: within(v.y, canvas.height) }
}

/** Where a pointer is on the canvas in canvas pixels (before the zoom), and how many of them a CSS pixel is; the
 * drawing sits centred in a canvas element that's a different shape (object-fit: contain). */
function onCanvas(e: { clientX: number; clientY: number }, canvas: HTMLCanvasElement): [number, number, number] {
  const box = canvas.getBoundingClientRect()
  const scale = Math.max(canvas.width / box.width, canvas.height / box.height)
  return [
    (e.clientX - box.left - box.width / 2) * scale + canvas.width / 2,
    (e.clientY - box.top - box.height / 2) * scale + canvas.height / 2,
    scale,
  ]
}
/** A raid replay shows at least this share of the room's image, padded by this share of the fight on each side. */
const FIGHT_MIN_SHARE = 0.2
const FIGHT_PAD = 0.15
/** A player seen this long after their death is alive again. */
const REVIVED_AFTER_MS = 2_000

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
            {t === Infinity ? 'whole path' : t === 0 ? 'no trail' : `${t / 1000} s trail`}
          </option>
        ))}
      </select>
    </>
  )
}

/** The party on the dungeon's map (from the key's track): live, the last 30 s of trails; for a finished key, a
 * replay with controls on the page's replay clock. Floors Keystone.guru maps are drawn on its tiles; any other floor's
 * shape is where parties have walked in its saved runs. A click on a player (their dot, or a row in the meter cards)
 * follows them, a click on an enemy shows its casts, and a right click on the map lets go of both. */
export function KeyMap(props: {
  keyView: KeyView
  clockMS: number
  track: KeyTrack | null
  replay: ReturnType<typeof useReplay>
  focus: string
  onFocus: (player: string) => void
}) {
  const { keyView: key, clockMS, track, replay, focus: follow, onFocus: setFollow } = props
  const live = key.status === 'running'
  const [dungeon, setDungeon] = useState<DungeonMap>({ floors: [], walked: [], enemies: [] })
  const [trail, setTrail] = useState(0)
  const [floor, setFloor] = useState<number | null>(null)
  const [enemy, setEnemy] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    fetchDungeonMap(key.mapID, controller.signal).then(setDungeon, () => undefined)
    return () => controller.abort()
  }, [key.mapID])

  const now = live ? clockMS : replay.at
  const floors = useMemo(() => floorsOf(track), [track])
  const followed = follow ? track?.players[follow] : undefined
  const upTo = follow && track ? abilitiesUpTo(track, key, follow, now) : undefined
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
      <div className="map-stage">
        <MapCanvas
          key={current}
          track={track}
          backdrop={backdrop}
          floor={current}
          now={now}
          clock={replay.playing ? replay.now : undefined}
          trailMS={live ? LIVE_TRAIL_MS : trail}
          deaths={key.deaths}
          focus={follow}
          onPick={setFollow}
          enemy={enemy}
          onPickEnemy={setEnemy}
        />
        <BossBox track={track} now={now} />
        <FocusBoxes track={track} player={follow} enemy={enemy} taken={upTo?.taken} now={now} />
      </div>
      <p className="muted small">
        Scroll to zoom, drag to move, double click for the whole floor · click a player to follow them or an enemy for
        its casts, right click for neither
        {tiled && (
          <>
            {' '}
            · map:{' '}
            <a href="https://keystone.guru" target="_blank" rel="noreferrer">
              Keystone.guru
            </a>
          </>
        )}
      </p>
    </section>
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

/** A place in the combat log's yards. */
export type Yards = Pick<TrackPoint, 'x' | 'y'>

/** What a map draws over its backdrop: given where a place lands on the canvas and the size of a dot (the same on
 * screen at any zoom). `hit` makes a dot it drew an enemy a click picks (onPickEnemy) by the id given. */
export type Overlay = (
  ctx: CanvasRenderingContext2D,
  at: (p: Yards) => [number, number],
  dot: number,
  hit: (id: string, x: number, y: number, r: number) => void,
) => void

/** The map itself: the backdrop, each player's trail and dot in their class colour (a cross where they died), a raid
 * boss's units and minibosses (named), the other mobs alive (small, unnamed), and mob deaths so far. The wheel zooms
 * around the pointer, a drag pans and a double click shows all of it again. With onPick, a click on a player's dot
 * picks them and a right click picks nobody (''); the focused player gets a ring and their name, and the others'
 * trails fade. With onPickEnemy, a click on a mob or a boss picks it (by GUID), the right click lets go of it too, and
 * the picked enemy gets a ring and its name. An overlay is drawn on the backdrop, under all of those. */
export function MapCanvas(props: {
  track: KeyTrack
  backdrop: Backdrop
  floor: number
  now: number
  /** While a replay plays: its moment right now, so the map draws every frame between the page's updates. */
  clock?: () => number
  trailMS: number
  deaths: { player: string; clockMS: number }[]
  focus?: string
  onPick?: (player: string) => void
  enemy?: string
  /** Called with whether Shift was held. */
  onPickEnemy?: (guid: string, shift: boolean) => void
  overlay?: Overlay
}) {
  const { track, backdrop, floor, now, clock, trailMS, deaths, focus, onPick, enemy, onPickEnemy, overlay } = props
  const ref = useRef<HTMLCanvasElement>(null)
  const [loaded, setLoaded] = useState(0)
  const [view, setView] = useState(WHOLE)
  const shown = useRef(WHOLE) // the view as last drawn, for the wheel
  // Where each player and enemy (by its GUID) was last drawn, and its dot's radius.
  const dots = useRef<{ id: string; enemy: boolean; x: number; y: number; r: number }[]>([])
  const press = useRef<{ x: number; y: number; dragged: boolean } | null>(null)
  const [width, height] = canvasSize(backdrop)
  useEffect(() => {
    const canvas = ref.current
    if (!canvas) return
    // Not React's onWheel: that listener is passive, so it can't keep the page from scrolling while the map zooms.
    const wheel = (e: WheelEvent) => {
      const from = shown.current
      const k = Math.min(MAX_ZOOM, Math.max(1, from.k * (e.deltaY < 0 ? ZOOM_STEP : 1 / ZOOM_STEP)))
      if (k === from.k) return // all the way out (or in): the page scrolls
      e.preventDefault()
      const [x, y] = onCanvas(e, canvas)
      setView(inside({ k, x: from.x + x / from.k - x / k, y: from.y + y / from.k - y / k }, canvas))
    }
    canvas.addEventListener('wheel', wheel, { passive: false })
    return () => canvas.removeEventListener('wheel', wheel)
  }, [])
  const drag = (e: PointerEvent<HTMLCanvasElement>) => {
    const from = press.current
    if (!from) return
    const [dx, dy] = [e.clientX - from.x, e.clientY - from.y]
    if (!from.dragged && Math.hypot(dx, dy) < DRAG_PX) return
    press.current = { x: e.clientX, y: e.clientY, dragged: true }
    const canvas = e.currentTarget
    const scale = onCanvas(e, canvas)[2]
    setView((v) => inside({ k: v.k, x: v.x - (dx * scale) / v.k, y: v.y - (dy * scale) / v.k }, canvas))
  }
  const release = (e: PointerEvent<HTMLCanvasElement>) => {
    const from = press.current
    press.current = null
    if (!from || from.dragged || !(onPick || onPickEnemy)) return
    const [x, y] = onCanvas(e, e.currentTarget)
    const [atX, atY] = [view.x + x / view.k, view.y + y / view.k]
    const far = (d: { x: number; y: number; r: number }) => Math.hypot(d.x - atX, d.y - atY) - d.r // from its edge
    const reach = ((e.currentTarget.width / 160) * 1.3) / view.k
    const pick = (d: { enemy: boolean }) => (d.enemy ? onPickEnemy : onPick)
    const [nearest] = dots.current.filter((d) => pick(d) && far(d) <= reach).sort((a, b) => far(a) - far(b))
    if (nearest?.enemy) onPickEnemy?.(nearest.id, e.shiftKey)
    else if (nearest) onPick?.(nearest.id)
  }
  const area = useMemo<[number, number, number]>(
    () => (backdrop.kind === 'image' ? fightArea(backdrop.map, track, floor) : [0, 0, 1]),
    [backdrop, track, floor],
  )
  const walkedHere = useMemo(
    () => (backdrop.kind === 'walked' ? backdrop.walked.filter((p) => p.map === floor) : []),
    [backdrop, floor],
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
    const css = {
      muted: style.getPropertyValue('--muted'),
      text: style.getPropertyValue('--text'),
      bad: style.getPropertyValue('--bad'),
      warn: style.getPropertyValue('--warn'),
      accent: style.getPropertyValue('--accent'),
    }
    const draw = (now: number) => {
      ctx.setTransform(1, 0, 0, 1, 0, 0)
      ctx.clearRect(0, 0, canvas.width, canvas.height)
      ctx.setTransform(view.k, 0, 0, view.k, -view.x * view.k, -view.y * view.k)
      shown.current = view
      dots.current = []
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
        ctx.fillStyle = css.muted
        ctx.globalAlpha = 0.18
        walkedHere.forEach((p) => {
          const [x, y] = at(p)
          ctx.fillRect(x - 2, y - 2, 4, 4)
        })
        ctx.globalAlpha = 1
      }
      const dot = canvas.width / 160 / view.k // the same size on screen at any zoom
      const label = (text: string, x: number, y: number) => {
        ctx.fillStyle = css.text
        ctx.font = `${Math.round(dot * 2.6 * 10) / 10}px system-ui, sans-serif`
        ctx.textAlign = 'center'
        ctx.fillText(text, x, y - dot * 3.4)
      }
      const ring = (x: number, y: number, r: number) => {
        ctx.strokeStyle = css.text
        ctx.lineWidth = dot / 3
        ctx.beginPath()
        ctx.arc(x, y, r, 0, 2 * Math.PI)
        ctx.stroke()
      }
      overlay?.(ctx, at, dot, (id, x, y, r) => dots.current.push({ id, enemy: true, x, y, r }))
      ctx.fillStyle = css.bad
      track.deaths
        .filter((d) => d.map === floor && d.t <= now)
        .forEach((d) => {
          const [x, y] = at(d)
          ctx.fillRect(x - dot / 2, y - dot / 2, dot, dot)
        })
      Object.entries(track.mobs ?? {}).forEach(([guid, m]) => {
        const last = positionAt(m.p, now)
        if (!last || last.map !== floor || (m.died ? m.died <= now : now - last.t > MOB_GONE_MS)) return
        const [x, y] = at(last)
        dots.current.push({ id: guid, enemy: true, x, y, r: dot * 0.75 })
        ctx.fillStyle = css.bad
        ctx.strokeStyle = '#000'
        ctx.lineWidth = dot / 5
        ctx.beginPath()
        ctx.arc(x, y, dot * 0.75, 0, 2 * Math.PI)
        ctx.fill()
        ctx.stroke()
        if (guid === enemy) {
          ring(x, y, dot * 2)
          label(enemyName(track, guid), x, y)
        }
      })
      Object.entries(track.bosses ?? {}).forEach(([guid, points]) => {
        const last = positionAt(points, now)
        if (!last || last.map !== floor || now - last.t > BOSS_GONE_MS) return
        const [x, y] = at(last)
        dots.current.push({ id: guid, enemy: true, x, y, r: dot * 2.4 })
        ctx.fillStyle = '#000'
        ctx.strokeStyle = css.warn
        ctx.lineWidth = dot / 2
        ctx.beginPath()
        ctx.arc(x, y, dot * 2.4, 0, 2 * Math.PI)
        ctx.fill()
        ctx.stroke()
        if (guid === enemy) ring(x, y, dot * 3.4)
        label(last.name ?? '', x, y)
      })
      const died = new Map<string, number>() // each player's last death so far
      deaths
        .filter((d) => d.clockMS <= now)
        .forEach((d) => died.set(d.player, Math.max(died.get(d.player) ?? 0, d.clockMS)))
      const players = Object.entries(track.players).sort(([a], [b]) => Number(a === focus) - Number(b === focus))
      players.forEach(([player, points]) => {
        const colour = classColors[track.classes[player]] ?? css.accent
        const here = positionAt(points, now)
        const seen = points
          .slice(indexAfter(points, now - trailMS - 1), indexAfter(points, now))
          .filter((p) => p.map === floor)
        if (here && here.map === floor && seen.length > 0) seen.push(here)
        ctx.strokeStyle = colour
        ctx.lineWidth = dot / 2
        ctx.globalAlpha = focus && player !== focus ? 0.3 : 1
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
        ctx.globalAlpha = 1
        // Where they are: their last place even past the trail, and before their first sample where they were first
        // seen. Dead until they're seen again after their last death (a release and run back, or a battle res).
        const last = here ?? points[0]
        const dead = died.has(player) && (!last || last.t <= (died.get(player) ?? 0) + REVIVED_AFTER_MS)
        if (!last || last.map !== floor) return
        const [x, y] = at(last)
        dots.current.push({ id: player, enemy: false, x, y, r: dot * 1.2 })
        if (player === focus) {
          ring(x, y, dot * 2.2)
          label(player, x, y)
        }
        if (dead) {
          ctx.strokeStyle = css.bad
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
    }
    if (!clock) {
      draw(now)
      return
    }
    let frame = 0
    const tick = () => {
      draw(clock())
      frame = requestAnimationFrame(tick)
    }
    tick()
    return () => cancelAnimationFrame(frame)
  }, [track, backdrop, walkedHere, area, floor, now, clock, trailMS, deaths, loaded, view, focus, enemy, overlay])
  return (
    <canvas
      ref={ref}
      width={width}
      height={height}
      className={view.k > 1 ? 'map-canvas zoomed' : 'map-canvas'}
      role="img"
      aria-label="players on the map"
      onPointerDown={(e) => {
        if (e.button !== 0) return
        press.current = { x: e.clientX, y: e.clientY, dragged: false }
        e.currentTarget.setPointerCapture(e.pointerId)
      }}
      onPointerMove={drag}
      onPointerUp={release}
      onDoubleClick={() => setView(WHOLE)}
      onContextMenu={
        onPick &&
        ((e) => {
          e.preventDefault()
          onPick('')
          onPickEnemy?.('', false)
        })
      }
    />
  )
}

/** Where a log position lands on a Keystone.guru floor: its x across is the log's -y within the floor's bounds,
 * its y down the log's x (both as Keystone.guru's CoordinatesService computes them). */
function onTiles(floor: DungeonFloor, canvas: HTMLCanvasElement) {
  return (p: Yards): [number, number] => [
    ((floor.maxX + p.y) / (floor.maxX - floor.minX)) * canvas.width,
    ((floor.maxY - p.x) / (floor.maxY - floor.minY)) * canvas.height,
  ]
}

/** Where a log position lands on a raid room's whole image, as shares of its width and height: across from its maxY
 * (west, the left edge), down from its maxX (north, the top), as the game's world map draws it. */
function onRoom(map: RaidMap, p: Yards): [number, number] {
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
  return (p: Yards): [number, number] => {
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
  return (p: Yards): [number, number] => [pad + (-p.y - minX) * scale, canvas.height - pad - (p.x - minY) * scale]
}
