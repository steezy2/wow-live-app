import { abilityViews, raidTargets } from './abilities'
import type { KeyTrack, RaidPull } from './api'
import { classColors } from './classColors'
import { amount, clock, coverText, seconds } from './format'
import { AbilityBars, AbilityTabs } from './PlayerFocus'
import { At, NoAbilities } from './RaidCards'
import { abilitiesUpTo, meterBetween } from './replay'

/** Something that happened to a raider, or that they did, at a time in the pull. */
interface FocusEvent {
  clockMS: number
  text: string
  bad: boolean
}

/** A raider's pull in order: their deaths, the avoidable hits and stacks past the limit the lanes marked for them,
 * debuffs on them and how they came off, and the raid cooldowns and potions they used. */
function eventsOf(pull: RaidPull, player: string): FocusEvent[] {
  const marked = (kind: string, mine: (text: string) => boolean, say: (lane: string, text: string) => string) =>
    pull.lanes
      .filter((l) => l.kind === kind)
      .flatMap((l) =>
        l.marks.filter((m) => mine(m.text)).map((m) => ({ clockMS: m.clockMS, text: say(l.name, m.text), bad: true })),
      )
  return [
    ...pull.deaths
      .filter((d) => d.player === player)
      .map((d) => ({
        clockMS: d.clockMS,
        text: `Died to ${d.ability || 'unknown'} · ${coverText(d.defensive, d.ready ?? [])}`,
        bad: true,
      })),
    ...marked('avoidable', (text) => text === player, (lane) => `Hit by ${lane}`),
    ...marked('stacks', (text) => text.startsWith(`${player} `), (lane, text) => `${lane} · ${text}`),
    ...pull.dispels
      .filter((d) => d.player === player)
      .map((d) => ({
        clockMS: d.clockMS,
        text: `${d.spell} · ${d.by ? `dispelled by ${d.by}` : 'ran out'} after ${seconds(d.afterMS)}`,
        bad: false,
      })),
    ...pull.cooldowns
      .filter((c) => c.player === player)
      .map((c) => ({ clockMS: c.clockMS, text: `${c.prepot ? 'Pre-pot' : 'Used'} ${c.ability}`, bad: false })),
  ].sort((a, b) => a.clockMS - b.clockMS)
}

/** One raider's pull up close, beside the replay: their numbers up to the replay's moment and over the whole pull,
 * what happened to them and what they pressed in order (a click shows it in the replay), and their damage, healing,
 * damage taken, casts and targets as bars. Whole raid, or a right click, goes back. */
export function RaidFocus(props: {
  pull: RaidPull
  track: KeyTrack
  player: string
  now: number
  onSeek: (clockMS: number) => void
  onBack: () => void
}) {
  const { pull, track, player, now, onSeek, onBack } = props
  const color = classColors[track.classes[player]] ?? 'var(--accent)'
  const sofar = meterBetween(track, 0, now).rows.find((r) => r.player === player)
  const whole = pull.meter.rows.find((r) => r.player === player)
  const events = eventsOf(pull, player)
  const upTo = abilitiesUpTo(track, player, now, pull.abilities ?? [])
  const views = abilityViews(player, upTo ?? pull.abilities ?? [], pull.taken ?? [])
  return (
    <div
      className="card focus raid-focus"
      onContextMenu={(e) => {
        e.preventDefault()
        onBack()
      }}
    >
      <h2>
        <span style={{ color }}>{player}</span>{' '}
        <button className="back" onClick={onBack}>
          Whole raid
        </button>
      </h2>
      <p className="muted small">
        {sofar && `At ${clock(now)}: ${amount(sofar.dps)} DPS · ${amount(sofar.hps)} HPS`}
        {whole && ` · whole pull: ${amount(whole.dps)} DPS · ${amount(whole.hps)} HPS · ${amount(whole.damageTaken)} taken`}
      </p>
      {events.length > 0 && (
        <ul className="list events">
          {events.map((e, i) => (
            <li key={i}>
              <span>
                <At ms={e.clockMS} onSeek={onSeek} /> <span className={e.bad ? 'bad' : ''}>{e.text}</span>
              </span>
            </li>
          ))}
        </ul>
      )}
      {pull.abilities && (
        <p className="muted small">
          {upTo
            ? `Damage, healing and casts up to ${clock(now)}; damage taken and targets over the whole pull`
            : 'Over the whole pull'}
        </p>
      )}
      {pull.abilities ? (
        <AbilityTabs
          views={views}
          color={color}
          more={[
            { name: 'Targets', body: <AbilityBars rows={raidTargets(pull, player)} color={color} label="Enemy" /> },
          ]}
        />
      ) : (
        <NoAbilities />
      )}
    </div>
  )
}
