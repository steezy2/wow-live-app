import type { KeyAbility, RaidPull } from './api'

/** One line of a player's breakdown: an ability (or an enemy) and its number, with how often it was cast and hit
 * where that's known. */
export interface AbilityRow {
  name: string
  value: number
  casts?: number
  hits?: number
}

/** One player's breakdowns: their damage and healing by ability with casts and hits, what they took damage from, and
 * what they cast. */
export function abilityViews(
  player: string,
  abilities: KeyAbility[],
  taken: { player: string; ability: string; damage: number }[],
) {
  const mine = abilities.filter((a) => a.player === player)
  const done = (pick: 'damage' | 'healing'): AbilityRow[] =>
    mine.filter((a) => a[pick] > 0).map((a) => ({ name: a.ability, value: a[pick], casts: a.casts, hits: a.hits }))
  return {
    damage: done('damage'),
    healing: done('healing'),
    taken: taken.filter((h) => h.player === player).map((h): AbilityRow => ({ name: h.ability, value: h.damage })),
    casts: mine.filter((a) => a.casts > 0).map((a): AbilityRow => ({ name: a.ability, value: a.casts })),
  }
}

/** A raider's damage into each enemy over a pull. */
export function raidTargets(pull: RaidPull, player: string): AbilityRow[] {
  return (pull.targets ?? []).filter((t) => t.player === player).map((t) => ({ name: t.target, value: t.damage }))
}
