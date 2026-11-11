import { describe, expect, it } from 'vitest'
import type { KeyTrack, RaidPull, TrackPoint } from './api'
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
    abilities: { Kaz: [{ a: 'Fireball', p: [[10_000, 100, 0, 0], [20_000, 50, 0, 40]] }] },
    targetSeries: { Kaz: [{ a: 'Boss', p: [[10_000, 60, 0, 0], [20_000, 40, 0, 0]] }] },
    abilityTimes: [10_000, 20_000],
    casts: { Kaz: [{ t: 1000, s: 'Fireball', e: 3000 }, { t: 14_000, s: 'Fireball', e: 16_000 }] },
  } as unknown as KeyTrack
  const pull = { abilities: [{ player: 'Kaz', ability: 'Fireball', school: 4 }] } as unknown as RaidPull

  it('shares out the stretch the moment is in, and counts casts as they finished', () => {
    expect(abilitiesUpTo(track, pull, 'Kaz', 15_000)).toEqual({
      abilities: [{ player: 'Kaz', ability: 'Fireball', damage: 125, healing: 0, casts: 1, hits: 0, school: 4 }],
      taken: [{ player: 'Kaz', ability: 'Fireball', damage: 20, school: 4 }],
      targets: [{ player: 'Kaz', target: 'Boss', damage: 80 }],
    })
  })

  it('is empty at 0:00', () => {
    expect(abilitiesUpTo(track, pull, 'Kaz', 0)).toEqual({ abilities: [], taken: [], targets: [] })
  })

  it.each([
    [10_000, 100, 0, 60],
    [20_000, 150, 40, 100],
  ])('at the end of a stretch (%i) equals the sum so far', (now, damage, taken, target) => {
    const got = abilitiesUpTo(track, pull, 'Kaz', now)
    expect([got?.abilities[0].damage, got?.taken[0]?.damage ?? 0, got?.targets?.[0].damage]).toEqual([
      damage,
      taken,
      target,
    ])
  })

  it('has no targets for a track saved before they were kept', () => {
    const old = { ...track, targetSeries: undefined } as KeyTrack
    expect(abilitiesUpTo(old, pull, 'Kaz', 15_000)?.targets).toBeUndefined()
  })

  it('has nothing for a track without abilities', () => {
    expect(abilitiesUpTo({} as KeyTrack, pull, 'Kaz', 15_000)).toBeUndefined()
  })
})
