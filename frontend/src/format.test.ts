import { describe, expect, it } from 'vitest'
import { clock, oddsText, resultText } from './format'

describe('clock', () => {
  it('shows minutes and seconds, hours past an hour, and a sign when negative', () => {
    expect(clock(1_686_807)).toBe('28:06')
    expect(clock(3_909_000)).toBe('1:05:09')
    expect(clock(-12_400)).toBe('-0:12')
    expect(clock(0)).toBe('0:00')
  })
})

describe('oddsText', () => {
  it('lists the results runs point at, best first', () => {
    expect(oddsText([0.1, 0.55, 0.35, 0])).toBe('+2 35% · +1 55% · depleted 10%')
  })
})

describe('resultText', () => {
  it.each([
    ['timed', 2, 1_312_271, 1_920_000, 'Timed +2 · 21:52 / 32:00 · 10:07 under'],
    ['depleted', 0, 2_050_000, 1_920_000, 'Depleted · 34:10 / 32:00 · 2:10 over'],
    ['abandoned', 3, 723_000, 1_920_000, 'Abandoned at 12:03'],
    ['timed', -1, 1_312_271, 0, 'Timed · 21:52'],
  ])('%s with %i chests', (status, chests, clockMS, timerMS, want) => {
    expect(resultText(status, chests, clockMS, timerMS)).toBe(want)
  })
})
