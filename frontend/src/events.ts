import type { BuffSpan, KeyCast, KeyDeath, KeyDispel, KeyInterrupt, RaidLane, TrackCast } from './api'

/** The lanes whose marks each name one player, first in the text ("Name · ability", or the name alone). */
const playerLanes = new Set(['deaths', 'cooldowns', 'potions', 'cc'])

/** The lanes with the deaths, defensives, potions and crowd control cut to one player's; other lanes as they are. */
export function lanesOf(lanes: RaidLane[], player: string): RaidLane[] {
  return lanes.map((l) =>
    playerLanes.has(l.kind) ? { ...l, marks: l.marks.filter((m) => m.text.split(' · ')[0] === player) } : l,
  )
}

/** What happened on a fight's clock, for the Events feed's filter. */
export const eventKinds = ['death', 'defensive', 'raid cooldown', 'potion', 'interrupt', 'wasted kick', 'cc', 'dispel'] as const
export type EventKind = (typeof eventKinds)[number]

/** One line of the Events feed: who did it (or who it happened to) and what, at a moment. */
export interface FightEvent {
  clockMS: number
  kind: EventKind
  player: string
  text: string
}

const castKind: Record<KeyCast['kind'], EventKind> = {
  defensive: 'defensive',
  healthstone: 'potion',
  'health potion': 'potion',
  'combat potion': 'potion',
  kick: 'wasted kick',
  cc: 'cc',
  'raid cooldown': 'raid cooldown',
}

/** A key's or pull's deaths, buttons pressed, interrupts and dispels as one list in time order. */
export function fightEvents(f: {
  deaths: KeyDeath[]
  interrupts: KeyInterrupt[]
  casts: KeyCast[]
  debuffs: KeyDispel[]
}): FightEvent[] {
  const events: FightEvent[] = [
    ...f.deaths.map((d) => ({
      clockMS: d.clockMS,
      kind: 'death' as const,
      player: d.player,
      text: `died to ${d.ability || 'unknown'} from ${d.killer || 'unknown'}`,
    })),
    ...f.casts.map((c) => ({
      clockMS: c.clockMS,
      kind: castKind[c.kind],
      player: c.player,
      text: c.kind === 'combat potion' && c.prepot ? 'pre-pot' : c.ability,
    })),
    ...f.interrupts.map((i) => ({
      clockMS: i.clockMS,
      kind: 'interrupt' as const,
      player: i.player,
      text: `${i.kick ? `${i.kick} → ` : ''}${i.spell} on ${i.target}`,
    })),
    ...f.debuffs.map((d) => ({
      clockMS: d.clockMS,
      kind: 'dispel' as const,
      player: d.by || d.player,
      text: d.by ? `${d.spell} off ${d.player}` : `${d.spell} on ${d.player} ran out`,
    })),
  ]
  return events.sort((a, b) => a.clockMS - b.clockMS)
}

/** A player's casts of the spells used at most once a minute through the fight (their cooldowns), a lane per spell. */
export function cooldownLanes(casts: TrackCast[], durationMS: number): RaidLane[] {
  const bySpell = new Map<string, TrackCast[]>()
  casts.filter((c) => !c.x).forEach((c) => bySpell.set(c.s, [...(bySpell.get(c.s) ?? []), c]))
  const most = Math.max(1, Math.ceil(durationMS / 60_000))
  return [...bySpell]
    .filter(([, list]) => list.length <= most)
    .sort((a, b) => a[1][0].t - b[1][0].t)
    .map(([spell, list]) => ({ name: spell, kind: 'cooldowns', marks: list.map((c) => ({ clockMS: c.t, text: spell })) }))
}

/** One buff's share of the fight it was up on a player (0 to 100), most up first; a buff cast again counts its
 * stretches together. */
export function buffUptimes(spans: BuffSpan[], durationMS: number): { id: number; name: string; pct: number }[] {
  const up = new Map<number, { name: string; ms: number }>()
  spans.forEach((s) => {
    const row = up.get(s.i) ?? { name: s.n, ms: 0 }
    row.ms += Math.max(0, Math.min(s.t, durationMS) - Math.max(s.f, 0))
    up.set(s.i, row)
  })
  return [...up]
    .map(([id, { name, ms }]) => ({ id, name, pct: durationMS > 0 ? Math.min(100, (ms / durationMS) * 100) : 0 }))
    .sort((a, b) => b.pct - a.pct || a.name.localeCompare(b.name))
}
