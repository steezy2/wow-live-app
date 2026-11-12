import { useState } from 'react'
import type { KeyHit, KeyTrack, KeyView } from './api'
import type { FightData } from './KeyTabs'
import { amount, causesText, clock, coverText, groupBy } from './format'
import { defense, offense } from './meterColumns'
import { RecapRow, recapOf } from './HitRecap'
import { MeterCard } from './MeterCards'
import { meterBetween } from './replay'
import { When, Who, type Seeker } from './Seek'

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
export function KickLedger({ data: key, seek }: { data: FightData; seek?: Seeker }) {
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
                  <When ms={s.clockMS} seek={seek} /> {s.mob}: {s.spell} — {s.outcome}
                  {s.by && ` (${s.by}${s.intoMS ? `, ${(s.intoMS / 1000).toFixed(1)} s in` : ''})`}
                </span>
              ))}
          </li>
        ))}
      </ul>
    </>
  )
}

/** Each trash pull with priority targets (minibosses): when each died against the regular mobs of its pack (the ones
 * first hit within 10 s of it; red when it died after more than half of them), the share of the pull's damage into it
 * (and of its health when it's the only one), and the pull's share of damage in the first 10 s. */
export function PriorityLedger({ keyView: key }: { keyView: KeyView }) {
  return (
    <ul className="list scroll">
      {key.pulls.map((p) => (
        <li key={p.pull} className="stack">
          {p.minibosses.map((m, i) => (
            <span key={i} className={m.order - 1 > (m.of - 1) / 2 ? 'bad' : undefined}>
              Pull {p.pull} <span className="muted">· {clock(p.clockMS)}</span> {m.name}{' '}
              {!m.order
                ? "didn't die"
                : m.afterMS
                  ? `died ${Math.round(m.afterMS / 1000)} s after its pack`
                  : m.of === 1
                    ? 'died with no regular mobs in its pack'
                    : m.order === m.of
                      ? 'died after its pack'
                      : `died with ${m.of - m.order} of ${m.of - 1} pack mobs still up`}
              {m.damagePct > 0 && ` · ${Math.round(m.damagePct)}% of damage`}
              {p.minibosses.length === 1 && ` (${Math.round(p.healthPct)}% of health)`}
            </span>
          ))}
          <span className="muted">first 10 s: {Math.round(p.openingPct)}% of damage</span>
        </li>
      ))}
      {key.pulls.length === 0 && <li className="muted">No pulls with a priority target yet</li>}
    </ul>
  )
}

/** The worst bursts of party damage with each member's cover, and how fast dispellable debuffs came off. */
export function SpikeLedger(props: { data: FightData; group: 'party' | 'raid'; seek?: Seeker }) {
  const { data: key, group, seek } = props
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
              <b>
                <When ms={s.clockMS} seek={seek} />
              </b>{' '}
              {amount(s.damage)}
              {s.seconds > 0 && ` in ${s.seconds} s`}{' '}
              <span className={s.partyCDs.length ? 'muted' : 'bad'}>
                {s.partyCDs.length ? s.partyCDs.join(', ') : `no ${group} cooldown`}
                {s.overlap && ' (overlapping)'}
              </span>
            </span>
            {s.causes?.length ? <span>{causesText(s)}</span> : null}
            {s.members.map((m) => (
              <span key={m.player} className="muted">
                {m.player} {amount(m.taken)}: {coverText(m)}
              </span>
            ))}
          </li>
        ))}
        {key.spikes.length === 0 && <li className="muted">No spikes yet (needs a minute of fighting)</li>}
      </ul>
    </>
  )
}

/** Each big hit a player took, newest first: when, who, how much (and what % of their health), what hit them and their
 * cover, red when a defensive was ready and none used; a click opens its recap (a player who died soon after has the
 * death's). */
export function BigHits({ data, seek, track }: { data: FightData; seek?: Seeker; track?: KeyTrack | null }) {
  if (!data.bigHits) {
    return <p className="muted">Saved before big hits were kept: Read old logs in Settings fills it in</p>
  }
  const hits = [...data.bigHits].reverse()
  return (
    <ul className="list scroll">
      {hits.map((h) => (
        <RecapRow
          key={`${h.player}-${h.clockMS}`}
          recap={recapOf(track, h.player, h.clockMS)}
          ready={h.ready}
          died={h.died}
        >
          <span className={!h.used && !h.defensive && h.ready.length > 0 ? 'bad' : ''}>
            <When ms={h.clockMS} seek={seek} /> <Who player={h.player} seek={seek} /> {amount(h.taken)} (
            {Math.round(h.pct)}%){h.died && <b className="bad"> died</b>}
          </span>
          <span className="muted">
            {causesText({ damage: h.taken, causes: h.causes })} &middot; {coverText(h)}
          </span>
        </RecapRow>
      ))}
      {hits.length === 0 && <li className="muted">None</li>}
    </ul>
  )
}

const total = (hits: KeyHit[], pick: 'damage' | 'hits') => hits.reduce((n, h) => n + h[pick], 0)

/** The avoidable damage: a small card per player hit (what they took, from which ability), the most first, then the
 * list by ability, the most first: when it first hit (where the lanes mark it), how much and how many hits, and each
 * player it hit with theirs. */
export function Avoidable({ data, seek }: { data: FightData; seek?: Seeker }) {
  if (!data.reference) return <p className="muted">No avoidable list for this dungeon</p>
  if (!data.avoidableHits) {
    return <p className="muted">Saved before avoidable damage was kept: Read old logs in Settings fills it in</p>
  }
  const byAbility = groupBy(data.avoidableHits, (h) => h.ability)
  const first = (ability: string) =>
    data.lanes.find((l) => l.kind === 'avoidable' && l.name === ability)?.marks[0]?.clockMS
  const mostFirst = (a: [string, KeyHit[]], b: [string, KeyHit[]]) => total(b[1], 'damage') - total(a[1], 'damage')
  const abilities = [...byAbility].sort(mostFirst)
  const players = [...groupBy(data.avoidableHits, (h) => h.player)].sort(mostFirst)
  return (
    <>
      {players.length > 0 && (
        <div className="cards compact">
          {players.map(([player, hits]) => (
            <div key={player} className="card small">
              <div className="card-row card-head">
                <span>
                  <Who player={player} seek={seek} />
                </span>
                <span>{amount(total(hits, 'damage'))}</span>
              </div>
              {[...hits]
                .sort((a, b) => b.damage - a.damage)
                .map((h) => (
                  <div key={h.ability} className="card-row">
                    <span className="name">{h.ability}</span>
                    <span>{amount(h.damage)}</span>
                    <span className="muted">{h.hits > 0 && `(${h.hits})`}</span>
                  </div>
                ))}
            </div>
          ))}
        </div>
      )}
      <ul className="list scroll">
        {abilities.map(([ability, hits]) => {
          const at = first(ability)
          return (
            <li key={ability} className="stack">
              <span>
                {at !== undefined && <When ms={at} seek={seek} />} <b>{ability}</b> {amount(total(hits, 'damage'))}{' '}
                <span className="muted">· {total(hits, 'hits')} hits</span>
              </span>
              <span className="muted">
                {[...hits]
                  .sort((a, b) => b.damage - a.damage)
                  .map((h, i) => (
                    <span key={h.player}>
                      {i > 0 && ', '}
                      <Who player={h.player} seek={seek} /> {amount(h.damage)} {h.hits > 0 && `(${h.hits})`}
                    </span>
                  ))}
              </span>
            </li>
          )
        })}
        {abilities.length === 0 && <li className="muted">None</li>}
      </ul>
    </>
  )
}
