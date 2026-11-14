import { useEffect, useState } from 'react'
import { fetchDungeonMap, type DungeonMap, type EnemyRef, type RouteEnemy, type RoutePull, type RouteView } from './api'
import type { Yards } from './KeyMap'

/** A dungeon's map with its floors and route enemies, null while it's read or without a dungeon. */
export function useDungeonMap(mapID: number | null): DungeonMap | null {
  const [read, setRead] = useState<{ mapID: number; map: DungeonMap } | null>(null)
  useEffect(() => {
    if (mapID === null) return
    const controller = new AbortController()
    fetchDungeonMap(mapID, controller.signal).then((map) => setRead({ mapID, map }), () => undefined)
    return () => controller.abort()
  }, [mapID])
  return read?.mapID === mapID ? read.map : null // never the last dungeon's while this one is read
}

/** An enemy's key in a lookup: its Mythic Dungeon Tools enemy and clone index. */
export const enemyKey = (e: EnemyRef): string => `${e.index}:${e.clone}`

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

/** A route's name and pulls as the editor changes them. */
export interface Draft {
  name: string
  pulls: Required<RoutePull>[]
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
 * enemies in the selected pull (or take them out when it has them all), add a pull after the selected one, delete or
 * move the selected one, rename the route, and take back the last edit. */
export type Edit =
  | { kind: 'load'; route: RouteView }
  | { kind: 'select'; pull: number }
  | { kind: 'toggle'; enemies: EnemyRef[] }
  | { kind: 'new' }
  | { kind: 'delete' }
  | { kind: 'move'; by: -1 | 1 }
  | { kind: 'rename'; name: string }
  | { kind: 'undo' }

/** A saved route as a draft. */
export const draftOf = (route: Pick<RouteView, 'name' | 'pulls'>): Draft => ({
  name: route.name,
  pulls: route.pulls.map((p) => ({ color: p.color, enemies: p.enemies ?? [] })),
})

/** An editor on a saved route, its first pull selected, with nothing to undo. */
export function editorOf(route: Pick<RouteView, 'name' | 'pulls'>): Editor {
  const draft = draftOf(route)
  return { draft, selected: draft.pulls.length ? 0 : -1, undo: [], renaming: false }
}

/** The enemies a Shift+click takes: the enemy's whole pack, or the enemy alone when it's in none (pack 0). */
export const packOf = (enemy: RouteEnemy, enemies: RouteEnemy[]): RouteEnemy[] =>
  enemy.pack ? enemies.filter((e) => e.pack === enemy.pack) : [enemy]

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
    case 'rename':
      return to({ name: e.name }, s.selected, true)
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
