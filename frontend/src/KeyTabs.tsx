import type { ReactNode } from 'react'
import type { KeyBigHit, KeyCast, KeyHit, KeyTrack, KeyView, RaidLane } from './api'
import { coverText } from './format'
import type { PlayerEvent } from './events'
import { RecapRow, recapOf } from './HitRecap'
import { Avoidable, BigHits, KickLedger, SpikeLedger } from './KeyLedgers'
import { EventList, When, Who, type Seeker } from './Seek'

/** What a fight's tabs read: a key's view, or a raid pull's lists; avoidableHits missing in pulls saved before Nov
 * 11. */
export type FightData = Pick<
  KeyView,
  'deaths' | 'interrupts' | 'casts' | 'stops' | 'wastedKicks' | 'spikes' | 'debuffs' | 'reference'
> & { avoidableHits?: KeyHit[]; bigHits?: KeyBigHit[]; lanes: RaidLane[] }

/** One player's moments for their tab: the focused player, else the log's writer; none for a raid without a focus. */
export interface Moments {
  player?: string
  events: PlayerEvent[]
}

/** A tab: its name and what it shows. */
export interface FightTab {
  name: string
  body: ReactNode
}

/** The tabs a key and a raid pull share, in order: Deaths (named deathsName), Interrupts, Must-stops, Spikes, Big
 * hits, Avoidable, Defensives and potions, CC, then (with moments, once the fight is over) one player's moments; group
 * names the cooldowns that cover a spike. With seek, a row's time moves the replay and a name focuses the player. With
 * the fight's track, a death or big hit opens its recap. */
export function fightTabs(
  data: FightData,
  deathsName: string,
  group: 'party' | 'raid',
  seek?: Seeker,
  moments?: Moments,
  track?: KeyTrack | null,
): FightTab[] {
  const cc = data.casts.filter((c) => c.kind === 'cc')
  const pressed = data.casts.filter((c) => c.kind !== 'cc')
  return [
    {
      name: deathsName,
      body: (
        <ul className="list scroll">
          {data.deaths.map((d, i) => (
            <RecapRow key={`${d.player}-${d.clockMS}-${i}`} recap={recapOf(track, d.player, d.clockMS)} ready={d.ready} died>
              <span>
                <Who player={d.player} seek={seek} /> {d.ability || 'unknown'}{' '}
                <span className="muted">from {d.killer || 'unknown'}</span>
              </span>
              <span className="muted">
                <When ms={d.clockMS} seek={seek} /> · {coverText(d)}
              </span>
            </RecapRow>
          ))}
          {data.deaths.length === 0 && <li className="muted">None</li>}
        </ul>
      ),
    },
    {
      name: `Interrupts ${data.interrupts.length}`,
      body: (
        <>
          <p className="muted">
            Every interrupt, must-stop or not:{' '}
            {countBy(data.interrupts.map((i) => `${i.player} (${i.kick || 'kick'})`))}
          </p>
          <ul className="list scroll">
            {[...data.interrupts].reverse().map((i, n) => (
              <li key={n}>
                <span>
                  <Who player={i.player} seek={seek} /> {i.kick && `${i.kick} → `}
                  {i.spell} <span className="muted">on {i.target}</span>
                </span>
                <span className="muted">
                  <When ms={i.clockMS} seek={seek} />
                </span>
              </li>
            ))}
          </ul>
        </>
      ),
    },
    {
      name: `Must-stops ${data.stops.filter((s) => s.outcome !== 'went off').length}/${data.stops.length}`,
      body: <KickLedger data={data} seek={seek} />,
    },
    { name: `Spikes ${data.spikes.length}`, body: <SpikeLedger data={data} group={group} seek={seek} /> },
    { name: `Big hits ${data.bigHits?.length ?? 0}`, body: <BigHits data={data} seek={seek} track={track} /> },
    {
      name: `Avoidable ${new Set(data.avoidableHits?.map((h) => h.ability)).size}`,
      body: <Avoidable data={data} seek={seek} />,
    },
    {
      name: 'Defensives and potions',
      body: (
        <ul className="list scroll">
          {byPlayer(pressed).map(([player, casts]) => (
            <li key={player} className="stack">
              <b>{player}</b>
              <span className="muted">{countBy(casts.map(castName))}</span>
            </li>
          ))}
          {pressed.length === 0 && <li className="muted">None</li>}
        </ul>
      ),
    },
    {
      name: `CC ${cc.length}`,
      body: (
        <ul className="list scroll">
          {byPlayer(cc).map(([player, casts]) => (
            <li key={player} className="stack">
              <span>
                <b>{player}</b> {casts.length}
              </span>
              <span className="muted">{countBy(casts.map((c) => c.ability))}</span>
            </li>
          ))}
          {cc.length === 0 && <li className="muted">None ([cc] in mplus.toml)</li>}
        </ul>
      ),
    },
    ...(moments && seek
      ? [
          {
            name: moments.player ?? 'Moments',
            body: moments.player ? (
              <EventList events={moments.events} onSeek={seek.onSeek} />
            ) : (
              <p className="muted">Pick a {group === 'raid' ? 'raider' : 'player'} to see their moments</p>
            ),
          },
        ]
      : []),
  ]
}

/** How a pressed button is counted: a defensive by its name, a raid cooldown by its name marked as one, a potion drunk
 * before the pull as a pre-pot, the rest by kind. */
function castName(c: KeyCast): string {
  if (c.kind === 'defensive') return c.ability
  if (c.kind === 'raid cooldown') return `${c.ability} (raid cooldown)`
  return c.prepot ? 'pre-pot' : c.kind
}

/** "Testshaman 20 · Testpriest 19": how often each value comes up, most first. */
function countBy(values: string[]): string {
  const counts = new Map<string, number>()
  values.forEach((v) => counts.set(v, (counts.get(v) ?? 0) + 1))
  return [...counts]
    .sort((a, b) => b[1] - a[1])
    .map(([v, n]) => `${v} ${n}`)
    .join(' · ')
}

/** Casts grouped by player, in the order players first used something. */
function byPlayer(casts: KeyCast[]): [string, KeyCast[]][] {
  const groups = new Map<string, KeyCast[]>()
  casts.forEach((c) => groups.set(c.player, [...(groups.get(c.player) ?? []), c]))
  return [...groups]
}
