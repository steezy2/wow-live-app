import { describe, expect, it } from 'vitest'
import { buffUptimes, cooldownLanes, fightEvents, lanesOf } from './events'

describe('lanesOf', () => {
  it("cuts the player lanes to one player's marks and leaves the rest", () => {
    const lanes = lanesOf(
      [
        {
          name: 'Deaths',
          kind: 'deaths',
          marks: [
            { clockMS: 1, text: 'Tank · Smash' },
            { clockMS: 2, text: 'Tanky · Smash' },
          ],
        },
        { name: 'Potions', kind: 'potions', marks: [{ clockMS: 3, text: 'Tank' }, { clockMS: 4, text: 'Dps' }] },
        { name: 'Bosses', kind: 'bosses', marks: [{ clockMS: 5, text: 'Boss' }] },
      ],
      'Tank',
    )
    expect(lanes.map((l) => l.marks.map((m) => m.clockMS))).toEqual([[1], [3], [5]])
  })
})

describe('fightEvents', () => {
  it('merges deaths, casts, interrupts and dispels in time order with a kind each', () => {
    const events = fightEvents({
      deaths: [{ player: 'Tank', clockMS: 50, killer: 'Boss', ability: 'Smash', defensive: '', ready: [] }],
      casts: [
        { player: 'Dps', ability: 'Potion of Power', kind: 'combat potion', clockMS: 0, prepot: true },
        { player: 'Healer', ability: 'Barrier', kind: 'raid cooldown', clockMS: 40 },
      ],
      interrupts: [{ player: 'Dps', kick: 'Pummel', target: 'Mob', spell: 'Bolt', clockMS: 10 }],
      debuffs: [{ player: 'Tank', spell: 'Curse', by: '', afterMS: 4000, clockMS: 20 }],
    })
    expect(events.map((e) => [e.clockMS, e.kind, e.player, e.text])).toEqual([
      [0, 'potion', 'Dps', 'pre-pot'],
      [10, 'interrupt', 'Dps', 'Pummel → Bolt on Mob'],
      [20, 'dispel', 'Tank', 'Curse on Tank ran out'],
      [40, 'raid cooldown', 'Healer', 'Barrier'],
      [50, 'death', 'Tank', 'died to Smash from Boss'],
    ])
  })
})

describe('cooldownLanes', () => {
  it('keeps the spells cast at most once a minute and not the ones cut short', () => {
    const casts = [
      { t: 1000, s: 'Combustion' },
      { t: 61_000, s: 'Combustion' },
      ...[1, 2, 3, 4, 5].map((n) => ({ t: n * 1000, s: 'Fireball' })),
      { t: 5000, s: 'Meteor', x: true },
    ]
    const lanes = cooldownLanes(casts, 90_000)
    expect(lanes.map((l) => [l.name, l.marks.map((m) => m.clockMS)])).toEqual([['Combustion', [1000, 61_000]]])
  })
})

describe('buffUptimes', () => {
  it('adds a buff cast again, stops at the fight end and lists the most up first', () => {
    const spans = [
      { i: 1, n: 'Haste', f: 0, t: 25_000 },
      { i: 1, n: 'Haste', f: 50_000, t: 75_000 },
      { i: 2, n: 'Shield', f: 90_000, t: 150_000 },
    ]
    expect(buffUptimes(spans, 100_000)).toEqual([
      { id: 1, name: 'Haste', pct: 50 },
      { id: 2, name: 'Shield', pct: 10 },
    ])
  })
})
