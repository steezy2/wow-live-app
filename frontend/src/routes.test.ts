import { describe, expect, it } from 'vitest'
import type { RouteEnemy } from './api'
import { pullCentres } from './routes'

const enemy = (index: number, x: number, y: number): RouteEnemy => ({
  index,
  clone: 1,
  npc: 1,
  count: 1,
  pack: 0,
  x,
  y,
  map: 1,
})

describe('pullCentres', () => {
  it('puts a pull in the middle of its enemies and leaves out pulls with none on the floor', () => {
    const enemies = [enemy(1, 0, 0), enemy(2, 10, 20), enemy(3, 99, 99)]
    const pulls = [
      { color: 'ff0000', enemies: [{ index: 1, clone: 1 }, { index: 2, clone: 1 }] },
      { color: '00ff00', enemies: [{ index: 4, clone: 1 }] },
    ]
    expect(pullCentres(enemies, pulls)).toEqual([{ pull: 0, x: 5, y: 10 }])
  })
})
