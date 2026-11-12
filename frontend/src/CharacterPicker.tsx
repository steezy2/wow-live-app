import type { Character } from './api'
import { classColors } from './classColors'
import { withRealm } from './format'

/** The top bar's character picker: every character ('') or one, each named with its realm in its class colour;
 * nothing while there's only one character to pick. */
export function CharacterPicker({
  characters,
  value,
  onChange,
}: {
  characters: Character[]
  value: string
  onChange: (id: string) => void
}) {
  if (characters.length < 2) return null
  const picked = characters.find((c) => c.id === value)
  return (
    <select
      className="character-picker"
      aria-label="Character"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      style={{ color: picked && classColors[picked.class] }}
    >
      <option value="">All characters</option>
      {characters.map((c) => (
        <option key={c.id} value={c.id} style={{ color: classColors[c.class] }}>
          {withRealm(c.name, c.realm)}
        </option>
      ))}
    </select>
  )
}
