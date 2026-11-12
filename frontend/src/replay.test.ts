import { describe, expect, it } from 'vitest'
import type { KeyTrack, RaidPull, TrackPoint } from './api'
import {
  abilitiesBetween,
  abilitiesUpTo,
  enemyName,
  floorsOf,
  indexAfter,
  lastAt,
  meterBetween,
  MIN_FLOOR_POINTS,
  positionAt,
} from './replay'

const at = (t: number, x: number, map = 1): TrackPoint => ({ t, x, y: 0, map })

describe('indexAfter', () => {
  it.each([
    ['empty', [], 5, 0],
    ['before the first point', [at(10, 0), at(20, 0)], 5, 0],
    ['exactly on a point', [at(10, 0), at(20, 0), at(30, 0)], 20, 2],
    ['between two points', [at(10, 0), at(20, 0), at(30, 0)], 25, 2],
    ['after the last point', [at(10, 0), at(20, 0)], 99, 2],
    ['duplicate times', [at(10, 0), at(20, 1), at(20, 2), at(30, 0)], 20, 3],
  ])('%s', (_, points, now, i) => {
    expect(indexAfter(points, now)).toBe(i)
  })
})

describe('lastAt', () => {
  const points = [at(10, 1), at(20, 2), at(20, 3), at(30, 4)]
  it.each([
    [5, undefined],
    [10, 1],
    [25, 3],
    [99, 4],
  ])('at %i', (now, x) => {
    expect(lastAt(points, now)?.x).toBe(x)
  })
  it('has none without points', () => {
    expect(lastAt([], 5)).toBeUndefined()
  })
})

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

describe('abilitiesBetween', () => {
  const track = {
    abilities: { Kaz: [{ a: 'Fireball', p: [[10_000, 100, 0, 0], [20_000, 50, 0, 40]] }] },
    targetSeries: { Kaz: [{ a: 'Boss', p: [[10_000, 60, 0, 0], [20_000, 40, 0, 0]] }] },
    abilityTimes: [10_000, 20_000],
    casts: { Kaz: [{ t: 1000, s: 'Fireball', e: 3000 }, { t: 14_000, s: 'Fireball', e: 16_000 }] },
  } as unknown as KeyTrack
  const pull = { abilities: [] } as unknown as RaidPull

  it('subtracts the window start', () => {
    expect(abilitiesBetween(track, pull, 'Kaz', 10_000, 20_000)).toEqual({
      abilities: [{ player: 'Kaz', ability: 'Fireball', damage: 50, healing: 0, casts: 1, hits: 0, school: undefined }],
      taken: [{ player: 'Kaz', ability: 'Fireball', damage: 40, school: undefined }],
      targets: [{ player: 'Kaz', target: 'Boss', damage: 40 }],
    })
  })
})

describe('meterBetween', () => {
  it('fills avoidable, priority, kicks and dispels', () => {
    const track = {
      classes: { Kaz: 'Mage' },
      meters: {
        Kaz: [
          { t: 0, d: 0, h: 0, tk: 0 },
          { t: 2000, d: 1000, h: 0, tk: 500, av: 300, pd: 400, k: 1, ds: 2 },
          { t: 4000, d: 3000, h: 0, tk: 900, av: 700, pd: 1000, k: 3, ds: 2 },
        ],
      },
    } as unknown as KeyTrack
    const [row] = meterBetween(track, 2000, 4000).rows
    expect([row.damage, row.damageTaken, row.avoidable, row.priorityDamage, row.interrupts, row.dispels]).toEqual([
      2000, 400, 400, 600, 2, 0,
    ])
  })
})

describe('floorsOf', () => {
  const on = (map: number, n: number) => Array.from({ length: n }, (_, i) => at(i * 1000, 0, map))
  it('drops a floor with a few stray points', () => {
    const players = { a: [...on(1, 40), ...on(0, 2)], b: on(2, MIN_FLOOR_POINTS) }
    expect(floorsOf({ players } as unknown as KeyTrack)).toEqual([1, 2])
  })
  it('keeps the only floor of a track just started', () => {
    expect(floorsOf({ players: { a: on(1, 3) } } as unknown as KeyTrack)).toEqual([1])
  })
})

describe('enemyName', () => {
  const track = {
    bosses: { boss: [{ ...at(0, 0), name: 'Boss' }] },
    mobs: { near: { p: [at(0, 5)], died: 9 }, alive: { p: [at(0, 0)] } },
    deaths: [
      { ...at(9, 50), name: 'Far Brawler' },
      { ...at(9, 6), name: 'Lost Sethrak' },
    ],
  } as unknown as KeyTrack
  it.each([
    ['a boss, from its points', 'boss', 'Boss'],
    ['a mob, from the nearest death when it died', 'near', 'Lost Sethrak'],
    ['a mob that never died', 'alive', 'Enemy'],
  ])('%s', (_, guid, name) => {
    expect(enemyName(track, guid)).toBe(name)
  })
})
