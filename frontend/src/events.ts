import type { RaidLane } from './api'

/** The lanes whose marks each name one player, first in the text ("Name · ability", or the name alone). */
const playerLanes = new Set(['deaths', 'cooldowns', 'potions', 'cc'])

/** The lanes with the deaths, defensives, potions and crowd control cut to one player's; other lanes as they are. */
export function lanesOf(lanes: RaidLane[], player: string): RaidLane[] {
  return lanes.map((l) =>
    playerLanes.has(l.kind) ? { ...l, marks: l.marks.filter((m) => m.text.split(' · ')[0] === player) } : l,
  )
}
