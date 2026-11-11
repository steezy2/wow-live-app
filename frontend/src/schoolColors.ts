/** The spell schools by their bit in the combat log's school field, each in the game's colour for it (physical in
 * tan, to stand apart from holy). Presentation only. */
const SCHOOLS = [
  { bit: 1, name: 'Physical', color: '#C9A77C' },
  { bit: 2, name: 'Holy', color: '#FFE680' },
  { bit: 4, name: 'Fire', color: '#FF8000' },
  { bit: 8, name: 'Nature', color: '#4DFF4D' },
  { bit: 16, name: 'Frost', color: '#80FFFF' },
  { bit: 32, name: 'Shadow', color: '#8080FF' },
  { bit: 64, name: 'Arcane', color: '#FF80FF' },
]

const of = (school = 0) => SCHOOLS.filter((s) => school & s.bit)

/** A school field's name: "Fire", or "Fire + Frost" for a spell of several; undefined without one. */
export function schoolName(school?: number): string | undefined {
  return (
    of(school)
      .map((s) => s.name)
      .join(' + ') || undefined
  )
}

/** A meter bar's colours (BarRow's) for a school field: the school's colour, with a fill blending from one to the
 * next for a spell of several, or fallback without one. */
export function schoolBar(school: number | undefined, fallback: string): { color: string; fill?: string } {
  const colors = of(school).map((s) => s.color)
  if (colors.length < 2) return { color: colors[0] ?? fallback }
  const stops = colors.map((c) => `color-mix(in srgb, ${c} 22%, transparent)`).join(', ')
  return { color: colors[0], fill: `linear-gradient(90deg, ${stops})` }
}
