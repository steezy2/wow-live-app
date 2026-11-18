import type { HitRecap, KeyDeath, KeyMeterRow, KeyTrack, KeyView, RaidPull } from './api'

/** Dummy data for the overlay's preview (Settings → Overlay → Preview), so every card and option can be tried
 * without a key or a pull. The shapes are cut down to the fields the overlay reads, hence the casts. */

/** The preview key's and pull's start, which tells the overlay to read their recaps from here, not from the app, and
 * not to pop a death's recap over their meters. */
export const DEMO_START = -1

const MIN = 60_000

const row = (player: string, cls: string, dps: number, hps: number, rest: Partial<KeyMeterRow> = {}): KeyMeterRow => ({
  player,
  class: cls,
  damage: dps * 1500,
  healing: hps * 1500,
  dps,
  hps,
  interrupts: 0,
  dispels: 0,
  purges: 0,
  damageTaken: 900_000,
  avoidable: 120_000,
  priorityDamage: dps * 300,
  ...rest,
})

const mplusRows = [
  row('Pyrelord', 'Mage', 842_000, 4_000, { interrupts: 4, priorityDamage: 90_000_000 }),
  row('Stabbyone', 'Rogue', 715_000, 3_000, { interrupts: 7, avoidable: 40_000 }),
  row('Moonwhisper', 'Druid', 120_000, 610_000, { dispels: 6, avoidable: 300_000 }),
  row('Ironwall', 'Warrior', 480_000, 8_000, { interrupts: 3, damageTaken: 2_400_000 }),
  row('Lightbringer', 'Priest', 390_000, 355_000, { dispels: 3, purges: 2 }),
]

const lastPullRows = mplusRows.map((r) => ({ ...r, dps: r.dps * 1.1, hps: r.hps * 0.9, interrupts: 1 }))

const recap = (player: string, clockMS: number, killer: string): HitRecap => ({
  player,
  clockMS,
  maxHP: 1_200_000,
  events: [
    { t: clockMS - 6_000, k: 'damage', a: 'Melee', s: 'Fanatical Bruiser', n: 80_000, hp: 92 },
    { t: clockMS - 4_500, k: 'heal', a: 'Rejuvenation', s: 'Moonwhisper', n: 120_000, hp: 96 },
    { t: clockMS - 3_000, k: 'damage', a: 'Cleave', s: 'Fanatical Bruiser', n: 420_000, hp: 58 },
    { t: clockMS - 1_500, k: 'cast', a: 'Shield Wall', s: player, hp: 58 },
    { t: clockMS - 400, k: 'damage', a: killer, s: 'Fanatical Bruiser', n: 780_000 },
  ],
})

const keyDeaths: KeyDeath[] = [
  { player: 'Ironwall', clockMS: 6 * MIN + 12_000, killer: 'Bruiser', ability: 'Cleave', defensive: '', ready: [] },
  { player: 'Pyrelord', clockMS: 14 * MIN + 40_000, killer: 'Bruiser', ability: 'Fireball', defensive: '', ready: [] },
]

/** The preview Mythic+ key: 23:41 into a +14 with a 33:00 timer. */
export const DEMO_KEY = {
  dungeon: 'Preview Dungeon',
  mapID: 0,
  level: 14,
  affixes: [],
  start: DEMO_START,
  status: 'running',
  timerMS: 33 * MIN,
  deaths: keyDeaths,
  bosses: [],
  players: mplusRows.map((r) => r.player),
  pulls: [],
  reference: false,
  forces: { count: 168, total: 280, pct: 60, at: [], needsCheck: false, pace: { bestPct: 56, fasterPct: 3 } },
  lastPull: { startMS: 21 * MIN, endMS: 22 * MIN + 30_000, ongoing: false, rows: lastPullRows },
  overall: { startMS: 0, endMS: 23 * MIN + 41_000, ongoing: false, rows: mplusRows },
  buffs: [],
  clockMS: 23 * MIN + 41_000,
  penaltyMS: 10_000,
  plus2MS: 26 * MIN + 24_000,
  plus3MS: 19 * MIN + 48_000,
  chests: 2,
  silentMS: 0,
  route: {
    name: 'Preview route',
    current: null,
    next: { number: 7, pct: 4.6, totalPct: 64.6 },
    forcesPct: 100,
    missed: 0,
    behind: false,
  },
  projection: { finalMS: 29 * MIN + 10_000, lowMS: 27 * MIN, highMS: 31 * MIN, chests: 2, odds: [], runs: 3, best: [] },
} as unknown as KeyView

/** The recaps of the preview key's deaths, shown through the same drill as a real key's. */
export const DEMO_TRACK = { recaps: keyDeaths.map((d) => recap(d.player, d.clockMS, d.ability)) } as unknown as KeyTrack

const raidRows = [
  row('Pyrelord', 'Mage', 1_320_000, 3_000),
  row('Stabbyone', 'Rogue', 1_150_000, 2_000),
  row('Hexmaster', 'Warlock', 1_090_000, 2_000),
  row('Ironwall', 'Warrior', 980_000, 6_000, { interrupts: 2 }),
  row('Sunfire', 'Druid', 940_000, 5_000),
  row('Deadeye', 'Hunter', 910_000, 2_000),
  row('Moonwhisper', 'Druid', 130_000, 820_000, { dispels: 8 }),
  row('Lightbringer', 'Priest', 210_000, 760_000, { dispels: 5 }),
  row('Tidecaller', 'Shaman', 180_000, 690_000),
  row('Brewmaster', 'Monk', 160_000, 610_000),
]

const raidDeath = (player: string, clockMS: number, ability: string, early: boolean) => ({
  player,
  clockMS,
  killer: 'Boss',
  ability,
  defensive: '',
  ready: [],
  early,
})
const raidDeaths = [
  raidDeath('Hexmaster', 2 * MIN + 5_000, 'Shadow Cleave', true),
  raidDeath('Deadeye', 3 * MIN + 20_000, 'Void Eruption', true),
  raidDeath('Sunfire', 4 * MIN + 2_000, 'Rending Claws', false),
]

/** The preview raid pull: a wipe at 18% on pull 7. */
export const DEMO_RAID = {
  start: DEMO_START,
  night: '2026-11-17',
  boss: 'Preview Boss',
  difficulty: 'Heroic',
  number: 7,
  durationMS: 5 * MIN + 12_000,
  kill: false,
  bossPct: 18,
  phase: 'P2',
  end: 5 * MIN + 12_000,
  players: raidRows.map((r) => r.player),
  deaths: raidDeaths,
  meter: { startMS: 0, endMS: 5 * MIN + 12_000, ongoing: false, rows: raidRows },
  comparisons: { tonight: { number: 5, bossPct: 9, kill: false, boss: 'Preview Boss', phase: 'P3' } },
  suggestions: ['Use a defensive for Shadow Cleave', 'Soak the Void Eruption with more players'],
  soaks: [],
} as unknown as RaidPull

/** The recaps of the preview pull's deaths. */
export const DEMO_RAID_TRACK = {
  recaps: raidDeaths.map((d) => recap(d.player, d.clockMS, d.ability)),
} as unknown as KeyTrack
