import type { HitRecap, KeyAbility, KeyDeath, KeyMeterRow, KeyTrack, KeyView, RaidPull } from './api'

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

/** What each demo player did (an ability's damage, healing and damage taken) and which enemies they hit. */
const demoAbilities: Record<string, [string, number, number, number][]> = {
  Pyrelord: [['Fireball', 420, 0, 0], ['Pyroblast', 300, 0, 0], ['Living Bomb', 90, 0, 0], ['Cleave', 0, 0, 40]],
  Stabbyone: [['Sinister Strike', 380, 0, 0], ['Eviscerate', 250, 0, 0], ['Kick', 20, 0, 0], ['Cleave', 0, 0, 30]],
  Moonwhisper: [['Rejuvenation', 0, 300, 0], ['Wrath', 100, 0, 0], ['Regrowth', 0, 210, 0], ['Cleave', 0, 0, 60]],
  Ironwall: [['Mortal Strike', 260, 0, 0], ['Execute', 160, 0, 0], ['Cleave', 0, 0, 180], ['Fireball', 0, 0, 90]],
  Lightbringer: [['Smite', 210, 0, 0], ['Flash Heal', 0, 190, 0], ['Power Word: Shield', 0, 120, 0]],
}
const demoEnemies = ['Fanatical Bruiser', 'Shadowy Caster', 'Boss']
const DEMO_END = 23 * MIN + 41_000

/** The preview key's track: the recaps of its deaths and each player's abilities over one stretch of the whole key,
 * shown through the same drill as a real key's. */
export const DEMO_TRACK = {
  recaps: keyDeaths.map((d) => recap(d.player, d.clockMS, d.ability)),
  abilityTimes: [DEMO_END],
  abilities: Object.fromEntries(
    Object.entries(demoAbilities).map(([p, list]) => [
      p,
      list.map(([a, d, h, t]) => ({ a, p: [[DEMO_END, d * 1e6, h * 1e6, t * 1e6]] })),
    ]),
  ),
  targetSeries: Object.fromEntries(
    Object.entries(demoAbilities).map(([p, list]) => {
      const damage = list.reduce((n, [, d]) => n + d * 1e6, 0)
      return [p, demoEnemies.map((a, i) => ({ a, p: [[DEMO_END, damage / (2 + i * 2), 0, 0]] }))]
    }),
  ),
} as unknown as KeyTrack

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

const raidAbilities: KeyAbility[] = Object.entries(demoAbilities).flatMap(([player, list]) =>
  list.map(([ability, d, h]) => ({ player, ability, damage: d * 5e5, healing: h * 5e5, casts: 20, hits: 40 })),
)

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
  abilities: raidAbilities,
  taken: Object.entries(demoAbilities).flatMap(([player, list]) =>
    list.filter(([, , , t]) => t > 0).map(([ability, , , t]) => ({ player, ability, damage: t * 5e5 })),
  ),
  targets: Object.keys(demoAbilities).flatMap((player) =>
    demoEnemies.map((target, i) => ({ player, target, damage: 2e8 / (1 + i * 2) })),
  ),
} as unknown as RaidPull

/** The recaps of the preview pull's deaths. */
export const DEMO_RAID_TRACK = {
  recaps: raidDeaths.map((d) => recap(d.player, d.clockMS, d.ability)),
} as unknown as KeyTrack
