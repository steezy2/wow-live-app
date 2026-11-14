import type { TrackPoint } from './api'

/** Walked points further apart than this (yards) aren't one walk: a moving player is sampled about every 7 yards. */
const LINK_YARDS = 12

/** Walked points closer than this (yards) are one place. */
const SNAP_YARDS = 3

/** A walk along a line of points: its length in yards and where the unit is after some yards of it. */
export interface Path {
  length: number
  at: (yards: number) => [number, number]
}

/** The way between two track points on one map over places parties have walked, if there is one. */
export type PathFinder = (from: TrackPoint, to: TrackPoint) => Path | undefined

type Spot = { x: number; y: number; map: number }

const dist = (a: Spot, b: Spot) => Math.hypot(a.x - b.x, a.y - b.y)

function along(points: [number, number][]): Path {
  const ends = points.reduce<number[]>(
    (sum, p, i) => [...sum, i === 0 ? 0 : sum[i - 1] + Math.hypot(p[0] - points[i - 1][0], p[1] - points[i - 1][1])],
    [],
  )
  return {
    length: ends[ends.length - 1],
    at(yards) {
      const i = ends.findIndex((end) => end >= yards)
      if (i < 0) return points[points.length - 1]
      if (i === 0) return points[0]
      const share = (yards - ends[i - 1]) / (ends[i] - ends[i - 1] || 1)
      return [
        points[i - 1][0] + (points[i][0] - points[i - 1][0]) * share,
        points[i - 1][1] + (points[i][1] - points[i - 1][1]) * share,
      ]
    },
  }
}

/** A pathfinder over where parties walked (the dungeon's walked area and the key's own track): the shortest line
 * through those places, each linked to the ones within LINK_YARDS. Ends close to each other walk straight; ends with
 * nothing walked near them, or no way between, have no path. Each answer is kept, since a replay asks every frame. */
export function pathFinder(walked: TrackPoint[]): PathFinder {
  const spots: Spot[] = []
  const known = new Set<string>()
  const buckets = new Map<string, number[]>()
  const bucketKey = (map: number, x: number, y: number) => `${map}:${x}:${y}`
  walked.forEach((p) => {
    const snapped = bucketKey(p.map, Math.floor(p.x / SNAP_YARDS), Math.floor(p.y / SNAP_YARDS))
    if (known.has(snapped)) return
    known.add(snapped)
    const key = bucketKey(p.map, Math.floor(p.x / LINK_YARDS), Math.floor(p.y / LINK_YARDS))
    buckets.set(key, [...(buckets.get(key) ?? []), spots.length])
    spots.push({ x: p.x, y: p.y, map: p.map })
  })

  const near = (p: Spot) => {
    const [bx, by] = [Math.floor(p.x / LINK_YARDS), Math.floor(p.y / LINK_YARDS)]
    return [-1, 0, 1]
      .flatMap((dx) => [-1, 0, 1].flatMap((dy) => buckets.get(bucketKey(p.map, bx + dx, by + dy)) ?? []))
      .filter((i) => dist(spots[i], p) <= LINK_YARDS)
  }
  const nearest = (p: Spot) => near(p).sort((a, b) => dist(spots[a], p) - dist(spots[b], p))[0]

  // A*: the cheapest way over the spots, as their indexes.
  const search = (start: number, goal: number) => {
    const cost = new Map([[start, 0]])
    const before = new Map<number, number>()
    const done = new Set<number>()
    const open = [{ f: dist(spots[start], spots[goal]), i: start }]
    while (open.length > 0) {
      open.sort((a, b) => b.f - a.f)
      const { i } = open.pop()!
      if (done.has(i)) continue
      done.add(i)
      if (i === goal) {
        const way = [i]
        while (before.has(way[0])) way.unshift(before.get(way[0])!)
        return way
      }
      near(spots[i]).forEach((j) => {
        const c = cost.get(i)! + dist(spots[i], spots[j])
        if (c < (cost.get(j) ?? Infinity)) {
          cost.set(j, c)
          before.set(j, i)
          open.push({ f: c + dist(spots[j], spots[goal]), i: j })
        }
      })
    }
  }

  const answers = new Map<string, Path | undefined>()
  return (from, to) => {
    if (from.map !== to.map) return undefined
    const key = `${from.map}:${from.x}:${from.y}:${to.x}:${to.y}`
    if (!answers.has(key)) {
      let path: Path | undefined
      if (dist(from, to) <= LINK_YARDS) {
        path = along([[from.x, from.y], [to.x, to.y]])
      } else {
        const [start, goal] = [nearest(from), nearest(to)]
        const way = start === undefined || goal === undefined ? undefined : search(start, goal)
        const spotsOnWay = (way ?? []).map((i): [number, number] => [spots[i].x, spots[i].y])
        path = way && along([[from.x, from.y], ...spotsOnWay, [to.x, to.y]])
      }
      answers.set(key, path)
    }
    return answers.get(key)
  }
}
