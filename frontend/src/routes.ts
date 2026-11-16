import { useEffect, useState } from 'react'
import {
  fetchDungeonMap,
  fetchMobs,
  type Drawing,
  type DungeonMap,
  type DungeonMob,
  type DungeonMobs,
  type EnemyRef,
  type Patrol,
  type RouteEnemy,
  type RoutePull,
  type RouteView,
} from './api'
import { groupBy } from './format'
import { useFetched } from './useFetched'
import type { DrawTool } from './DrawTools'
import type { Yards } from './KeyMap'

/** A dungeon's map with its floors and route enemies, null while it's read or without a dungeon. */
export function useDungeonMap(mapID: number | null): DungeonMap | null {
  const { data } = useFetched(mapID === null ? null : String(mapID), (signal, key) =>
    fetchDungeonMap(Number(key), signal),
  )
  return data ?? null // never the last dungeon's while this one is read
}

/** A dungeon's mobs at a key level (null: the level of the character's newest key there), null until the dungeon's
 * first answer; a new level or character keeps the last answer until its own comes, never another dungeon's. */
export function useMobs(mapID: number | null, level: number | null, character: string): DungeonMobs | null {
  const [read, setRead] = useState<{ mapID: number; mobs: DungeonMobs } | null>(null)
  useEffect(() => {
    if (mapID === null) return
    const controller = new AbortController()
    fetchMobs(mapID, level, character, controller.signal).then((mobs) => setRead({ mapID, mobs }), () => undefined)
    return () => controller.abort()
  }, [mapID, level, character])
  return read?.mapID === mapID ? read.mobs : null
}

/** A mob's name, or its NPC ID when the reference file has no mobs. */
export const mobName = (npc: number, mob?: DungeonMob): string => mob?.name ?? `NPC ${npc}`

/** A pull's mobs grouped by name, most first ("3× Name"), and their health at the key level summed (0 without
 * mobs). */
export function pullMobs(pull: EnemyRef[], byKey: Map<string, RouteEnemy>, mobs: Map<number, DungeonMob>) {
  const its = pull.flatMap((ref) => byKey.get(enemyKey(ref)) ?? [])
  const groups = [...groupBy(its, (e) => mobName(e.npc, mobs.get(e.npc)))]
    .map(([name, of]) => ({ name, enemies: of }))
    .sort((a, b) => b.enemies.length - a.enemies.length)
  return { groups, health: its.reduce((sum, e) => sum + (mobs.get(e.npc)?.health ?? 0), 0) }
}

/** Whether the drawing tool puts a drawing on the route (Select and Erase don't). */
export const addsDrawing = (tool: DrawTool): boolean => tool !== 'select' && tool !== 'erase'

/** An enemy's key in a lookup: its Mythic Dungeon Tools enemy and clone index. */
export const enemyKey = (e: EnemyRef): string => `${e.index}:${e.clone}`

/** The patrol the enemy walks, if any. */
export const patrolOf = (enemy: EnemyRef, patrols: Patrol[]): Patrol | undefined =>
  patrols.find((p) => p.enemies.some((e) => enemyKey(e) === enemyKey(enemy)))

/** The index in `pulls` of each enemy's pull, by enemyKey. */
export const pullIndexes = (pulls: RoutePull[]): Map<string, number> =>
  new Map(pulls.flatMap((p, i) => (p.enemies ?? []).map((e) => [enemyKey(e), i] as const)))

/** Where each pull's number goes: the middle of its enemies among the given ones (those on one floor), as the pull's
 * index in `pulls` and a place; a pull with none of them is left out. */
export function pullCentres(enemies: RouteEnemy[], pulls: RoutePull[]): (Yards & { pull: number })[] {
  const pullOf = pullIndexes(pulls)
  const sums = new Map<number, { x: number; y: number; n: number }>()
  enemies.forEach((e) => {
    const pull = pullOf.get(enemyKey(e))
    if (pull === undefined) return
    const sum = sums.get(pull) ?? { x: 0, y: 0, n: 0 }
    sums.set(pull, { x: sum.x + e.x, y: sum.y + e.y, n: sum.n + 1 })
  })
  return [...sums].map(([pull, s]) => ({ pull, x: s.x / s.n, y: s.y / s.n }))
}

/** Mythic Dungeon Tools' default (Rainbow) pull colors, which it gives pulls by their place in the route. */
export const PULL_COLORS = [
  '3eff3e',
  '3eff9e',
  '3effff',
  '3e9eff',
  '3e3eff',
  '9e9eff',
  'ff3eff',
  'ff3e9e',
  'ff3e3e',
  'ff9b3e',
  'fffb3e',
  'a1ff3e',
]
/** How many edits Undo can take back. */
export const UNDO_EDITS = 20

/** A route's name, pulls and drawings as the editor changes them. */
export interface Draft {
  name: string
  pulls: Required<RoutePull>[]
  drawings: Drawing[]
}

/** The route editor: the draft, the selected pull (-1 with none), the drafts before each edit (newest last) and
 * whether the last edit was a rename, so typing a name is one edit. */
export interface Editor {
  draft: Draft
  selected: number
  undo: Draft[]
  renaming: boolean
}

/** What the editor can do: start over from a saved route (keeping the selected pull's place), select a pull, put
 * enemies in the selected pull (or take them out when it has them all), add a pull after the selected one, delete,
 * move or recolor the selected one, rename the route, add or erase a drawing, and take back the last edit. */
export type Edit =
  | { kind: 'load'; route: RouteView }
  | { kind: 'select'; pull: number }
  | { kind: 'toggle'; enemies: EnemyRef[] }
  | { kind: 'new' }
  | { kind: 'delete' }
  | { kind: 'move'; by: -1 | 1 }
  | { kind: 'recolor'; color: string }
  | { kind: 'rename'; name: string }
  | { kind: 'draw'; drawing: Drawing }
  | { kind: 'erase'; id: string }
  | { kind: 'undo' }

/** What a saved route is made of in a draft. */
type Saved = Pick<RouteView, 'name' | 'pulls' | 'drawings'>

/** A saved route as a draft. */
export const draftOf = (route: Saved): Draft => ({
  name: route.name,
  pulls: route.pulls.map((p) => ({ color: p.color, enemies: p.enemies ?? [] })),
  drawings: route.drawings ?? [],
})

/** An editor on a saved route, its first pull selected, with nothing to undo. */
export function editorOf(route: Saved): Editor {
  const draft = draftOf(route)
  return { draft, selected: draft.pulls.length ? 0 : -1, undo: [], renaming: false }
}

/** The enemies a click takes: the enemy's whole pack (Keystone.guru's, else MDT's group), or the enemy alone when it's
 * in none (pack 0). */
export const packOf = (enemy: RouteEnemy, enemies: RouteEnemy[]): RouteEnemy[] =>
  enemy.pack ? enemies.filter((e) => e.pack === enemy.pack) : [enemy]

/** What a mob's menu can do to the route: put the enemy in the selected pull or take it out, the same for its pack
 * (when it has others), and select the pull it is in (when that isn't the selected one). */
export function mobActions(enemy: RouteEnemy, s: Editor, enemies: RouteEnemy[]): { label: string; edit: Edit }[] {
  const at = Math.max(s.selected, 0)
  const has = new Set((s.draft.pulls[at]?.enemies ?? []).map(enemyKey))
  const toggle = (group: RouteEnemy[], what: string) => {
    const all = group.every((e) => has.has(enemyKey(e)))
    const label = all ? `Take ${what} out of pull ${at + 1}` : `Add ${what} to pull ${at + 1}`
    return { label, edit: { kind: 'toggle', enemies: group } as const }
  }
  const pack = packOf(enemy, enemies)
  const pull = pullIndexes(s.draft.pulls).get(enemyKey(enemy))
  return [
    toggle([enemy], 'it'),
    ...(pack.length > 1 ? [toggle(pack, 'its pack')] : []),
    ...(pull !== undefined && pull !== s.selected
      ? [{ label: `Select its pull (${pull + 1})`, edit: { kind: 'select', pull } as const }]
      : []),
  ]
}

/** Calls close on Escape or a press outside the element `inside` says holds the target; gives what stops it. */
export function dismissOn(target: EventTarget, inside: (node: EventTarget | null) => boolean, close: () => void) {
  const key = (e: Event) => (e as KeyboardEvent).key === 'Escape' && close()
  const press = (e: Event) => !inside(e.target) && close()
  target.addEventListener('keydown', key)
  target.addEventListener('pointerdown', press)
  return () => {
    target.removeEventListener('keydown', key)
    target.removeEventListener('pointerdown', press)
  }
}

/** The editor after an edit. */
export function edit(s: Editor, e: Edit): Editor {
  const { pulls } = s.draft
  const to = (changed: Partial<Draft>, selected = s.selected, renaming = false): Editor => ({
    draft: { ...s.draft, ...changed },
    selected,
    undo: renaming && s.renaming ? s.undo : [...s.undo, s.draft].slice(-UNDO_EDITS),
    renaming,
  })
  switch (e.kind) {
    case 'load': {
      const loaded = editorOf(e.route) // the same pull stays selected after a save
      return { ...loaded, selected: Math.min(Math.max(s.selected, 0), loaded.draft.pulls.length - 1) }
    }
    case 'select':
      return { ...s, selected: e.pull, renaming: false }
    case 'toggle': {
      const at = Math.max(s.selected, 0)
      const all = pulls.length ? pulls : [{ color: PULL_COLORS[0], enemies: [] }]
      const keys = new Set(e.enemies.map(enemyKey))
      const has = new Set(all[at].enemies.map(enemyKey))
      const adding = [...keys].some((k) => !has.has(k))
      const refs = e.enemies.map(({ index, clone }) => ({ index, clone }))
      return to(
        {
          pulls: all.map((p, i) => {
            const rest = p.enemies.filter((x) => !keys.has(enemyKey(x)))
            return { ...p, enemies: i === at && adding ? [...rest, ...refs] : rest }
          }),
        },
        at,
      )
    }
    case 'new': {
      const at = s.selected + 1
      const pull = { color: PULL_COLORS[at % PULL_COLORS.length], enemies: [] }
      return to({ pulls: [...pulls.slice(0, at), pull, ...pulls.slice(at)] }, at)
    }
    case 'delete':
      if (s.selected < 0) return s
      return to({ pulls: pulls.filter((_, i) => i !== s.selected) }, Math.min(s.selected, pulls.length - 2))
    case 'move': {
      const at = s.selected + e.by
      if (s.selected < 0 || at < 0 || at >= pulls.length) return s
      const moved = [...pulls]
      ;[moved[s.selected], moved[at]] = [moved[at], moved[s.selected]]
      return to({ pulls: moved }, at)
    }
    case 'recolor':
      if (s.selected < 0) return s
      return to({ pulls: pulls.map((p, i) => (i === s.selected ? { ...p, color: e.color } : p)) })
    case 'rename':
      return to({ name: e.name }, s.selected, true)
    case 'draw':
      return to({ drawings: [...s.draft.drawings, e.drawing] })
    case 'erase':
      return to({ drawings: s.draft.drawings.filter((d) => d.id !== e.id) })
    case 'undo': {
      const draft = s.undo[s.undo.length - 1]
      if (!draft) return s
      const selected = Math.min(s.selected, draft.pulls.length - 1)
      return { draft, selected, undo: s.undo.slice(0, -1), renaming: false }
    }
  }
}

/** A draft's pulls with the forces their enemies give, summed from the map's enemies while the route isn't saved (the
 * site's view is the source of truth once it is); the percents are NaN without a forces total. */
export function pullForces(pulls: Draft['pulls'], enemies: RouteEnemy[], total?: number) {
  const count = new Map(enemies.map((e) => [enemyKey(e), e.count]))
  const pct = (n: number) => (n * 100) / (total ?? NaN)
  let running = 0
  return pulls.map((p) => {
    const forces = p.enemies.reduce((sum, e) => sum + (count.get(enemyKey(e)) ?? 0), 0)
    running += forces
    return { ...p, forces, total: running, pct: pct(forces), totalPct: pct(running) }
  })
}

/** How many points round each corner of a hull. */
const HULL_ARC = 16

/** The outline of a pull's points (one floor), padded by `pad` and rounded: one point is a circle, two a capsule,
 * more their convex hull with round corners. Counter-clockwise in the points' own units, empty without points. */
export function pullHull(points: [number, number][], pad: number): [number, number][] {
  const round = points.flatMap(([x, y]) =>
    Array.from({ length: HULL_ARC }, (_, i): [number, number] => {
      const a = (2 * Math.PI * i) / HULL_ARC
      return [x + pad * Math.cos(a), y + pad * Math.sin(a)]
    }),
  )
  return convexHull(round)
}

/** The convex hull of points (Andrew's monotone chain), counter-clockwise, without collinear points. */
function convexHull(points: [number, number][]): [number, number][] {
  const sorted = [...points].sort((a, b) => a[0] - b[0] || a[1] - b[1])
  const cross = (o: number[], a: number[], b: number[]) => (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])
  const half = (pts: [number, number][]) => {
    const out: [number, number][] = []
    pts.forEach((p) => {
      while (out.length >= 2 && cross(out[out.length - 2], out[out.length - 1], p) <= 1e-9) out.pop()
      out.push(p)
    })
    return out.slice(0, -1)
  }
  return sorted.length < 3 ? sorted : [...half(sorted), ...half([...sorted].reverse())]
}

/** A point on a map, [x, y]. */
export type Point = [number, number]

/** How far point p is from the segment a to b. */
function fromSegment(p: Point, a: Point, b: Point): number {
  const [dx, dy] = [b[0] - a[0], b[1] - a[1]]
  const t = Math.max(0, Math.min(1, ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / (dx * dx + dy * dy || 1)))
  return Math.hypot(p[0] - a[0] - t * dx, p[1] - a[1] - t * dy)
}

/** A freehand line with the points that stray less than `tolerance` from it left out (Ramer-Douglas-Peucker), the
 * tolerance doubled until at most `max` points are left. Its ends always stay. */
export function simplifyLine(points: Point[], tolerance: number, max: number): Point[] {
  const keep = (from: number, to: number, out: Point[]) => {
    let [far, at] = [0, -1]
    for (let i = from + 1; i < to; i++) {
      const d = fromSegment(points[i], points[from], points[to])
      if (d > far) [far, at] = [d, i]
    }
    if (far > tolerance) {
      keep(from, at, out)
      keep(at, to, out)
    } else out.push(points[to])
  }
  if (points.length < 3) return points
  const out: Point[] = [points[0]]
  keep(0, points.length - 1, out)
  return out.length > max ? simplifyLine(points, Math.max(tolerance, 1e-6) * 2, max) : out
}

/** The drawing on a floor nearest a place, if one is within `reach` of it: a line by its nearest segment, a marker or a
 * note by its point. */
export function drawingAt(drawings: Drawing[], map: number, p: Point, reach: number): Drawing | undefined {
  const far = (d: Drawing) =>
    d.points.length === 1
      ? Math.hypot(p[0] - d.points[0][0], p[1] - d.points[0][1])
      : Math.min(...d.points.slice(1).map((b, i) => fromSegment(p, d.points[i], b)))
  return drawings
    .filter((d) => d.map === map && far(d) <= reach)
    .sort((a, b) => far(a) - far(b))[0]
}
