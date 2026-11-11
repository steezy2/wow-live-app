import { useState } from 'react'
import type { KeyTrack, KeyView } from './api'
import type { FightData } from './KeyTabs'
import { amount, clock, coverText } from './format'
import { defense, offense } from './meterColumns'
import { MeterCard } from './MeterCards'
import { meterBetween } from './replay'

/** The key's boss fights; a click moves the replay to one and opens its details: its length against your best run at
 * the level and the top runs (topBosses, from the review), the deaths and must-stops that went off in it, and the
 * party's DPS and HPS on it. */
export function BossList(props: {
  keyView: KeyView
  track: KeyTrack | null
  topBosses?: Record<string, number>
  onSeek: (clockMS: number) => void
}) {
  const { keyView: key, track, topBosses, onSeek } = props
  const [open, setOpen] = useState('')
  if (key.bosses.length === 0) return <p className="muted">None yet</p>
  return (
    <ul className="list">
      {key.bosses.map((b) => {
        const id = `${b.name}-${b.startMS}`
        const end = b.endMS ?? key.clockMS
        const inFight = (ms: number) => ms >= b.startMS && ms <= end
        const best = b.kill ? key.bestBosses?.[b.name] : undefined
        const top = b.kill ? topBosses?.[b.name] : undefined
        const went = key.stops.filter((s) => s.outcome === 'went off' && inFight(s.clockMS))
        const deaths = key.deaths.filter((d) => inFight(d.clockMS))
        return (
          <li key={id} className="stack">
            <button
              className="row-button"
              aria-expanded={open === id}
              onClick={() => {
                setOpen(open === id ? '' : id)
                onSeek(b.startMS)
              }}
            >
              <span>{b.name}</span>
              <span className="muted">
                {clock(b.startMS)} →{' '}
                {b.endMS === undefined ? 'on now' : `${clock(b.endMS)}${b.kill ? '' : ' (wipe)'}`}
              </span>
            </button>
            {open === id && (
              <>
                <span>
                  {clock(end - b.startMS)}
                  {best !== undefined && (
                    <span className="muted">
                      {' '}
                      · {clock(Math.abs(end - b.startMS - best))}{' '}
                      {end - b.startMS <= best ? 'faster' : 'slower'} than your best +{key.level}
                    </span>
                  )}
                  {top !== undefined && <span className="muted"> · top runs about {clock(top)}</span>}
                </span>
                <span className={deaths.length ? 'bad' : 'muted'}>
                  Deaths: {deaths.length ? deaths.map((d) => `${d.player} (${clock(d.clockMS)})`).join(', ') : 'none'}
                </span>
                <span className={went.length ? 'bad' : 'muted'}>
                  Must-stops that went off: {went.length ? went.map((s) => `${s.spell} (${s.mob})`).join(', ') : 'none'}
                </span>
                {track && (
                  <div className="replay-meters">
                    {[offense.slice(0, 2), defense(false).slice(0, 2)].map((columns) => (
                      <MeterCard
                        key={columns[0].label}
                        title={`On ${b.name}`}
                        meter={meterBetween(track, b.startMS, end)}
                        columns={columns}
                      />
                    ))}
                  </div>
                )}
              </>
            )}
          </li>
        )
      })}
    </ul>
  )
}

/** Must-stop casts (the dungeon's casts that have to be stopped) by pull, what happened to each, and each player's
 * must-stop interrupts and wasted kicks. Every interrupt, must-stop or not, is on the Interrupts tab. */
export function KickLedger({ data: key }: { data: FightData }) {
  const outcomes = new Map<string, number>()
  key.stops.forEach((s) => outcomes.set(s.outcome, (outcomes.get(s.outcome) ?? 0) + 1))
  const players = new Map<string, { kicks: number; wasted: number }>()
  const add = (player: string, field: 'kicks' | 'wasted') => {
    const counts = players.get(player) ?? { kicks: 0, wasted: 0 }
    counts[field]++
    players.set(player, counts)
  }
  key.stops.filter((s) => s.outcome === 'interrupted').forEach((s) => add(s.by, 'kicks'))
  key.wastedKicks.forEach((w) => add(w.player, 'wasted'))
  const pulls = [...new Set(key.stops.map((s) => s.pull))]
  if (!key.reference)
    return <p className="muted">No must-stop list for this fight (reference/mplus-dungeons.json).</p>
  return (
    <>
      <p>
        Must-stop casts {key.stops.length}
        <span className="muted"> · {[...outcomes].map(([o, n]) => `${o} ${n}`).join(' · ')}</span>
      </p>
      <p className="muted">
        Interrupted by:{' '}
        {[...players].map(([p, n]) => `${p} ${n.kicks}${n.wasted ? ` (${n.wasted} wasted kicks)` : ''}`).join(' · ')}
      </p>
      <ul className="list scroll">
        {pulls.reverse().map((pull) => (
          <li key={pull} className="stack">
            <span className="muted">Pull {pull}</span>
            {key.stops
              .filter((s) => s.pull === pull)
              .map((s, i) => (
                <span key={i} className={s.outcome === 'went off' ? 'bad' : undefined}>
                  {clock(s.clockMS)} {s.mob}: {s.spell} — {s.outcome}
                  {s.by && ` (${s.by}${s.intoMS ? `, ${(s.intoMS / 1000).toFixed(1)} s in` : ''})`}
                </span>
              ))}
          </li>
        ))}
      </ul>
    </>
  )
}

/** Each trash pull with minibosses: how many of each one's pack (the regular mobs first hit within 10 s of it) died
 * before it, which is fine as long as it's under half (red when it isn't), and the share of damage into them against
 * their share of the pull's health. */
export function PriorityLedger({ keyView: key }: { keyView: KeyView }) {
  return (
    <ul className="list scroll">
      {key.pulls.map((p) => (
        <li key={p.pull} className="stack">
          <span>
            Pull {p.pull} <span className="muted">{clock(p.clockMS)}</span>
          </span>
          {p.minibosses.map((m, i) => (
            <span key={i} className={m.order - 1 > (m.of - 1) / 2 ? 'bad' : undefined}>
              {m.name}:{' '}
              {!m.order
                ? "didn't die"
                : m.of === 1
                  ? 'no regular mobs in its pack'
                  : `${m.order - 1} of ${m.of - 1} pack mobs died first`}
            </span>
          ))}
          <span className="muted">
            {Math.round(p.priorityPct)}% of the damage into the minibosses ({Math.round(p.healthPct)}% of the pull's
            health; {Math.round(p.openingPct)}% in the first 10 s)
          </span>
        </li>
      ))}
      {key.pulls.length === 0 && <li className="muted">No pulls with a miniboss yet</li>}
    </ul>
  )
}

/** The worst bursts of party damage with each member's cover, and how fast dispellable debuffs came off. */
export function SpikeLedger({ data: key, group }: { data: FightData; group: 'party' | 'raid' }) {
  const dispelled = key.debuffs.filter((d) => d.by).map((d) => d.afterMS)
  dispelled.sort((a, b) => a - b)
  return (
    <>
      <p>
        Dispellable debuffs {key.debuffs.length}
        <span className="muted">
          {dispelled.length > 0 &&
            ` · median ${(dispelled[Math.floor(dispelled.length / 2)] / 1000).toFixed(1)} s to dispel`}
          {key.debuffs.length > dispelled.length && ` · ${key.debuffs.length - dispelled.length} ran out`}
        </span>
      </p>
      <ul className="list scroll">
        {key.spikes.map((s) => (
          <li key={s.clockMS} className="stack">
            <span>
              <b>{clock(s.clockMS)}</b> {amount(s.damage)} in 3 s{' '}
              <span className={s.partyCDs.length ? 'muted' : 'bad'}>
                {s.partyCDs.length ? s.partyCDs.join(', ') : `no ${group} cooldown`}
                {s.overlap && ' (overlapping)'}
              </span>
            </span>
            {s.members.map((m) => (
              <span key={m.player} className="muted">
                {m.player} {amount(m.taken)}: {coverText(m.defensive, m.ready)}
              </span>
            ))}
          </li>
        ))}
        {key.spikes.length === 0 && <li className="muted">No spikes yet (needs a minute of fighting)</li>}
      </ul>
    </>
  )
}
