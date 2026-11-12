import { describe, expect, it } from 'vitest'
import {
  clock,
  coverText,
  importKindText,
  importProgress,
  importTotal,
  median,
  oddsText,
  periodLabel,
  progressText,
  resultText,
} from './format'

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

describe('progressText', () => {
  const pull = { start: 0, night: '', boss: '', difficulty: 'Mythic' as const, number: 1, durationMS: 0 }
  it.each([
    [true, 0, '', 'Kill'],
    [false, 70.9, 'P2', '71% P2'],
    [false, -1, 'P2', 'P2'],
    [false, 40.2, '', '40%'],
    [false, -1, '', 'Wipe'],
  ])('kill %s at %d%% %s', (kill, bossPct, phase, want) => {
    expect(progressText({ ...pull, kill, bossPct, phase })).toBe(want)
  })
})

describe('median', () => {
  it.each([
    [[], 0],
    [[5], 5],
    [[3, 1, 2], 2],
    [[4, 1, 3, 2], 2.5],
  ])('median of %j is %d', (values, want) => {
    expect(median(values)).toBe(want)
  })
})

describe('periodLabel', () => {
  const now = new Date(2026, 10, 11, 20).getTime()
  const at = (month: number, day: number, hour = 21) => new Date(2026, month, day, hour).getTime()
  const day = (month: number, d: number) =>
    new Date(2026, month, d).toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
  const week = (month: number, d: number) =>
    `Week of ${new Date(2026, month, d).toLocaleDateString([], { month: 'short', day: 'numeric' })}`
  it.each([
    ['today', at(10, 11, 0), 'Today'],
    ['yesterday late', at(10, 10, 23), 'Yesterday'],
    ['six days ago', at(10, 5), day(10, 5)],
    ['a week ago, a Wednesday', at(10, 4), week(10, 2)],
    ['a Sunday', at(10, 1), week(9, 26)],
  ])('%s', (_, ms, want) => expect(periodLabel(ms, now)).toBe(want))
})

describe('importProgress', () => {
  const status = {
    state: 'running' as const,
    character: 'Testmage-Somerealm',
    fights: 40,
    fightsDone: 11,
    fightsFailed: 1,
    pointsSpent: 300,
    pointsPerFight: 25,
    resumeAtMS: 0,
    problem: 'boom',
    imported: 11,
    listedFor: '',
  }
  it('counts the fights and what they saved, with the points and the failures', () => {
    expect(importProgress(status, 0)).toBe(
      '12 of 40 fights · 11 keys and pulls imported · about 25 points a fight · 1 failed (boom)',
    )
  })
  it('says how long it waits for the points', () => {
    expect(importProgress({ ...status, state: 'waiting', resumeAtMS: 23 * 60_000, fightsFailed: 0 }, 0)).toBe(
      "11 of 40 fights · 11 keys and pulls imported · waiting 23 min for the hour's points",
    )
  })
  it('is empty before any import', () => {
    expect(importProgress({ ...status, state: 'idle', fights: 0 }, 0)).toBe('')
  })
})

describe('import plan', () => {
  const plan = {
    character: 'Testmage-Somerealm',
    kinds: [
      { kind: 'keys' as const, fights: 86, minutes: 1846, points: 2493, default: true },
      { kind: 'raidKills' as const, fights: 26, minutes: 155, points: 837, default: true },
      { kind: 'raidWipes' as const, fights: 0, minutes: 0, points: 0, default: false },
    ],
    pointsPerHour: 3400,
    listedPoints: 120,
  }
  it('names each kind with its fights, hours and points', () => {
    expect(importKindText(plan.kinds[0])).toBe('Keys 86 · 31 h · ~2,500 points')
    expect(importKindText(plan.kinds[1])).toBe('Raid kills 26 · 2.5 h · ~840 points')
  })
  it('totals the kinds ticked, with the hours the points take', () => {
    expect(importTotal(plan, ['keys', 'raidKills'])).toBe('Import 112 fights: ~3,300 points, about 1 h')
    expect(importTotal(plan, ['raidWipes'])).toBe('Nothing to import')
  })
})

describe('coverText', () => {
  it('says used for a defensive the player pressed, had for one put on them, else what was still ready', () => {
    expect(coverText({ defensive: 'Ice Cold', used: true, ready: [] })).toBe('used Ice Cold')
    expect(coverText({ defensive: 'Pain Suppression', ready: [] })).toBe('had Pain Suppression')
    expect(coverText({ defensive: '', ready: ['Barkskin', 'Renewal'] })).toBe('still had Barkskin, Renewal')
    expect(coverText({ defensive: '' })).toBe('nothing ready')
  })
})
