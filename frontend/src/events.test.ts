import { describe, expect, it } from 'vitest'
import { fightEvents, lanesOf } from './events'

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
