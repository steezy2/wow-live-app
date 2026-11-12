import type { KeyCast, KeyDeath, KeyDispel, RaidLane } from './api'
import { coverText, seconds } from './format'

/** Something that happened to a player, or that they did, at a time on the replay's clock. */
export interface PlayerEvent {
  clockMS: number
  text: string
  bad: boolean
}

/** What a key or a raid pull keeps about its players' moments. */
export interface EventSource {
  deaths: KeyDeath[]
  lanes: RaidLane[]
  dispels: KeyDispel[]
  /** The defensives, potions and cooldowns pressed; kicks and crowd control are left out. */
  cooldowns: KeyCast[]
}

/** The lanes whose marks each name one player, first in the text ("Name · ability", or the name alone). */
const playerLanes = new Set(['deaths', 'cooldowns', 'potions', 'cc'])

/** The lanes with the deaths, defensives, potions and crowd control cut to one player's; other lanes as they are. */
export function lanesOf(lanes: RaidLane[], player: string): RaidLane[] {
  return lanes.map((l) =>
    playerLanes.has(l.kind) ? { ...l, marks: l.marks.filter((m) => m.text.split(' · ')[0] === player) } : l,
  )
}

/** A player's key or pull in order: their deaths, the avoidable hits and stacks past the limit the lanes marked for
 * them, debuffs on them and how they came off, and the cooldowns and potions they used. */
export function eventsOf(src: EventSource, player: string): PlayerEvent[] {
  const marked = (kind: string, mine: (text: string) => boolean, say: (lane: string, text: string) => string) =>
    src.lanes
      .filter((l) => l.kind === kind)
      .flatMap((l) =>
        l.marks.filter((m) => mine(m.text)).map((m) => ({ clockMS: m.clockMS, text: say(l.name, m.text), bad: true })),
      )
  return [
    ...src.deaths
      .filter((d) => d.player === player)
      .map((d) => ({
        clockMS: d.clockMS,
        text: `Died to ${d.ability || 'unknown'} · ${coverText(d.defensive, d.ready ?? [])}`,
        bad: true,
      })),
    ...marked('avoidable', (text) => text === player, (lane) => `Hit by ${lane}`),
    ...marked('stacks', (text) => text.startsWith(`${player} `), (lane, text) => `${lane} · ${text}`),
    ...src.dispels
      .filter((d) => d.player === player)
      .map((d) => ({
        clockMS: d.clockMS,
        text: `${d.spell} · ${d.by ? `dispelled by ${d.by}` : 'ran out'} after ${seconds(d.afterMS)}`,
        bad: false,
      })),
    ...src.cooldowns
      .filter((c) => c.player === player && c.kind !== 'kick' && c.kind !== 'cc')
      .map((c) => ({ clockMS: c.clockMS, text: `${c.prepot ? 'Pre-pot' : 'Used'} ${c.ability}`, bad: false })),
  ].sort((a, b) => a.clockMS - b.clockMS)
}
