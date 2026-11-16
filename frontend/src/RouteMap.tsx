import { useCallback, useMemo, useRef, useState } from 'react'
import type {
  Drawing,
  DrawingLimits,
  DungeonFloor,
  DungeonMap,
  DungeonMob,
  EnemyRef,
  KeyTrack,
  RouteEnemy,
  RoutePull,
} from './api'
import { DrawTools, type DrawTool } from './DrawTools'
import { amount, counted } from './format'
import { MapCanvas, type Overlay, type Yards } from './KeyMap'
import {
  drawingAt,
  enemyKey,
  mobName,
  PULL_COLORS,
  pullCentres,
  pullHull,
  pullIndexes,
  simplifyLine,
  type Edit,
} from './routes'

/** A dungeon's map draws no party. */
const NO_TRACK: KeyTrack = { players: {}, classes: {}, deaths: [], meters: {} }
/** What an enemy in no pull is filled with. */
const UNPULLED = '#8a93a6'
/** Characters a note's line holds before it wraps. */
const NOTE_LINE = 28

type Point = [number, number]

/** A place in yards kept on a floor (whose bounds' min can be above its max): the canvas's edge for a press outside
 * it, beside a letterboxed map. */
function onFloor(f: DungeonFloor, p: Yards): Point {
  const clamp = (v: number, a: number, b: number) => Math.min(Math.max(a, b), Math.max(Math.min(a, b), v))
  return [clamp(p.x, f.minY, f.maxY), clamp(p.y, -f.maxX, -f.minX)]
}

/** A note's words in lines of about NOTE_LINE characters. */
function wrap(text: string): string[] {
  const lines: string[] = []
  text.split(/\s+/).forEach((word) => {
    const last = lines.length - 1
    if (last >= 0 && lines[last].length + word.length < NOTE_LINE) lines[last] += ` ${word}`
    else lines.push(word)
  })
  return lines
}

/** Draws a floor's drawings: lines through their points, markers as diamonds, notes as their text by a dot. */
function paint(ctx: CanvasRenderingContext2D, at: (p: Yards) => Point, dot: number, drawings: Drawing[]) {
  ctx.lineCap = 'round'
  ctx.lineJoin = 'round'
  ctx.textAlign = 'left'
  ctx.textBaseline = 'middle'
  ctx.font = `bold ${dot * 2.2}px system-ui, sans-serif`
  drawings.forEach((d) => {
    const [[x, y], ...rest] = d.points.map(([x, y]) => at({ x, y }))
    ctx.fillStyle = ctx.strokeStyle = `#${d.color}`
    if (d.kind === 'line') {
      ctx.lineWidth = dot / 2
      trace(ctx, [[x, y], ...rest], false)
      ctx.stroke()
      return
    }
    const s = d.kind === 'marker' ? dot * 1.6 : dot * 0.6
    trace(ctx, [[x, y - s], [x + s, y], [x, y + s], [x - s, y]])
    ctx.fill()
    ctx.strokeStyle = '#000'
    ctx.lineWidth = dot / 4
    ctx.stroke()
    if (d.kind !== 'note') return
    ctx.lineWidth = dot / 2
    wrap(d.text ?? '').forEach((line, i) => {
      const [tx, ty] = [x + dot * 1.4, y + i * dot * 2.6]
      ctx.strokeText(line, tx, ty)
      ctx.fillText(line, tx, ty)
    })
  })
}

/** Traces points as a path, closed unless told otherwise (a hull is, a patrol back and forth isn't). */
function trace(ctx: CanvasRenderingContext2D, points: [number, number][], closed = true) {
  ctx.beginPath()
  points.forEach(([x, y], i) => (i ? ctx.lineTo(x, y) : ctx.moveTo(x, y)))
  if (closed) ctx.closePath()
}

/** A dungeon's floors with every enemy a route can pull as a dot (a boss larger): the ones in a pull filled with its
 * color (the selected pull's ringed in white), and the pull's number where its enemies are. A switcher picks the
 * floor, a Packs button outlines the unpulled enemies of each pack, and a Patrols button (on, shown when the dungeon
 * has any) dashes each patrol's path in its color. Each pull is outlined under its dots in its color, the selected and
 * the highlighted one stronger. With onEnemy, a click on an enemy's dot gives it and whether Shift was held, and with
 * onContext a right click gives it and where in the window. With mobs (by NPC, at the key
 * level), the mouse on an enemy tells its name, health there and forces. The killed enemies (a key's) are ticked.
 * The floor's drawings go on top; with onDraw, DrawTools' tools draw lines (simplified as they end), markers and notes
 * in a pull color, or erase one, each an Edit. */
export function RouteMap(props: {
  mapID: number
  dungeon: DungeonMap | null
  pulls: RoutePull[]
  selected?: number
  highlight?: number
  onEnemy?: (enemy: RouteEnemy, shift: boolean) => void
  onContext?: (enemy: RouteEnemy, x: number, y: number) => void
  mobs?: Map<number, DungeonMob>
  level?: number
  killed?: EnemyRef[]
  drawings?: Drawing[]
  onDraw?: (edit: Edit) => void
  limits?: DrawingLimits
}) {
  const { mapID, dungeon, pulls, selected, highlight, onEnemy, onContext, mobs, level, killed } = props
  const { drawings, onDraw, limits } = props
  const [floor, setFloor] = useState<number | null>(null)
  const [packs, setPacks] = useState(false)
  const [walks, setWalks] = useState(true)
  const [tool, setTool] = useState<DrawTool>('select')
  const [color, setColor] = useState(PULL_COLORS[8])
  const [text, setText] = useState('')
  const [line, setLine] = useState<Point[] | null>(null) // the line being drawn
  const drawn = useRef<Point[]>([]) // the same, for the press's end
  const full = (drawings?.length ?? 0) >= (limits?.drawings ?? Infinity)

  const floors = useMemo(
    () => dungeon?.floors.filter((f) => dungeon.enemies.some((e) => e.map === f.uiMapID)) ?? [],
    [dungeon],
  )
  const here = floors.find((f) => f.uiMapID === floor) ?? floors[0]
  const enemies = useMemo(() => dungeon?.enemies.filter((e) => e.map === here?.uiMapID) ?? [], [dungeon, here])
  const patrols = useMemo(
    () => (walks && dungeon?.patrols.filter((p) => p.map === here?.uiMapID)) || [],
    [dungeon, here, walks],
  )
  const drawnHere = useMemo(() => drawings?.filter((d) => d.map === here?.uiMapID) ?? [], [drawings, here])
  // MapCanvas restarts its draw loop when the overlay changes, so it changes only with what it draws
  const overlay = useCallback<Overlay>(
    (ctx, at, dot, hit) => {
      const pullOf = pullIndexes(pulls)
      const outline = (group: RouteEnemy[]) => pullHull(group.map((e) => at(e)), dot * 2.5)
      const groups = (key: (e: RouteEnemy) => number | undefined) => {
        const by = new Map<number, RouteEnemy[]>()
        enemies.forEach((e) => {
          const k = key(e)
          if (k !== undefined) by.set(k, [...(by.get(k) ?? []), e])
        })
        return by
      }
      if (packs) {
        ctx.setLineDash([dot, dot])
        ctx.strokeStyle = UNPULLED
        ctx.lineWidth = dot / 4
        groups((e) => (e.pack && !pullOf.has(enemyKey(e)) ? e.pack : undefined)).forEach((group) => {
          if (group.length < 2) return
          trace(ctx, outline(group))
          ctx.stroke()
        })
        ctx.setLineDash([])
      }
      groups((e) => pullOf.get(enemyKey(e))).forEach((group, pull) => {
        trace(ctx, outline(group))
        ctx.globalAlpha = pull === highlight ? 0.4 : 0.18
        ctx.fillStyle = `#${pulls[pull].color}`
        ctx.fill()
        ctx.globalAlpha = 0.9
        ctx.strokeStyle = ctx.fillStyle
        ctx.lineWidth = pull === selected || pull === highlight ? dot / 1.5 : dot / 4
        ctx.stroke()
        ctx.globalAlpha = 1
      })
      ctx.setLineDash([dot * 1.5, dot])
      ctx.lineWidth = dot / 2.5
      patrols.forEach((p) => {
        trace(ctx, p.points.map(([x, y]) => at({ x, y })), p.loop)
        ctx.strokeStyle = `#${p.color}`
        ctx.stroke()
      })
      ctx.setLineDash([])
      ctx.lineWidth = dot / 5
      enemies.forEach((e) => {
        const pull = pullOf.get(enemyKey(e))
        const [x, y] = at(e)
        const r = e.boss ? dot * 1.4 : dot * 0.8
        hit(enemyKey(e), x, y, r)
        ctx.fillStyle = pull === undefined ? UNPULLED : `#${pulls[pull].color}`
        ctx.strokeStyle = pull !== undefined && pull === selected ? '#fff' : '#000'
        ctx.beginPath()
        ctx.arc(x, y, r, 0, 2 * Math.PI)
        ctx.fill()
        ctx.stroke()
      })
      const dead = new Set(killed?.map(enemyKey))
      ctx.strokeStyle = '#000'
      ctx.lineWidth = dot / 3
      enemies
        .filter((e) => dead.has(enemyKey(e)))
        .forEach((e) => {
          const [x, y] = at(e)
          const s = dot * 0.6
          ctx.beginPath()
          ctx.moveTo(x - s, y)
          ctx.lineTo(x - s / 3, y + s * 0.7)
          ctx.lineTo(x + s, y - s * 0.7)
          ctx.stroke()
        })
      ctx.font = `bold ${dot * 3}px system-ui, sans-serif`
      ctx.textAlign = 'center'
      ctx.textBaseline = 'middle'
      ctx.strokeStyle = '#000'
      ctx.lineWidth = dot / 2
      pullCentres(enemies, pulls).forEach((c) => {
        const [x, y] = at(c)
        ctx.strokeText(String(c.pull + 1), x, y)
        ctx.fillStyle = '#fff'
        ctx.fillText(String(c.pull + 1), x, y)
      })
      const drawing = line && here && { id: '', map: here.uiMapID, kind: 'line' as const, color, points: line }
      paint(ctx, at, dot, drawing ? [...drawnHere, drawing] : drawnHere)
    },
    [enemies, patrols, pulls, selected, highlight, killed, packs, drawnHere, line, color, here],
  )
  /** A drawing tool's press on the map: a line follows it and ends with it, a marker or a note (with its text) goes
   * where it ends, and Erase takes the drawing it lands on. */
  const stroke = (phase: 'down' | 'move' | 'up', p: Yards, reach: number) => {
    if (!here || !onDraw) return
    const at = onFloor(here, p)
    const add = (d: Omit<Drawing, 'id' | 'map' | 'color'>) => {
      const id = crypto.randomUUID().replace(/-/g, '') // 32 characters, as the site takes them
      if (!full) onDraw({ kind: 'draw', drawing: { id, map: here.uiMapID, color, ...d } })
    }
    if (tool === 'erase' && phase === 'down') {
      const hit = drawingAt(drawnHere, here.uiMapID, at, reach)
      if (hit) onDraw({ kind: 'erase', id: hit.id })
    } else if (tool === 'line' && phase !== 'up') {
      drawn.current = phase === 'down' ? [at] : [...drawn.current, at]
      setLine(drawn.current)
    } else if (tool === 'line') {
      setLine(null)
      const points = simplifyLine([...drawn.current, at], reach / 4, limits?.linePoints ?? Infinity)
      if (points.length > 1 && points.some((q) => q[0] !== points[0][0] || q[1] !== points[0][1])) {
        add({ kind: 'line', points })
      }
    } else if (tool === 'marker' && phase === 'up') add({ kind: 'marker', points: [at] })
    else if (tool === 'note' && phase === 'up' && text.trim()) {
      add({ kind: 'note', points: [at], text: text.trim() })
      setText('')
    }
  }
  const enemyOf = (key: string) => enemies.find((e) => enemyKey(e) === key)
  const pick = (key: string, shift: boolean) => {
    const enemy = enemyOf(key)
    if (enemy) onEnemy?.(enemy, shift)
  }
  const context = (key: string, x: number, y: number) => {
    const enemy = enemyOf(key)
    if (enemy) onContext?.(enemy, x, y)
  }
  const tipOf = (key: string) => {
    const enemy = enemyOf(key)
    if (!enemy) return undefined
    const mob = mobs?.get(enemy.npc)
    const head = [mobName(enemy.npc, mob), mob?.level && `level ${mob.level}`].filter(Boolean).join(' · ')
    const health = mob?.health ? `${amount(mob.health)} health at +${level}\n` : ''
    return { head, text: `${health}${counted(enemy.count, 'force')}` }
  }

  if (!here) return <p className="muted">{dungeon ? 'No map for this dungeon.' : 'Loading…'}</p>
  return (
    <section className="map">
      <div className="map-controls">
        {floors.length > 1 &&
          floors.map((f, i) => (
            <button key={f.uiMapID} aria-pressed={f === here} onClick={() => setFloor(f.uiMapID)}>
              Floor {i + 1}
            </button>
          ))}
        <button aria-pressed={packs} onClick={() => setPacks(!packs)}>
          Packs
        </button>
        {(dungeon?.patrols.length ?? 0) > 0 && (
          <button aria-pressed={walks} onClick={() => setWalks(!walks)}>
            Patrols
          </button>
        )}
        {onDraw && (
          <DrawTools
            tool={tool}
            onTool={setTool}
            color={color}
            onColor={setColor}
            text={text}
            onText={setText}
            maxText={limits?.noteText ?? Infinity}
            full={full}
          />
        )}
      </div>
      <MapCanvas
        key={here.uiMapID}
        track={NO_TRACK}
        backdrop={{ kind: 'tiles', floor: here, mapID, perSide: dungeon?.tilesPerSide ?? 0 }}
        floor={here.uiMapID}
        now={0}
        trailMS={0}
        deaths={[]}
        overlay={overlay}
        onPickEnemy={onEnemy && pick}
        onContext={onContext && context}
        tipOf={mobs && tipOf}
        onStroke={onDraw && tool !== 'select' ? stroke : undefined}
      />
      <p className="muted small">
        Scroll to zoom, drag to move, double click for the whole floor
        {onEnemy && ' · click an enemy for its pack in the selected pull (again: out), Shift+click for it alone'}
        {onContext && ' · right click an enemy for its card'} · map:{' '}
        <a href="https://keystone.guru" target="_blank" rel="noreferrer">
          Keystone.guru
        </a>
      </p>
    </section>
  )
}
