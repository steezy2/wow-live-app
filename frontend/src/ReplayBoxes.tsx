import { classColors } from './classColors'
import { amount } from './format'
import { BOSS_GONE_MS, damageIntoEnemy, enemyName, healthAt, lastAt, type AbilitiesUpTo } from './replay'
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

/** The boxes over the bottom of the replay's map: the focused player's casts over the damage they took at
 * the left, a picked enemy's casts under the damage each player has done to it at the right. */
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
      <div className="map-stack left">
        {player && (
          <CastBox
            title={player}
            color={classColors[track.classes[player]] ?? 'var(--accent)'}
            casts={track.casts?.[player]}
            hp={healthAt(track.players[player] ?? [], now)}
            now={now}
          />
        )}
        {taken && <TakenBox taken={taken} />}
      </div>
      {enemy && (
        <div className="map-stack">
          <EnemyDamageBox track={track} name={enemyName(track, enemy)} now={now} />
          <CastBox
            title={enemyName(track, enemy)}
            color="var(--bad)"
            casts={track.mobCasts ? (track.mobCasts[enemy] ?? []) : undefined}
            hp={enemyHealth(track, enemy, now)}
            now={now}
          />
        </div>
      )}
    </>
  )
}

/** A picked enemy's health (%) at the replay's moment: 0 once it died, undefined while the track has none. */
function enemyHealth(track: KeyTrack, guid: string, now: number): number | undefined {
  const mob = track.mobs?.[guid]
  if (mob?.died && mob.died <= now) return 0
  return healthAt(track.bosses?.[guid] ?? mob?.p ?? [], now)
}

/** A caster at the replay's moment: their health as a bar, what they're casting (with how far along) and their last
 * few casts, newest first, under their name in its colour. Nothing for a track without casts or health. */
function CastBox(props: {
  title: string
  color: string
  casts: TrackCast[] | undefined
  hp: number | undefined
  now: number
}) {
  const { title, color, casts, hp, now } = props
  if (!casts && hp === undefined) return null
  const started = (casts ?? []).filter((c) => c.t <= now)
  const current = started.filter((c) => c.e !== undefined && c.e > now).pop()
  const recent = started
    .filter((c) => c !== current)
    .slice(-CAST_BOX_CASTS)
    .reverse()
  return (
    <div className="map-box cast-box" aria-label={`${title}'s casts`}>
      <b style={{ color }}>{title}</b>
      {hp !== undefined && (
        <div className="hp-bar" aria-label={`${title}'s health`}>
          <b>{hp.toFixed(0)}%</b>
          <div className="bar">
            <div className="fill" style={{ width: `${hp}%` }} />
          </div>
        </div>
      )}
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

/** What each player has done to the picked enemy (every enemy of its name) up to the replay's moment, the
 * TAKEN_BOX_ROWS who did the most, in their class's colour. Nothing before anyone hit it. */
function EnemyDamageBox({ track, name, now }: { track: KeyTrack; name: string; now: number }) {
  const top = damageIntoEnemy(track, name, now).slice(0, TAKEN_BOX_ROWS)
  if (top.length === 0) return null
  return (
    <div className="map-box taken-box" aria-label={`damage taken by ${name}`}>
      <b>{name} took</b>
      {top.map((p) => (
        <div key={p.player}>
          <span style={{ color: classColors[track.classes[p.player]] }}>{p.player}</span> <b>{amount(p.damage)}</b>
        </div>
      ))}
    </div>
  )
}
