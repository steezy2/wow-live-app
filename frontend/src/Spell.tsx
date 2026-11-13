import { Fragment } from 'react'
import { spellTip } from './tipAttrs'

/** An ability's name; the mouse on it says what it does and its cooldown. */
export function Spell({ name }: { name: string }) {
  return (
    <span className="spell" {...spellTip(name)}>
      {name}
    </span>
  )
}

/** Abilities in a row, commas between. */
export function Spells({ names }: { names: string[] }) {
  return names.map((n, i) => (
    <Fragment key={`${i}-${n}`}>
      {i > 0 && ', '}
      <Spell name={n} />
    </Fragment>
  ))
}

/** What covered a death or a hit (coverOf), its buttons hoverable. */
export function Cover({ lead, spells }: { lead: string; spells: string[] }) {
  return (
    <>
      {lead}
      {spells.length > 0 && ' '}
      <Spells names={spells} />
    </>
  )
}
