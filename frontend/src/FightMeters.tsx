import type { MouseEvent } from 'react'
import { abilityViews, raidTargets, type AbilityRow } from './abilities'
import type { KeyHit, KeyMeter, KeyTrack, RaidPull } from './api'
import { classColors } from './classColors'
import { amount } from './format'
import { defense, type Column } from './meterColumns'
import { BarRow, MeterCard } from './MeterCards'
import { abilitiesBetween, hasCounts, meterBetween } from './replay'
import { schoolBar, schoolName } from './schoolColors'
import { Name, type Classes } from './Seek'

/** A stretch the cards show: its title, when it runs on the fight's clock (to: the replay's moment), and its saved
 * meter, shown at the fight's end or without a track. */
export interface MeterWindow {
  title: string
  from: number
  to: number
  saved: KeyMeter
}

/** What a fight keeps of its players' abilities: a key's or a raid pull's. */
type FightAbilities = Pick<RaidPull, 'abilities' | 'taken' | 'targets'> & { avoidableHits?: KeyHit[] }

/** The middle meter cards of a key or a raid pull, following the replay: an offense and a defense card per window
 * (four for a key, the pull the replay is in and the whole key so far; two for a raid pull), a click on a player
 * focusing them. Focused, each window's cards drill in place: their damage by ability over their damage into each
 * enemy, and their healing by ability over what they took damage from (avoidable abilities marked); a right click or
 * the back button goes back. Without onFocus (a key still running) they don't drill. At the end (atEnd) or without a
 * track, the cards show the saved meters, so they match the review. */
export function FightMeters(props: {
  windows: MeterWindow[]
  track: KeyTrack | null
  atEnd: boolean
  fight: FightAbilities
  offense: Column[]
  /** The dungeon or boss has an avoidable list. */
  listed: boolean
  group: 'party' | 'raid'
  classes: Classes
  focus: string
  onFocus?: (player: string) => void
}) {
  const { windows, track, atEnd, fight, offense, listed, group, classes, focus, onFocus } = props
  const fromTrack = !atEnd && track !== null ? track : null
  if (!focus || !onFocus) {
    const full = !fromTrack || hasCounts(fromTrack)
    const cards = (columns: Column[]) =>
      windows.map((w) => {
        const m = fromTrack ? meterBetween(fromTrack, w.from, w.to) : w.saved
        return (
          <MeterCard
            key={`${columns[0].label}-${w.title}`}
            title={w.title}
            meter={m}
            columns={full ? nonZero(columns, m) : columns.slice(0, 2)}
            onPick={onFocus}
          />
        )
      })
    return (
      <section className="cards">
        {cards(offense)}
        {cards(defense(listed))}
      </section>
    )
  }
  const color = classColors[classes[focus]] ?? 'var(--accent)'
  const avoidable = new Set((fight.avoidableHits ?? []).filter((h) => h.player === focus).map((h) => h.ability))
  const back = (e: MouseEvent) => {
    e.preventDefault()
    onFocus('')
  }
  const views = windows.map((w) => {
    const whole = w.from <= 0 && !fromTrack && fight.abilities !== undefined
    const between = track ? abilitiesBetween(track, fight, focus, w.from, w.to) : undefined
    const v = abilityViews(
      focus,
      (whole ? fight.abilities : between?.abilities) ?? [],
      (whole ? fight.taken : between?.taken) ?? [],
    )
    const taken = v.taken.map((r) => ({ ...r, avoidable: avoidable.has(r.name) }))
    const targets = raidTargets((whole ? fight.targets : undefined) ?? between?.targets, focus)
    return { ...v, taken, targets, known: whole || between !== undefined }
  })
  const card = (i: number, title: string, rows: AbilityRow[], under: string, underLabel: string, more: AbilityRow[]) => (
    <div key={`${title}-${windows[i].title}`} className="card focus drill" onContextMenu={back}>
      <h2>
        <button className="back" onClick={back}>
          ‹ Whole {group}
        </button>{' '}
        {windows[i].title} · {title} · <Name player={focus} classes={classes} />
      </h2>
      {views[i].known ? (
        <>
          <AbilityBars rows={rows} color={color} />
          <h3 className="muted small">{under}</h3>
          <AbilityBars rows={more} color={color} label={underLabel} />
        </>
      ) : !fight.abilities ? (
        <NoAbilities />
      ) : (
        <p className="muted">
          {track
            ? 'The replay keeps no ability timeline for this fight: Read old logs in Settings fills it in.'
            : 'Loading the replay…'}
        </p>
      )}
    </div>
  )
  return (
    <section className="cards">
      {views.map((v, i) => card(i, 'Damage done', v.damage, 'Damage into each enemy', 'Enemy', v.targets))}
      {views.map((v, i) => card(i, 'Healing and damage taken', v.healing, 'Damage taken from', 'Ability', v.taken))}
    </section>
  )
}

/** A card's columns without those every player has 0 in, the first (the one it sorts by) always kept. */
function nonZero(columns: Column[], meter: KeyMeter): Column[] {
  return columns.filter((c, i) => i === 0 || meter.rows.some((r) => c.value(r) !== 0))
}

/** What shows in place of a player's abilities for a fight saved without them. */
export function NoAbilities() {
  return (
    <p className="muted">
      This was saved before abilities were kept: <i>Read old logs</i> in Settings fills them in from the logs still in
      the game's folder.
    </p>
  )
}

/** Abilities (or whatever label names) as bars, biggest first, each in its spell school's colour (color without one):
 * the number, its share of the total, and casts and hits where the rows have them; an avoidable one marked ⚠. Counts
 * show whole; everything else shortened. */
function AbilityBars({
  rows,
  color,
  label = 'Ability',
}: {
  rows: AbilityRow[]
  color: string
  label?: string
}) {
  const sorted = [...rows].sort((a, b) => b.value - a.value)
  const total = sorted.reduce((sum, r) => sum + r.value, 0)
  const withCasts = sorted.some((r) => r.casts !== undefined)
  const withHits = sorted.some((r) => r.hits) // none up to a replay's moment, which keeps no hits
  const columns = 2 + Number(withCasts) + Number(withHits)
  const grid = { gridTemplateColumns: `minmax(120px, 2fr) repeat(${columns}, minmax(48px, 1fr))` }
  if (sorted.length === 0) return <p className="muted">None</p>
  return (
    <div className="scroll">
      <div className="card-row card-head muted" style={grid}>
        <span>{label}</span>
        <span>Amount</span>
        <span>%</span>
        {withCasts && <span>Casts</span>}
        {withHits && <span>Hits</span>}
      </div>
      {sorted.map((r) => (
        <BarRow key={r.name} share={r.value / sorted[0].value} {...schoolBar(r.school, color)} grid={grid}>
          <span className="name" title={r.avoidable ? 'avoidable' : schoolName(r.school)}>
            {r.avoidable && <span className="warning">⚠ </span>}
            {r.name}
          </span>
          <span>{amount(r.value)}</span>
          <span>{((100 * r.value) / total).toFixed(1)}</span>
          {withCasts && <span>{r.casts || '–'}</span>}
          {withHits && <span>{r.hits || '–'}</span>}
        </BarRow>
      ))}
    </div>
  )
}
