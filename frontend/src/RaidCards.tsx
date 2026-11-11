import { Fragment, memo, type MouseEvent, type ReactNode } from 'react'
import { abilityViews, raidTargets, type AbilityRow } from './abilities'
import type { KeyMeter, PlayerCount, RaidPull } from './api'
import { classColors } from './classColors'
import { amount, clock, coverText, seconds } from './format'
import { defense, raidOffense, type Column } from './meterColumns'
import { MeterCard } from './MeterCards'
import { AbilityBars } from './PlayerFocus'

/** How many raiders a mechanic's line names before it counts the rest. */
const NAMES_SHOWN = 6

/** Each raider's class, by name. */
type Classes = Record<string, string>

/** A raider's name in their class colour; with onPick, a button that focuses them. */
export function Name({ player, classes, onPick }: { player: string; classes: Classes; onPick?: (p: string) => void }) {
  const style = { color: classColors[classes[player]] }
  return onPick ? (
    <button className="name pick" style={style} onClick={() => onPick(player)} title={`Focus ${player}`}>
      {player}
    </button>
  ) : (
    <b className="name" style={style}>
      {player}
    </b>
  )
}

/** A time on the pull's clock; a click moves the replay there. */
export function At({ ms, onSeek }: { ms: number; onSeek: (clockMS: number) => void }) {
  return (
    <button className="at muted" onClick={() => onSeek(ms)} title="Show this moment in the replay">
      {clock(ms)}
    </button>
  )
}

/** Raiders' names in class colours with a number each where they have one, the ones past NAMES_SHOWN counted. */
function Names(props: { players: (PlayerCount | string)[]; classes: Classes; onFocus: (p: string) => void }) {
  const { players, classes, onFocus } = props
  return (
    <>
      {players.slice(0, NAMES_SHOWN).map((p, i) => {
        const [player, count] = typeof p === 'string' ? [p, ''] : [p.player, ` ${p.count}`]
        return (
          <Fragment key={player}>
            {i > 0 && ', '}
            <Name player={player} classes={classes} onPick={onFocus} />
            {count}
          </Fragment>
        )
      })}
      {players.length > NAMES_SHOWN && ` and ${players.length - NAMES_SHOWN} more`}
    </>
  )
}

interface CardProps {
  pull: RaidPull
  classes: Classes
  onSeek: (clockMS: number) => void
  onFocus: (player: string) => void
}

/** The deaths before the wipe: when (a click shows it in the replay), who, to what, what covered it and the last hits;
 * then each mass kill. */
export const EarlyDeaths = memo(function EarlyDeaths({ pull, classes, onSeek, onFocus }: CardProps) {
  const early = pull.deaths.filter((d) => d.early)
  return (
    <section className="panel">
      <h2>
        Early deaths {early.length} <span className="muted small">of {pull.deaths.length}</span>
      </h2>
      <ul className="list">
        {early.map((d, i) => (
          <li key={`${d.player}-${d.clockMS}-${i}`} className="stack">
            <span>
              <At ms={d.clockMS} onSeek={onSeek} /> <Name player={d.player} classes={classes} onPick={onFocus} />{' '}
              {d.ability || 'unknown'}
              {d.killer && <span className="muted"> from {d.killer}</span>}
            </span>
            <span className="muted small">
              {coverText(d.defensive, d.ready ?? [])}
              {d.recap.length > 0 && ` · before: ${d.recap.map((h) => `${h.ability} ${amount(h.amount)}`).join(', ')}`}
            </span>
          </li>
        ))}
        {early.length === 0 && <li className="muted">None</li>}
      </ul>
      {pull.massKills.map((m) => (
        <p key={`${m.ability}-${m.clockMS}`} className="warning">
          <At ms={m.clockMS} onSeek={onSeek} /> {m.ability} killed {m.killed}
        </p>
      ))}
    </section>
  )
})

/** One line of the mechanics card: when it (first) happened, what, and who and how much. */
interface MechanicLine {
  at?: number
  what: ReactNode
  detail: ReactNode
}

/** What went wrong by kind of mechanic, a heading per kind: each line leads with when it happened (a click shows it
 * in the replay; an ability's first hit, soak or stack past the limit, from the pull's lanes), then what, then who in
 * class colours and how much. A kind with nothing to show is left out. */
export const Mechanics = memo(function Mechanics({ pull, classes, onSeek, onFocus }: CardProps) {
  const first = (kind: string, name: string) =>
    pull.lanes.find((l) => l.kind === kind && l.name === name)?.marks[0]?.clockMS
  const names = (players: (PlayerCount | string)[]) => <Names players={players} classes={classes} onFocus={onFocus} />
  const sum = (players: PlayerCount[]) => players.reduce((n, p) => n + p.count, 0)
  const groups: [string, MechanicLine[]][] = [
    [
      'Avoidable hits',
      pull.avoidable.map((a) => ({
        at: first('avoidable', a.ability),
        what: a.ability,
        detail: (
          <>
            {sum(a.players)} hits: {names(a.players)}
          </>
        ),
      })),
    ],
    [
      'Soaks',
      pull.soaks.map((s) => ({
        at: first('soak', s.ability),
        what: s.ability,
        detail: (
          <>
            {s.casts} soaks, {s.fewest}–{s.most} raiders each
            {s.died.length > 0 && <> · died: {names(s.died)}</>}
          </>
        ),
      })),
    ],
    [
      'Stacks',
      pull.stacks.map((s) => {
        const over = s.players.filter((p) => !s.limit || p.count > s.limit)
        return {
          at: first('stacks', s.ability),
          what: s.limit ? `${s.ability} past ${s.limit} stacks` : s.ability,
          detail: over.length > 0 ? names(over) : 'nobody over the limit',
        }
      }),
    ],
    [
      'Dispels (slowest)',
      pull.dispels.map((d) => ({
        at: d.clockMS,
        what: (
          <>
            {d.spell} on <Name player={d.player} classes={classes} onPick={onFocus} />
          </>
        ),
        detail: (
          <>
            {d.by ? <>dispelled by {names([d.by])}</> : 'ran out'} after {seconds(d.afterMS)}
          </>
        ),
      })),
    ],
    [
      'Casts that went off',
      pull.wentOff.map((s) => ({ at: s.clockMS, what: s.spell, detail: `cast by ${s.mob}` })),
    ],
    [
      'Adds',
      pull.adds.map((a) => ({
        at: first('adds', a.name),
        what: a.name,
        detail:
          `${a.killed} of ${a.spawned} killed${a.aliveEnd ? `, ${a.aliveEnd} still up at the end` : ''} · longest ` +
          clock(a.longestMS),
      })),
    ],
    [
      'Damage into priority targets',
      pull.priority.map((p) => ({ what: p.target, detail: `${amount(p.damage)} damage` })),
    ],
  ]
  const shown = groups.filter(([, lines]) => lines.length > 0)
  return (
    <section className="panel">
      <h2>Mechanics</h2>
      {shown.map(([title, lines]) => (
        <div key={title} className="mechanic-group">
          <h3 className="muted small">{title}</h3>
          <ul className="list">
            {lines.map((l, i) => (
              <li key={i} className="stack">
                <span>
                  {l.at !== undefined && <At ms={l.at} onSeek={onSeek} />} <b>{l.what}</b>
                </span>
                <span className="muted small">{l.detail}</span>
              </li>
            ))}
          </ul>
        </div>
      ))}
      {shown.length === 0 && <p className="muted">Nothing flagged on this boss</p>}
    </section>
  )
})

/** A card's columns without those every raider has 0 in, the first (the one it sorts by) always kept. */
function nonZero(columns: Column[], meter: KeyMeter): Column[] {
  return columns.filter((c, i) => i === 0 || meter.rows.some((r) => c.value(r) !== 0))
}

/** The whole pull's meters: damage done, and healing with damage taken; a click on a raider focuses them. Focused,
 * the first card is their damage by ability over their damage into each enemy, and the second their healing by
 * ability over what they took damage from; a right click (or Whole raid) goes back. */
export const PullMeters = memo(function PullMeters(props: {
  pull: RaidPull
  classes: Classes
  focus: string
  onFocus: (player: string) => void
}) {
  const { pull, classes, focus, onFocus } = props
  if (!focus) {
    return (
      <section className="cards">
        <MeterCard title="Damage done" meter={pull.meter} columns={nonZero(raidOffense, pull.meter)} onPick={onFocus} />
        <MeterCard
          title="Healing and damage taken"
          meter={pull.meter}
          columns={nonZero(defense(true), pull.meter)}
          onPick={onFocus}
        />
      </section>
    )
  }
  const views = abilityViews(focus, pull.abilities ?? [], pull.taken ?? [])
  const targets = raidTargets(pull, focus)
  const back = (e: MouseEvent) => {
    e.preventDefault()
    onFocus('')
  }
  const card = (title: string, rows: AbilityRow[], under: string, underLabel: string, underRows: AbilityRow[]) => (
    <div className="card focus drill" onContextMenu={back}>
      <h2>
        <button className="back" onClick={back}>
          ‹ Whole raid
        </button>{' '}
        {title} · <Name player={focus} classes={classes} />
      </h2>
      {pull.abilities ? (
        <>
          <AbilityBars rows={rows} color={classColors[classes[focus]] ?? 'var(--accent)'} />
          <h3 className="muted small">{under}</h3>
          <AbilityBars rows={underRows} color={classColors[classes[focus]] ?? 'var(--accent)'} label={underLabel} />
        </>
      ) : (
        <NoAbilities />
      )}
    </div>
  )
  return (
    <section className="cards">
      {card('Damage done', views.damage, 'Damage into each enemy', 'Enemy', targets)}
      {card('Healing and damage taken', views.healing, 'Damage taken from', 'Ability', views.taken)}
    </section>
  )
})

/** What shows in place of a raider's abilities for a pull saved without them. */
export function NoAbilities() {
  return (
    <p className="muted">
      This pull was saved before abilities were kept: <i>Read old logs</i> in Settings fills them in from the logs still
      in the game's folder.
    </p>
  )
}
