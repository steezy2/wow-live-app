/** Each class's colour as the game shows it, by the class names the API sends. Presentation only. */
export const classColors: Record<string, string> = {
  DeathKnight: '#C41E3A',
  DemonHunter: '#A330C9',
  Druid: '#FF7C0A',
  Evoker: '#33937F',
  Hunter: '#AAD372',
  Mage: '#3FC7EB',
  Monk: '#00FF98',
  Paladin: '#F48CBA',
  Priest: '#FFFFFF',
  Rogue: '#FFF468',
  Shaman: '#0070DD',
  Warlock: '#8788EE',
  Warrior: '#C69B6D',
}

const byLower = Object.fromEntries(Object.keys(classColors).map((k) => [k.toLowerCase(), k]))

/** A simc class ("mage", "deathknight") as the game names it ("Mage", "Death Knight"); "" when unknown. */
export function simClassName(simClass?: string): string {
  return byLower[simClass?.toLowerCase() ?? '']?.replace(/([a-z])([A-Z])/g, '$1 $2') ?? ''
}

/** A simc class's colour, or undefined when unknown. */
export function simClassColor(simClass?: string): string | undefined {
  return classColors[byLower[simClass?.toLowerCase() ?? '']]
}
