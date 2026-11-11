import type { ReactNode } from 'react'
import type { KeyCast, KeyView } from './api'
import { clock, coverText } from './format'
import { KickLedger, SpikeLedger } from './KeyLedgers'

/** What a fight's tabs read: a key's view, or a raid pull's lists. */
export type FightData = Pick<
  KeyView,
  'deaths' | 'interrupts' | 'casts' | 'stops' | 'wastedKicks' | 'spikes' | 'debuffs' | 'reference'
>

/** A tab: its name and what it shows. */
export interface FightTab {
  name: string
  body: ReactNode
}

/** The tabs a key and a raid pull share, in order: Deaths (named deathsName), Interrupts, Must-stops, Spikes,
 * Defensives and potions, CC. */
export function fightTabs(data: FightData, deathsName: string): FightTab[] {
  const cc = data.casts.filter((c) => c.kind === 'cc')
  const pressed = data.casts.filter((c) => c.kind !== 'cc' && c.kind !== 'kick')
  return [
    {
      name: deathsName,
      body: (
        <ul className="list scroll">
          {data.deaths.map((d, i) => (
            <li key={`${d.player}-${d.clockMS}-${i}`} className="stack">
              <span>
                <b>{d.player}</b> {d.ability || 'unknown'} <span className="muted">from {d.killer || 'unknown'}</span>
              </span>
              <span className="muted">
                {clock(d.clockMS)} · {coverText(d.defensive, d.ready)}
              </span>
            </li>
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
                  <b>{i.player}</b> {i.kick && `${i.kick} → `}
                  {i.spell} <span className="muted">on {i.target}</span>
                </span>
                <span className="muted">{clock(i.clockMS)}</span>
              </li>
            ))}
          </ul>
        </>
      ),
    },
    {
      name: `Must-stops ${data.stops.filter((s) => s.outcome !== 'went off').length}/${data.stops.length}`,
      body: <KickLedger data={data} />,
    },
    { name: `Spikes ${data.spikes.length}`, body: <SpikeLedger data={data} /> },
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
  ]
}

/** How a pressed button is counted: a defensive by its name, a potion drunk before the pull as a pre-pot, the rest
 * by kind. */
function castName(c: KeyCast): string {
  if (c.kind === 'defensive') return c.ability
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
