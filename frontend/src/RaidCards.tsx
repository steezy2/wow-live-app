import { Fragment, memo, type MouseEvent, type ReactNode } from 'react'
import { abilityViews, raidTargets, type AbilityRow } from './abilities'
import type { KeyMeter, PlayerCount, RaidPull } from './api'
import { classColors } from './classColors'
import { amount, clock, coverText, seconds } from './format'
import { defense, raidOffense, type Column } from './meterColumns'
import { MeterCard } from './MeterCards'
import { AbilityBars } from './PlayerFocus'
import { At, Name, type Classes } from './Seek'

/** How many raiders a mechanic's line names before it counts the rest. */
const NAMES_SHOWN = 6

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

/** One line of the mechanics card: when it (first) happened, what, who and how much, and whether it went wrong. */
interface MechanicLine {
  at?: number
  what: ReactNode
  detail: ReactNode
  bad?: boolean
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
      pull.soaks.flatMap((s): MechanicLine[] => {
        const died = s.died.length > 0 ? [{ what: `Died to ${s.ability}`, detail: names(s.died), bad: true }] : []
        if (!s.hits) {
          const summary = `${s.casts} casts, ${s.fewest}–${s.most} raiders soaked each`
          return [{ at: first('soak', s.ability), what: s.ability, detail: summary }, ...died]
        }
        return [
          ...s.hits.map((h) => ({
            at: h.clockMS,
            what: s.ability,
            detail:
              h.players.length >= pull.players.length ? (
                `everyone (${h.players.length})`
              ) : (
                <>
                  {names(h.players)} ({s.needed ? `${h.players.length} of ${s.needed}` : h.players.length})
                </>
              ),
            bad: h.players.length < s.needed,
          })),
          ...died,
        ]
      }),
    ],
    [
      'Picked up',
      (pull.pickups ?? []).map((p) =>
        p.missed?.length
          ? {
              at: p.clockMS,
              what: (
                <>
                  Didn't pick up {p.name}: {names(p.missed)}
                </>
              ),
              detail: p.players.length > 0 ? <>picked up by {names(p.players)}</> : 'nobody picked one up',
              bad: true,
            }
          : { at: p.clockMS, what: `Picked up ${p.name}`, detail: names(p.players) },
      ),
    ],
    ['Tethers broken', breakLines(pull, names)],
    [
      'Stacks',
      pull.stacks.map((s) => {
        const over = s.limit ? s.over : s.players
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
      pull.adds.flatMap((a) => [
        {
          at: first('adds', a.name),
          what: a.name,
          detail:
            `${a.killed} of ${a.spawned} killed${a.aliveEnd ? `, ${a.aliveEnd} still up at the end` : ''} · longest ` +
            clock(a.longestMS),
        },
        ...(a.spawns ?? []).map((s) => ({
          at: s.clockMS,
          what: `${a.name} spawned`,
          detail: s.cause ? (
            <>
              from {s.cause}
              {s.players.length > 0 && <> ({names(s.players)})</>}
            </>
          ) : (
            'cause not seen'
          ),
        })),
      ]),
    ],
    [
      'Damage into priority targets',
      pull.priority.map((p) => ({ what: p.target, detail: `${amount(p.damage)} damage` })),
    ],
  ]
  const shown = groups.filter(([, lines]) => lines.length > 0)
  return (
    <section className="panel mechanics">
      <h2>Mechanics</h2>
      {shown.map(([title, lines]) => (
        <div key={title} className="mechanic-group">
          <h3 className="muted small">{title}</h3>
          <ul className="list">
            {lines.map((l, i) => (
              <li key={i} className="stack">
                <span>
                  {l.at !== undefined && <At ms={l.at} onSeek={onSeek} />}{' '}
                  <b className={l.bad ? 'bad' : undefined}>{l.what}</b>
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

/** The pull's buffs in one card: what each raider lacked at the start (at 0:00) until it came, and each flask, food
 * or raid buff that came off them for a while; a time shows that moment in the replay. Hidden when there are none, or
 * for a pull saved before they were kept. */
export const BuffGaps = memo(function BuffGaps({ pull, classes, onSeek, onFocus }: CardProps) {
  const gaps = pull.buffGaps ?? []
  if (gaps.length === 0) return null
  const until = (fromMS: number, toMS: number) =>
    toMS < pull.durationMS ? `until ${clock(toMS)}` : fromMS > 0 ? 'to the end' : 'all pull'
  return (
    <section className="panel">
      <h2>Missing buffs</h2>
      <ul className="list">
        {gaps.map((g) => (
          <li key={`${g.player}-${g.buff}-${g.fromMS}`}>
            <At ms={g.fromMS} onSeek={onSeek} /> <Name player={g.player} classes={classes} onPick={onFocus} /> no{' '}
            {g.buff} <span className="muted small">{until(g.fromMS, g.toMS)}</span>
          </li>
        ))}
      </ul>
    </section>
  )
})

/** The tethers broken as lines: a break, then the raiders whose breaks came right after it. */
function breakLines(pull: RaidPull, names: (players: string[]) => ReactNode): MechanicLine[] {
  return (pull.breaks ?? []).map((b) => ({
    at: b.clockMS,
    what: `${b.aura} broken`,
    detail: (
      <>
        {names([b.player])}
        {b.effect && ` → ${b.effect} on ${b.hit}`}
        {b.then && <>; then {names(b.then)}</>}
      </>
    ),
  }))
}

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
  const targets = raidTargets(pull.targets, focus)
  const color = classColors[classes[focus]] ?? 'var(--accent)'
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
          <AbilityBars rows={rows} color={color} />
          <h3 className="muted small">{under}</h3>
          <AbilityBars rows={underRows} color={color} label={underLabel} />
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
