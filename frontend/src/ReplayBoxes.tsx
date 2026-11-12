import { classColors } from './classColors'
import { amount } from './format'
import { BOSS_GONE_MS, enemyName, lastAt, type AbilitiesUpTo } from './replay'
import type { KeyTrack, TrackCast } from './api'

/** At most this many units in the boss box: the boss's own, then the adds to burst. */
const BOSS_BOX_UNITS = 4

/** How many of a caster's last casts the cast box lists. */
const CAST_BOX_CASTS = 5

/** How many abilities the damage taken box lists, the most damage first. */
const TAKEN_BOX_ROWS = 5

/** The boss's health at the replay's moment, over the map's top right: the units the track says to show, in its
 * order (the boss's own, then the adds to burst), seen in the last BOSS_GONE_MS, with their health as a bar. */
export function BossBox({ track, now }: { track: KeyTrack; now: number }) {
  const units = (track.health ?? [])
    .map((guid) => lastAt(track.bosses?.[guid] ?? [], now))
    .filter((p) => p !== undefined && p.hp !== undefined && now - p.t <= BOSS_GONE_MS)
    .slice(0, BOSS_BOX_UNITS)
  if (units.length === 0) return null
  return (
    <div className="map-box boss-box" aria-label="boss health">
      {units.map((p, i) => (
        <div key={i} className="boss-hp">
          <span>{p?.name}</span> <b>{p?.hp?.toFixed(1)}%</b>
          <div className="bar">
            <div className="fill" style={{ width: `${p?.hp ?? 0}%` }} />
          </div>
        </div>
      ))}
    </div>
  )
}

/** The boxes over the bottom of the replay's map: the focused player's casts at the left; at the right the damage they
 * took, over a picked enemy's casts. */
export function FocusBoxes(props: {
  track: KeyTrack
  player: string
  enemy: string
  taken: AbilitiesUpTo['taken'] | undefined
  now: number
}) {
  const { track, player, enemy, taken, now } = props
  return (
    <>
      {player && (
        <CastBox
          title={player}
          color={classColors[track.classes[player]] ?? 'var(--accent)'}
          casts={track.casts?.[player]}
          now={now}
        />
      )}
      <div className="map-stack">
        {taken && <TakenBox taken={taken} />}
        {enemy && (
          <CastBox
            title={enemyName(track, enemy)}
            color="var(--bad)"
            casts={track.mobCasts ? (track.mobCasts[enemy] ?? []) : undefined}
            now={now}
          />
        )}
      </div>
    </>
  )
}

/** A caster at the replay's moment: what they're casting (with how far along) and their last few casts, newest first,
 * under their name in its colour. Nothing for a track without casts. */
function CastBox(props: { title: string; color: string; casts: TrackCast[] | undefined; now: number }) {
  const { title, color, casts, now } = props
  if (!casts) return null
  const started = casts.filter((c) => c.t <= now)
  const current = started.filter((c) => c.e !== undefined && c.e > now).pop()
  const recent = started
    .filter((c) => c !== current)
    .slice(-CAST_BOX_CASTS)
    .reverse()
  return (
    <div className="map-box cast-box" aria-label={`${title}'s casts`}>
      <b style={{ color }}>{title}</b>
      {current && current.e !== undefined && (
        <div>
          Casting {current.s}
          <div className="bar">
            <div className="fill" style={{ width: `${(100 * (now - current.t)) / (current.e - current.t)}%` }} />
          </div>
        </div>
      )}
      {recent.map((c, i) => (
        <div key={i} className="muted">
          {c.s}
        </div>
      ))}
    </div>
  )
}

/** The focused player's damage taken up to the replay's moment: their TAKEN_BOX_ROWS abilities that took the most.
 * Nothing before they took any. */
function TakenBox({ taken }: { taken: AbilitiesUpTo['taken'] }) {
  const top = [...taken].sort((a, b) => b.damage - a.damage).slice(0, TAKEN_BOX_ROWS)
  if (top.length === 0) return null
  return (
    <div className="map-box taken-box" aria-label="damage taken">
      <b>Damage taken</b>
      {top.map((h) => (
        <div key={h.ability} className="muted">
          {h.ability} <b>{amount(h.damage)}</b>
        </div>
      ))}
    </div>
  )
}
