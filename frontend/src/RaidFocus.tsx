import { abilityViews, raidTargets } from './abilities'
import type { KeyTrack, RaidPull } from './api'
import { classColors } from './classColors'
import { eventsOf } from './events'
import { amount, clock } from './format'
import { AbilityBars, AbilityTabs } from './PlayerFocus'
import { NoAbilities } from './RaidCards'
import { EventList } from './Seek'
import { meterBetween, type AbilitiesUpTo } from './replay'

/** One raider's pull up close, beside the replay: their numbers up to the replay's moment and over the whole pull,
 * what happened to them and what they pressed in order (a click shows it in the replay), and their damage, healing,
 * damage taken, casts and targets as bars up to the moment (upTo; the whole pull for a track without them). Whole
 * raid, or a right click, goes back. */
export function RaidFocus(props: {
  pull: RaidPull
  track: KeyTrack
  player: string
  upTo?: AbilitiesUpTo
  now: number
  onSeek: (clockMS: number) => void
  onBack: () => void
}) {
  const { pull, track, player, upTo, now, onSeek, onBack } = props
  const color = classColors[track.classes[player]] ?? 'var(--accent)'
  const sofar = meterBetween(track, 0, now).rows.find((r) => r.player === player)
  const whole = pull.meter.rows.find((r) => r.player === player)
  const events = eventsOf(pull, player)
  const views = abilityViews(player, upTo?.abilities ?? pull.abilities ?? [], upTo?.taken ?? pull.taken ?? [])
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
        {whole &&
          ` · whole pull: ${amount(whole.dps)} DPS · ${amount(whole.hps)} HPS · ${amount(whole.damageTaken)} taken`}
      </p>
      <EventList events={events} onSeek={onSeek} />
      {pull.abilities && !upTo?.targets && (
        <p className="muted small">
          {upTo ? `Up to ${clock(now)}; targets over the whole pull` : 'Over the whole pull'}
        </p>
      )}
      {pull.abilities ? (
        <AbilityTabs
          views={views}
          color={color}
          more={[
            {
              name: 'Targets',
              body: (
                <AbilityBars rows={raidTargets(upTo?.targets ?? pull.targets, player)} color={color} label="Enemy" />
              ),
            },
          ]}
        />
      ) : (
        <NoAbilities />
      )}
    </div>
  )
}
