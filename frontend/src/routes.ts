import type { EnemyRef, RouteEnemy, RoutePull } from './api'
import type { Yards } from './KeyMap'

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
