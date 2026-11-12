import { describe, expect, it } from 'vitest'
import { lanesOf } from './events'

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
