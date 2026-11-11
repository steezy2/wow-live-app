import type { KeyForces, KeyGhost, KeyView } from './api'
import { chestLabel, clock, oddsText, resultText, share } from './format'
import { DamageGraph } from './DamageGraph'
import { PriorityLedger } from './KeyLedgers'
import { fightTabs } from './KeyTabs'
import { PlayerTable } from './PlayerTable'
import { KeyMap } from './KeyMap'
import { KeyReview } from './KeyReview'
import { MeterCards } from './MeterCards'
import { Tabs } from './Tabs'

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
  const fight = fightTabs(key, `Deaths ${key.deaths.length} (+${clock(key.penaltyMS)})`)
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

      {key.status !== 'running' && <KeyReview key={key.start} keyView={key} justEnded={justEnded} />}
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
          ...fight.slice(0, 4),
          {
            name: `Minibosses ${key.pulls.filter((p) => p.late).length}/${key.pulls.length}`,
            body: <PriorityLedger keyView={key} />,
          },
          ...fight.slice(4),
          {
            name: 'Players',
            body: (
              <PlayerTable
                title="Whole key"
                meter={key.overall}
                casts={key.casts}
                activity={key.activity}
                deaths={key.deaths}
                avoidableListed={key.reference}
              />
            ),
          },
        ]}
      />
      <p className="muted small">{key.players.join(' · ')}</p>
    </>
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
