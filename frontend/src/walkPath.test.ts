import { describe, expect, it } from 'vitest'
import type { TrackPoint } from './api'
import { pathFinder } from './walkPath'

const pt = (x: number, y: number, map = 1): TrackPoint => ({ t: 0, x, y, map })

// An L-shaped corridor: along x to 60, then up y to 60 (a wall fills the corner's inside).
const corridor = [
  ...Array.from({ length: 13 }, (_, i) => pt(i * 5, 0)),
  ...Array.from({ length: 12 }, (_, i) => pt(60, (i + 1) * 5)),
]

describe('pathFinder', () => {
  it('follows the walked corridor around a corner instead of cutting through the wall', () => {
    const path = pathFinder(corridor)(pt(0, 0), pt(60, 60))
    expect(path?.length).toBeGreaterThan(110)
    expect(path?.length).toBeLessThan(125) // a straight line is 85
    const [x, y] = path!.at(60)
    expect(Math.hypot(x - 60, y)).toBeLessThan(6) // a straight line would be at (21, 21)
  })

  it('walks straight when the two ends are near each other', () => {
    expect(pathFinder([])(pt(0, 0), pt(6, 8))?.length).toBe(10)
  })

  it.each([
    ['an end is far from anything walked', pt(500, 500)],
    ['the ends are on another map', pt(60, 60, 2)],
  ])('has no path when %s', (_, to) => {
    expect(pathFinder(corridor)(pt(0, 0), to)).toBeUndefined()
  })

  it('has no path between walked areas that do not touch', () => {
    const apart = [...corridor, pt(200, 200), pt(205, 200)]
    expect(pathFinder(apart)(pt(0, 0), pt(205, 200))).toBeUndefined()
  })

  it('gives the ends at 0 and past the length', () => {
    const path = pathFinder(corridor)(pt(0, 0), pt(60, 60))!
    expect(path.at(0)).toEqual([0, 0])
    expect(path.at(path.length + 50)).toEqual([60, 60])
  })
})
