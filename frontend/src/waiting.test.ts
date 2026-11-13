import { describe, expect, it } from 'vitest'
import { WAITING_STALE_MS, raidTonight, stateLine } from './waiting'

const NOW = 1_700_000_000_000
const found = { logsDir: 'Logs', logsDirFound: true }

describe('stateLine', () => {
  it.each([
    ['unknown settings', null, null],
    ['folder missing', { ...found, logsDirFound: false, lastLogMS: 0 }, { warn: true, has: 'Folder not found' }],
    ['no log yet', { ...found, lastLogMS: 0 }, { warn: true, has: '/combatlog' }],
    ['fresh log', { ...found, lastLogMS: NOW - 4 * 60_000 }, { warn: false, has: '4 min ago' }],
    ['stale log', { ...found, lastLogMS: NOW - WAITING_STALE_MS - 60_000 }, { warn: true, has: 'Logging may be off' }],
  ])('%s', (_name, view, want) => {
    const got = stateLine(view, NOW)
    if (!want) return expect(got).toBeNull()
    expect(got?.warn).toBe(want.warn)
    expect(got?.text).toContain(want.has)
  })
})

describe('raidTonight', () => {
  const pull = (start: number, boss: string, kill: boolean, bossPct: number) =>
    ({ start, boss, difficulty: 'Mythic', kill, bossPct }) as never
  it('shows a kill, else the best wipe, per boss, for tonight only', () => {
    const nights = [
      { night: '2026-11-13', pulls: [pull(3, 'A', false, 40), pull(2, 'A', false, 20), pull(1, 'B', true, 0)] },
      { night: '2026-11-12', pulls: [pull(0, 'C', true, 0)] },
    ]
    const opened: number[] = []
    const rows = raidTonight(nights, '2026-11-13', (s) => opened.push(s))
    expect(rows.map((r) => [r.label, r.result, r.good])).toEqual([
      ['A Mythic', 'best 20% · 2 pulls', false],
      ['B Mythic', 'Kill · 1 pull', true],
    ])
    rows[0].onOpen()
    expect(opened).toEqual([2])
  })
})
