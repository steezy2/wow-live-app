import { type ReactNode, useState } from 'react'
import type { KeyCast, KeyForces, KeyGhost, KeyView } from './api'
import { chestLabel, clock, coverText, oddsText, resultText, share } from './format'
import { DamageGraph } from './DamageGraph'
import { KickLedger, PriorityLedger, SpikeLedger } from './KeyLedgers'
import { KeyMap } from './KeyMap'
import { KeyReview } from './KeyReview'
import { MeterCards } from './MeterCards'

/** The result's icon, so it doesn't rest on colour alone. */
const resultIcon: Record<KeyView['status'], string> = {
  running: '',
  timed: '✓',
  depleted: '✗',
  abandoned: '–',
}

/** One key as the page shows it, live or saved, at a clock reading, with its review once it's over. */
export function KeyDetails({
  keyView: key,
  clockMS,
  justEnded = false,
}: {
  keyView: KeyView
  clockMS: number
  /** The key ended while the page was watching, so its review may still be on the way. */
  justEnded?: boolean
}) {
  const timer = key.timerMS ?? 0
  const projection = key.status === 'running' ? key.projection : undefined
  const cc = key.casts.filter((c) => c.kind === 'cc')
  return (
    <>
      <header className="title">
        <h1>
          {key.dungeon} <span className="level">+{key.level}</span>
        </h1>
        <span className={`status ${key.status}`}>{key.status}</span>
      </header>
      <p className="muted">{key.affixes.map((a) => a.name || `affix ${a.id}`).join(' · ')}</p>

      {key.status === 'running' ? (
        <section className="clock">
          <span className="big">{clock(clockMS)}</span>
          {timer > 0 && <span className="muted"> / {clock(timer)}</span>}
          {projection ? (
            <span className={`chests c${projection.chests}`}>
              On pace for {chestLabel(projection.chests)} · about {clock(projection.finalMS)}
              {projection.runs > 1 && (
                <span className="muted small">
                  {' '}
                  ({clock(projection.lowMS)}–{clock(projection.highMS)}) · {oddsText(projection.odds)}
                </span>
              )}
            </span>
          ) : (
            <span className={`chests c${key.chests}`}>
              {chestLabel(key.chests)}
              {key.chests > 0 && ' still possible'}
              <span className="muted small">
                {' '}
                · no estimate: no finished run of this dungeon here yet, and top Warcraft Logs runs need the keys on
                the Settings tab
              </span>
            </span>
          )}
        </section>
      ) : (
        <section className={`result ${key.status}`}>
          <span aria-hidden="true">{resultIcon[key.status]}</span> {resultText(key.status, key.chests, clockMS, timer)}
        </section>
      )}
      {timer > 0 && <TimerBar clockMS={clockMS} timerMS={timer} keyView={key} best={projection?.best ?? []} />}
      {key.forces && <ForcesBar forces={key.forces} />}
      {timer > 0 && key.status === 'running' && (
        <p className="muted">
          {clock(timer - clockMS)} left · +3 by {clock(key.plus3MS)} · +2 by {clock(key.plus2MS)}
          {projection && ` · estimate from ${projection.source}`}
        </p>
      )}
      {projection && projection.best.some((g) => !key.bosses.some((b) => b.kill && b.name === g.name)) && (
        <p className="muted small">
          To time it like your best run:{' '}
          {projection.best
            .filter((g) => !key.bosses.some((b) => b.kill && b.name === g.name))
            .map((g) => `${g.name} by ${clock(g.needByMS)}`)
            .join(' · ')}
        </p>
      )}
      {key.status === 'running' && key.stops.some((s) => s.outcome === 'went off') && (
        <p className="warning">
          Went off:{' '}
          {key.stops
            .filter((s) => s.outcome === 'went off')
            .slice(-3)
            .reverse()
            .map((s) => `${s.spell} (${s.mob}, ${clock(s.clockMS)})`)
            .join(' · ')}
        </p>
      )}
      {key.forces?.needsCheck && (
        <p className="warning">
          The kills here added up to{' '}
          {key.forces.count > 0 ? `${key.forces.count} of ${key.forces.total}` : `${key.forces.pct.toFixed(1)}%`} of
          the forces: the dungeon file needs checking (go run ./cmd/mplusref --check).
          {key.forces.uncounted?.length
            ? ` Not in it: ${key.forces.uncounted.map((u) => `${u.name} (NPC ${u.npc}) ×${u.deaths}`).join(', ')}.`
            : ''}
        </p>
      )}

      {key.status !== 'running' && key.status !== 'abandoned' && (
        <KeyReview key={key.start} keyView={key} justEnded={justEnded} />
      )}
      {key.status !== 'running' && <DamageGraph key={`graph-${key.start}`} keyView={key} />}

      {key.buffs.length > 0 && (
        <section className="panel attention">
          <h2>Missing at the start</h2>
          {key.buffs.map((b) => (
            <p key={b.player}>
              <b>{b.player}</b> <span className="muted">{b.missing.join(', ')}</span>
            </p>
          ))}
        </section>
      )}

      <MeterCards keyView={key} clockMS={clockMS} />

      <section className="panel map-panel">
        <h2>Replay</h2>
        <KeyMap key={key.start} keyView={key} clockMS={clockMS} />
      </section>

      <Tabs
        tabs={[
          {
            name: 'Bosses',
            body: (
              <ul className="list">
                {key.bosses.map((b) => (
                  <li key={`${b.name}-${b.startMS}`}>
                    <span>{b.name}</span>
                    <span className="muted">
                      {clock(b.startMS)} →{' '}
                      {b.endMS === undefined ? 'on now' : `${clock(b.endMS)}${b.kill ? '' : ' (wipe)'}`}
                    </span>
                  </li>
                ))}
                {key.bosses.length === 0 && <li className="muted">None yet</li>}
              </ul>
            ),
          },
          {
            name: `Deaths ${key.deaths.length} (+${clock(key.penaltyMS)})`,
            body: (
              <ul className="list">
                {key.deaths.map((d, i) => (
                  <li key={`${d.player}-${d.clockMS}-${i}`} className="stack">
                    <span>
                      <b>{d.player}</b> {d.ability || 'unknown'}{' '}
                      <span className="muted">from {d.killer || 'unknown'}</span>
                    </span>
                    <span className="muted">
                      {clock(d.clockMS)} · {coverText(d.defensive, d.ready)}
                    </span>
                  </li>
                ))}
                {key.deaths.length === 0 && <li className="muted">None</li>}
              </ul>
            ),
          },
          {
            name: `Interrupts ${key.interrupts.length}`,
            body: (
              <>
                <p className="muted">
                  Every interrupt, must-stop or not:{' '}
                  {countBy(key.interrupts.map((i) => `${i.player} (${i.kick || 'kick'})`))}
                </p>
                <ul className="list scroll">
                  {[...key.interrupts].reverse().map((i, n) => (
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
            name: `Must-stops ${key.stops.filter((s) => s.outcome !== 'went off').length}/${key.stops.length}`,
            body: <KickLedger keyView={key} />,
          },
          {
            name: `Spikes ${key.spikes.length}`,
            body: <SpikeLedger keyView={key} />,
          },
          {
            name: `Minibosses ${key.pulls.filter((p) => p.late).length}/${key.pulls.length}`,
            body: <PriorityLedger keyView={key} />,
          },
          {
            name: 'Defensives and potions',
            body: (
              <ul className="list scroll">
                {byPlayer(key.casts.filter((c) => c.kind !== 'cc')).map(([player, casts]) => (
                  <li key={player} className="stack">
                    <b>{player}</b>
                    <span className="muted">
                      {countBy(casts.map((c) => (c.kind === 'defensive' ? c.ability : c.kind)))}
                    </span>
                  </li>
                ))}
                {key.casts.length === cc.length && <li className="muted">None</li>}
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
        ]}
      />
      <p className="muted small">{key.players.join(' · ')}</p>
    </>
  )
}

/** Lists under tabs, one shown at a time. */
function Tabs({ tabs }: { tabs: { name: string; body: ReactNode }[] }) {
  const [open, setOpen] = useState(0)
  return (
    <section className="tabs">
      <div role="tablist">
        {tabs.map((t, i) => (
          <button key={t.name} role="tab" aria-selected={i === open} onClick={() => setOpen(i)}>
            {t.name}
          </button>
        ))}
      </div>
      <div role="tabpanel">{tabs[open].body}</div>
    </section>
  )
}

/** The timer as a bar: the clock so far, the +3 and +2 marks, each boss kill, each death, and the best run's kills. */
export function TimerBar({
  clockMS,
  timerMS,
  keyView,
  best,
}: {
  clockMS: number
  timerMS: number
  keyView: KeyView
  best: KeyGhost[]
}) {
  return (
    <div className="bar" role="img" aria-label={`${clock(clockMS)} of ${clock(timerMS)}`}>
      <div className="fill" style={{ width: `${share(clockMS, timerMS)}%` }} />
      <div className="mark" style={{ left: `${share(keyView.plus3MS, timerMS)}%` }} title="+3" />
      <div className="mark" style={{ left: `${share(keyView.plus2MS, timerMS)}%` }} title="+2" />
      {keyView.bosses
        .filter((b) => b.kill && b.endMS !== undefined)
        .map((b) => (
          <div key={b.name} className="boss" style={{ left: `${share(b.endMS ?? 0, timerMS)}%` }} title={b.name} />
        ))}
      {keyView.deaths.map((d, i) => (
        <div key={i} className="death" style={{ left: `${share(d.clockMS, timerMS)}%` }} title={d.player} />
      ))}
      {best.map((g) => (
        <div
          key={g.name}
          className="ghost"
          style={{ left: `${share(g.killMS, timerMS)}%` }}
          title={`${g.name} in your best run`}
        />
      ))}
    </div>
  )
}

/** Enemy forces as a bar: the share killed, a mark at 100%, and a tick where earlier runs stood at this time. */
export function ForcesBar({ forces }: { forces: KeyForces }) {
  const ref = forces.referencePct
  const ahead = forces.pct >= ref
  return (
    <div className="forces">
      <div className="bar" role="img" aria-label={`forces ${forces.pct.toFixed(1)}%`}>
        <div
          className={`fill ${ref > 0 && !ahead ? 'behind' : 'ahead'}`}
          style={{ width: `${Math.min(100, forces.pct)}%` }}
        />
        {ref > 0 && <div className="mark" style={{ left: `${Math.min(100, ref)}%` }} title="your earlier runs" />}
      </div>
      <span className="small">
        Forces {forces.pct >= 100 ? `100% (${forces.pct.toFixed(0)}%)` : `${forces.pct.toFixed(1)}%`}
        {forces.count > 0 && <span className="muted"> · {forces.count}/{forces.total}</span>}
        {ref > 0 && (
          <span className="muted">
            {' '}
            · {ahead ? 'ahead' : 'behind'} of your runs ({ref.toFixed(0)}%)
          </span>
        )}
      </span>
    </div>
  )
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
