import { type ReactNode, useState } from 'react'
import type { KeyBigHit, KeyCast, KeyDeath, KeyHit, KeyTrack, KeyView, RaidLane } from './api'
import { coverText } from './format'
import { RecapRow } from './HitRecap'
import { Avoidable, BigHits, KickLedger, SpikeLedger } from './KeyLedgers'
import { recapOf } from './replay'
import { When, Who, type Seeker } from './Seek'

/** What a fight's tabs read: a key's view, or a raid pull's lists; avoidableHits missing in pulls saved before Nov
 * 11. */
export type FightData = Pick<
  KeyView,
  'deaths' | 'interrupts' | 'casts' | 'stops' | 'wastedKicks' | 'spikes' | 'debuffs' | 'reference'
> & { avoidableHits?: KeyHit[]; bigHits?: KeyBigHit[]; lanes: RaidLane[] }

/** A tab: its name and what it shows. */
export interface FightTab {
  name: string
  body: ReactNode
}

/** The tabs a key and a raid pull share, in order: Deaths (named deathsName), Interrupts, Must-stops, Spikes, Big
 * hits, Avoidable, Defensives and potions, CC; group names the cooldowns that cover a spike. With seek, a row's time
 * moves the replay and a name focuses the player. With the fight's track, a death or big hit opens its recap. */
export function fightTabs(
  data: FightData,
  deathsName: string,
  group: 'party' | 'raid',
  seek?: Seeker,
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
            <RecapRow key={`${d.player}-${d.clockMS}-${i}`} recap={recapOf(track, d.player, d.clockMS)} ready={d.ready}>
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
        <div className="cards compact">
          {byPlayer(pressed, data.deaths).map(([player, casts]) => (
            <PlayerCard key={player} player={player} summary={countBy(casts.map(castName)) || 'pressed nothing'} seek={seek}>
              {casts.map((c, i) => (
                <div key={i} className="card-row">
                  <span className="name">{castName(c)}</span>
                  <When ms={c.clockMS} seek={seek} />
                </div>
              ))}
              <ul className="list">
                {data.deaths
                  .filter((d) => d.player === player)
                  .map((d, i) => (
                    <RecapRow key={i} recap={recapOf(track, d.player, d.clockMS)} ready={d.ready}>
                      <span className="bad">Died to {d.ability || 'unknown'}</span>
                      <span className="muted">
                        <When ms={d.clockMS} seek={seek} />
                      </span>
                    </RecapRow>
                  ))}
              </ul>
            </PlayerCard>
          ))}
          {pressed.length === 0 && data.deaths.length === 0 && <p className="muted">None</p>}
        </div>
      ),
    },
    {
      name: `CC ${cc.length}`,
      body: (
        <>
          <p className="muted small">Counts uses: an area stun counts once; a totem or trap counts when placed.</p>
          <div className="cards compact">
            {byPlayer(cc).map(([player, casts]) => (
              <PlayerCard key={player} player={player} summary={`${casts.length} uses`} seek={seek}>
                {casts.map((c, i) => (
                  <div key={i} className="card-row">
                    <span className="name">{c.ability}</span>
                    <When ms={c.clockMS} seek={seek} />
                  </div>
                ))}
              </PlayerCard>
            ))}
            {cc.length === 0 && <p className="muted">None ([cc] in mplus.toml)</p>}
          </div>
        </>
      ),
    },
  ]
}

/** A player's card: their name and a summary, with a button that opens the children below. */
// It lives with the tabs that use it; fightTabs beside it is why the rule fires.
// eslint-disable-next-line react-refresh/only-export-components
function PlayerCard(props: { player: string; summary: string; seek?: Seeker; children: ReactNode }) {
  const { player, summary, seek, children } = props
  const [open, setOpen] = useState(false)
  return (
    <div className="card small">
      <div className="card-row card-head">
        <span>
          <button className="back" aria-expanded={open} aria-label={`${player}'s details`} onClick={() => setOpen(!open)}>
            {open ? '▾' : '▸'}
          </button>{' '}
          <Who player={player} seek={seek} />
        </span>
        <span className="muted">{summary}</span>
      </div>
      {open && children}
    </div>
  )
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

/** Casts grouped by player, in the order players first used something; players in deaths who pressed nothing follow
 * with none. */
function byPlayer(casts: KeyCast[], deaths: KeyDeath[] = []): [string, KeyCast[]][] {
  const groups = new Map<string, KeyCast[]>()
  casts.forEach((c) => groups.set(c.player, [...(groups.get(c.player) ?? []), c]))
  deaths.forEach((d) => groups.has(d.player) || groups.set(d.player, []))
  return [...groups]
}
