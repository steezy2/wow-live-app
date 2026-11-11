import { describe, expect, it } from 'vitest'
import { eventsOf, type EventSource } from './events'

const src: EventSource = {
  deaths: [
    { player: 'Tank', clockMS: 90_000, killer: 'Boss', ability: 'Smash', defensive: '', ready: ['Shield Wall'] },
  ],
  lanes: [{ name: 'Fire', kind: 'avoidable', marks: [{ clockMS: 30_000, text: 'Tank' }, { clockMS: 31_000, text: 'Dps' }] }],
  dispels: [{ player: 'Tank', spell: 'Curse', by: '', afterMS: 6_000, clockMS: 60_000 }],
  cooldowns: [
    { player: 'Tank', ability: 'Ironbark', kind: 'defensive', clockMS: 80_000 },
    { player: 'Tank', ability: 'Kick', kind: 'kick', clockMS: 85_000 },
    { player: 'Dps', ability: 'Potion', kind: 'combat potion', clockMS: 1_000 },
  ],
}

describe('eventsOf', () => {
  it("lists one player's moments in order and leaves kicks and others' out", () => {
    const events = eventsOf(src, 'Tank')
    expect(events.map((e) => e.clockMS)).toEqual([30_000, 60_000, 80_000, 90_000])
    expect(events.map((e) => e.bad)).toEqual([true, false, false, true])
    expect(events[2].text).toBe('Used Ironbark')
  })
})
