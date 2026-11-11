import { describe, expect, it } from 'vitest'
import type { KeyTrack, TrackPoint } from './api'
import { abilitiesUpTo, positionAt } from './replay'

const at = (t: number, x: number, map = 1): TrackPoint => ({ t, x, y: 0, map })

describe('positionAt', () => {
  it.each([
    ['between two points', [at(0, 0), at(1000, 10)], 500, 5],
    ['past the last point', [at(0, 0), at(1000, 10)], 2000, 10],
    ['across a gap the unit was not seen in', [at(0, 0), at(10_000, 10)], 5000, 0],
    ['across a map change', [at(0, 0), at(1000, 10, 2)], 500, 0],
    ['across a jump', [at(0, 0), at(1000, 100)], 500, 0],
  ])('%s', (_, points, now, x) => {
    expect(positionAt(points, now)?.x).toBe(x)
  })

  it('has no position before the first point', () => {
    expect(positionAt([at(1000, 0)], 500)).toBeUndefined()
  })
})

describe('abilitiesUpTo', () => {
  const track = {
    abilities: { Kaz: [{ a: 'Fireball', p: [[10_000, 100, 0, 2], [20_000, 50, 0, 1]] }] },
  } as unknown as KeyTrack
  it('sums the stretches that ended by then, with the school from the whole pull', () => {
    const whole = [{ player: 'Kaz', ability: 'Fireball', damage: 150, healing: 0, casts: 3, hits: 9, school: 4 }]
    expect(abilitiesUpTo(track, 'Kaz', 15_000, whole)).toEqual([
      { player: 'Kaz', ability: 'Fireball', damage: 100, healing: 0, casts: 2, hits: 0, school: 4 },
    ])
  })
  it('has nothing for a track without abilities', () => {
    expect(abilitiesUpTo({} as KeyTrack, 'Kaz', 15_000, [])).toBeUndefined()
  })
})
