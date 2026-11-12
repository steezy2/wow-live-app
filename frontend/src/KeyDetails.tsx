import { useCallback, useMemo, useState } from 'react'
import type { KeyForces, KeyGhost, KeyReview as Review, KeyTrack, KeyView } from './api'
import { chestLabel, clock, oddsText, pullTitle, resultText, share } from './format'
import { DamageGraph } from './DamageGraph'
import { BossList, PriorityLedger } from './KeyLedgers'
import { lanesOf } from './events'
import { fightTabs } from './KeyTabs'
import { KeyMap } from './KeyMap'
import { KeyReview } from './KeyReview'
import { FightMeters, type MeterWindow } from './FightMeters'
import { offense } from './meterColumns'
import { useReplay, useTrack } from './replay'
import { RaidLanes } from './RaidReplay'
import { ReportLinks } from './ReportLinks'
import { Tabs } from './Tabs'
import { Who, type Seeker } from './Seek'

/** The result's icon, so it doesn't rest on colour alone. */
const resultIcon: Record<KeyView['status'], string> = {
  running: '',
  timed: '✓',
  depleted: '✗',
  abandoned: '–',
}

/** One key as the page shows it, live or saved, at a clock reading, with its review once it's over; the graph and the
 * boss list move the replay. Keyed by the key's start, so the replay starts over for another key. */
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
  const track = useTrack(key.start, key.status === 'running')
  const replay = useReplay(clockMS, clockMS)
  const [focus, setFocus] = useState('')
  const [replayBox, setReplayBox] = useState<HTMLElement | null>(null)
  const setAt = replay.setAt
  const show = useCallback(
    (ms: number) => {
      setAt(ms)
      replayBox?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    },
    [setAt, replayBox],
  )
  const seek = useMemo<Seeker | undefined>(
    () =>
      key.status === 'running' ? undefined : { onSeek: show, classes: track?.classes ?? {}, onFocus: setFocus },
    [key.status, show, track],
  )
  const fight = fightTabs(
    { ...key, lanes: key.lanes ?? [] },
    `Deaths ${key.deaths.length} (+${clock(key.penaltyMS)})`,
    'party',
    seek,
    track,
  )
  const [review, setReview] = useState<Review | null>(null)
  return (
    <>
      <header className="title">
        <h1>
          {key.dungeon} <span className="level">+{key.level}</span>
        </h1>
        <span className={`status ${key.status}`}>{key.status}</span>
      </header>
      <p className="muted">{key.affixes.map((a) => a.name || `affix ${a.id}`).join(' · ')}</p>
      {key.status !== 'running' && <ReportLinks fight={key.start} link={key.report} />}

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
      {(key.status === 'timed' || key.status === 'depleted') && key.bestFinalMS !== undefined && key.bestFinalMS > 0 && (
        <p className="muted">
          Your best timed +{key.level} here: {clock(key.bestFinalMS)} ·{' '}
          {clock(Math.abs(clockMS - key.bestFinalMS))} {clockMS <= key.bestFinalMS ? 'faster' : 'slower'}
        </p>
      )}
      {timer > 0 && <TimerBar clockMS={clockMS} timerMS={timer} keyView={key} best={projection?.best ?? []} />}
      {key.forces && <ForcesBar forces={key.forces} level={key.level} />}
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

      {key.status !== 'running' && key.suggestions && key.suggestions.length > 0 && (
        <section className="panel attention">
          <h2>Fix next key</h2>
          <ol className="fix-next">
            {key.suggestions.map((s) => (
              <li key={s}>{s}</li>
            ))}
          </ol>
        </section>
      )}
      {key.status !== 'running' && (
        <KeyReview keyView={key} justEnded={justEnded} onReview={setReview} />
      )}
      {key.status !== 'running' && track && (
        <DamageGraph
          keyView={key}
          track={track}
          now={replay.at}
          onSeek={replay.setAt}
          focus={focus}
          onFocus={setFocus}
        />
      )}

      {key.status !== 'running' && key.lanes && key.lanes.length > 0 && (
        <section className="panel">
          <h2>Timeline</h2>
          <RaidLanes
            lanes={focus ? lanesOf(key.lanes, focus) : key.lanes}
            end={clockMS}
            now={replay.at}
            onSeek={replay.setAt}
          />
        </section>
      )}

      <FightMeters
        windows={meterWindows(key, track, replay.at, clockMS)}
        track={track}
        atEnd={key.status === 'running' || replay.at >= clockMS}
        fight={key}
        offense={offense}
        group="party"
        classes={track?.classes ?? {}}
        focus={focus}
        onFocus={key.status === 'running' ? undefined : setFocus}
      />

      <section ref={setReplayBox} className="panel map-panel">
        <h2>Replay</h2>
        <KeyMap keyView={key} clockMS={clockMS} track={track} replay={replay} focus={focus} onFocus={setFocus} />
      </section>

      <Tabs
        tabs={[
          {
            name: `Priority targets ${key.pulls.filter((p) => p.late).length}/${key.pulls.length}`,
            body: <PriorityLedger keyView={key} />,
          },
          {
            name: `Buffs ${key.buffs.length}`,
            body: (
              <ul className="list scroll">
                {key.buffs.map((b) => (
                  <li key={b.player} className="stack">
                    <Who player={b.player} seek={seek} />
                    <span className="muted">{b.missing.join(', ')}</span>
                  </li>
                ))}
                {key.buffs.length === 0 && (
                  <li className="muted">Everyone had their flask, food, rune and party buffs</li>
                )}
              </ul>
            ),
          },
          {
            name: 'Bosses',
            body: <BossList keyView={key} track={track} topBosses={review?.topBosses} onSeek={show} />,
          },
          ...fight,
        ]}
      />
      <p className="muted small">{key.players.join(' · ')}</p>
    </>
  )
}

/** The key's meter windows at the replay's moment (at; end: the key's end): the pull it's in, or the last one before
 * it, then the whole key so far, each with its saved meter for the end. A running key, or one whose track has no
 * pulls, shows its saved last pull and whole key. */
function meterWindows(key: KeyView, track: KeyTrack | null, at: number, end: number): MeterWindow[] {
  const whole: MeterWindow = { title: 'Whole key', from: 0, to: at, saved: key.overall }
  const last = key.lastPull
  if (key.status === 'running' || !track?.pulls?.length) {
    return [{ title: pullTitle(key, end), from: last.startMS, to: last.endMS, saved: last }, { ...whole, to: end }]
  }
  const started = track.pulls.filter((p) => p.startMS <= at)
  const pull = started[started.length - 1] ?? { startMS: 0, endMS: 0 } // before the first pull: "Pull 1" from 0
  const to = pull.endMS > 0 && pull.endMS < at ? pull.endMS : at
  return [{ title: `Pull ${Math.max(1, started.length)}`, from: pull.startMS, to, saved: last }, whole]
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

/** Within this many % of the best run's pace, the key is on pace with it. */
const PACE_EVEN_PCT = 1

/** Enemy forces as a bar: the share killed, and against the best run at the level (a tick where it stood at this
 * time, and how much faster or slower the key is). */
export function ForcesBar({ forces, level }: { forces: KeyForces; level: number }) {
  const pace = forces.pace
  const best = `best +${level}`
  return (
    <div className="forces">
      <div className="bar" role="img" aria-label={`forces ${forces.pct.toFixed(1)}%`}>
        <div
          className={`fill ${pace && pace.fasterPct <= -PACE_EVEN_PCT ? 'behind' : 'ahead'}`}
          style={{ width: `${Math.min(100, forces.pct)}%` }}
        />
        {pace && <div className="mark" style={{ left: `${Math.min(100, pace.bestPct)}%` }} title={best} />}
      </div>
      <span className="small">
        Forces {forces.pct >= 100 ? `100% (${forces.pct.toFixed(0)}%)` : `${forces.pct.toFixed(1)}%`}
        {forces.count > 0 && <span className="muted"> · {forces.count}/{forces.total}</span>}
        {pace && (
          <span className="muted">
            {' '}
            ·{' '}
            {Math.abs(pace.fasterPct) < PACE_EVEN_PCT
              ? `on pace with ${best}`
              : `${Math.round(Math.abs(pace.fasterPct))}% ${pace.fasterPct > 0 ? 'faster' : 'slower'} than ${best}`}
          </span>
        )}
      </span>
    </div>
  )
}
