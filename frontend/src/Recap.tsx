import { useEffect, useState } from 'react'
import {
  fetchRecap,
  fetchRecapPeriods,
  fetchRecapTopTimes,
  type Character,
  type Recap as RecapData,
  type RecapDungeon,
  type RecapMPlus,
  type RecapKeyCounts,
  type RecapKeyRow,
  type RecapPeriod,
  type RecapRaid,
  type RecapTopTime,
} from './api'
import { CharacterParses } from './CharacterParses'
import { classColors } from './classColors'
import { Skeleton } from './Skeleton'
import { useDelayed } from './useDelayed'
import { amount, clock, counted, realmName, versus, withRealm } from './format'
import { Tabs } from './Tabs'

type Open = (kind: 'mplus' | 'raid', start: number) => void

/** The Recap tab for the character picked in the top bar (all of them when none): a period picked with chips (this
 * month by default), then Mythic+ and raid on tabs of their own, each read as a story: tiles, cards, facts that open
 * their key or pull (onOpen), and the Warcraft Logs parses of Settings' character (parsesFor), shown when the recap
 * is theirs or everyone's. */
export function Recap({ character, parsesFor, onOpen }: { character?: Character; parsesFor: string; onOpen: Open }) {
  const id = character?.id ?? ''
  const [periods, setPeriods] = useState<RecapPeriod[]>([])
  const [label, setLabel] = useState('')
  // The last answer, for the period and character it was asked for (key): a recap or why it failed. It stays on
  // screen while the next one loads, so the tabs keep the one picked.
  const [answer, setAnswer] = useState<{ key?: string; recap?: RecapData; error?: string }>({})
  const [top, setTop] = useState<{ key?: string; times?: RecapTopTime[] | null }>({})
  const period = periods.find((p) => p.label === label) ?? periods[0]
  const key = period && `${period.label} ${id}`
  useEffect(() => {
    const controller = new AbortController()
    fetchRecapPeriods(controller.signal).then(setPeriods, (err: unknown) => {
      if (!controller.signal.aborted) setAnswer({ error: err instanceof Error ? err.message : String(err) })
    })
    return () => controller.abort()
  }, [])
  useEffect(() => {
    if (!period) return
    const controller = new AbortController()
    fetchRecap(period, id, controller.signal).then(
      (recap) => setAnswer({ key, recap }),
      (err: unknown) => {
        if (!controller.signal.aborted) setAnswer({ key, error: err instanceof Error ? err.message : String(err) })
      },
    )
    // The recap reads fine without Warcraft Logs, so a failure only leaves the top runs out.
    fetchRecapTopTimes(period, id, controller.signal).then((times) => setTop({ key, times }), () => undefined)
    return () => controller.abort()
  }, [period, id, key])
  const loading = answer.key !== key
  const slow = useDelayed(loading)
  const showParses = !character || character.preferred
  return (
    <>
      <header className="title">
        <h1>
          Recap ·{' '}
          <span style={{ color: character && classColors[character.class] }}>
            {character ? withRealm(character.name, character.realm) : 'All characters'}
          </span>
        </h1>
        {slow && answer.recap && <span className="muted small">Loading…</span>}
      </header>
      <div className="map-controls" role="group" aria-label="Period">
        {periods.map((p) => (
          <button key={p.label} aria-pressed={p === period} onClick={() => setLabel(p.label)}>
            {p.label}
          </button>
        ))}
      </div>
      {answer.error && !loading ? (
        <p className="warning">{answer.error}</p>
      ) : answer.recap ? (
        <Tabs
          initial={answer.recap.mplus.keys === 0 && answer.recap.raid.pulls > 0 ? 1 : 0}
          tabs={[
            {
              name: 'Mythic+',
              body: (
                <>
                  <MPlusRecap r={answer.recap.mplus} top={top.key === key ? top.times : undefined} onOpen={onOpen} />
                  {showParses && <CharacterParses character={parsesFor} kind="mplus" />}
                </>
              ),
            },
            {
              name: 'Raid',
              body: (
                <>
                  <RaidRecap r={answer.recap.raid} onOpen={onOpen} />
                  {showParses && <CharacterParses character={parsesFor} kind="raid" />}
                </>
              ),
            },
          ]}
        />
      ) : (
        <Skeleton height={320} lines={5} />
      )}
    </>
  )
}

/** Big numbers in boxes, each with its label under it. */
function Tiles({ tiles }: { tiles: [string, string][] }) {
  return (
    <div className="recap-tiles">
      {tiles.map(([value, label]) => (
        <div key={label} className="card small">
          <b>{value}</b>
          <span className="muted small">{label}</span>
        </div>
      ))}
    </div>
  )
}

/** A key row's counts, label and field, each showing yours with the party's beside it. */
const KEY_COUNTS: [string, keyof RecapKeyCounts][] = [
  ['Kicks', 'kicks'],
  ['CC', 'cc'],
  ['Dispels', 'dispels'],
  ['Priority %', 'priorityPct'],
]

/** The key rows' columns: the key, DPS, HPS, then each count with the party's. */
const KEY_GRID = { gridTemplateColumns: `minmax(140px, 1fr) repeat(2, 56px) repeat(${KEY_COUNTS.length}, 80px)` }

/** A number of a key, "–" when not known (-1). */
function perKey(n: number, show: (n: number) => string = (n) => n.toFixed(0)): string {
  return n < 0 ? '–' : show(n)
}

/** The Mythic+ tab: tiles, a card per dungeon (top: Warcraft Logs' top runs, undefined while loading, null without
 * Warcraft Logs keys), a row per key, the character, who they ran with, and facts that open their key. */
function MPlusRecap({ r, top, onOpen }: { r: RecapMPlus; top?: RecapTopTime[] | null; onOpen: Open }) {
  if (r.keys === 0) return <p className="muted">No keys in this period</p>
  const you = r.you
  const tiles: [string, string][] = [
    [String(r.keys), 'keys'],
    [`${Math.round(r.successPct)}%`, 'timed'],
    [r.highestTimed ? `+${r.highestTimed}` : '–', 'highest timed'],
    [`${(r.timeMS / 3_600_000).toFixed(1)} h`, 'in keys'],
    [(r.deaths / r.keys).toFixed(1), 'party deaths a key'],
  ]
  if (you?.keys) tiles.push([(you.deaths / you.keys).toFixed(1), 'your deaths a key'])
  return (
    <>
      <Tiles tiles={tiles} />
      <div className="cards compact">
        {r.dungeons.map((d) => (
          <DungeonCard key={d.dungeon} d={d} top={top?.find((t) => t.dungeon === d.dungeon)} />
        ))}
      </div>
      <div className="cards compact">
        <KeyRows rows={r.keyRows} onOpen={onOpen} />
        {you && (
          <section className="card small">
            <h3>{you.character}</h3>
            <p className="muted small">{you.specs.join(', ') || 'Spec not recorded'}</p>
            {you.dungeons.map((d) => (
              <div key={d.dungeon + d.metric} className="card-row">
                <span className="name">{d.dungeon}</span>
                <span>
                  {amount(d.median)} <span className="muted small">{d.metric.toUpperCase()}</span>
                </span>
              </div>
            ))}
            {you.killers.length > 0 && (
              <p className="small">
                <span className="muted">Killed most by </span>
                {you.killers.map((k) => `${k.name} (${k.count})`).join(', ')}
              </p>
            )}
          </section>
        )}
        {r.party.length > 0 && (
          <section className="card small">
            <h3>Who you ran with</h3>
            {r.party.map((m) => (
              <div key={m.player + m.realm} className="card-row">
                <span className="name">
                  <span style={{ color: classColors[m.class] }}>{m.player}</span>
                  {m.realm && <span className="muted small"> {realmName(m.realm)}</span>}
                </span>
                <span>
                  {counted(m.keys, 'key')}{' '}
                  <span className="muted small">{Math.round((100 * m.timed) / m.keys)}% timed</span>
                </span>
              </div>
            ))}
          </section>
        )}
      </div>
      {r.facts.length > 0 && (
        <section className="panel">
          <h2>Facts</h2>
          <ul className="list">
            {r.facts.map((f) => (
              <li key={f.text}>
                <button className="link" onClick={() => onOpen(f.kind, f.start)}>
                  {f.text}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </>
  )
}

/** A row per key, newest first, each opening its key (onOpen): your DPS and HPS, then your counts with the party's
 * muted beside them. One card across the whole row that scrolls. */
function KeyRows({ rows, onOpen }: { rows: RecapKeyRow[]; onOpen: Open }) {
  return (
    <section className="card small" style={{ gridColumn: '1 / -1' }}>
      <h3>Keys</h3>
      <div className="card-scroll" style={{ maxHeight: 360, overflowY: 'auto' }}>
        <div className="card-table">
          <div className="card-row card-head muted" style={KEY_GRID}>
            <span>Key</span>
            <span>DPS</span>
            <span>HPS</span>
            {KEY_COUNTS.map(([label]) => (
              <span key={label}>{label}</span>
            ))}
          </div>
          {rows.map((k) => (
            <div key={k.start} className="card-row" style={KEY_GRID}>
              <span className="name">
                <button className="link" onClick={() => onOpen('mplus', k.start)}>
                  {k.dungeon} <span className={k.status === 'timed' ? 'good' : 'bad'}>+{k.level}</span>
                </button>
              </span>
              <span>{perKey(k.you.dps, amount)}</span>
              <span>{perKey(k.you.hps, amount)}</span>
              {KEY_COUNTS.map(([label, field]) => (
                <span key={label}>
                  {perKey(k.you[field])} <span className="muted small">{perKey(k.party[field])}</span>
                </span>
              ))}
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}

/** One dungeon: runs and success, then your timed runs at the highest level you timed and their times against
 * Warcraft Logs' top runs there (top; missing while loading or without Warcraft Logs keys). One time when they all
 * took the same (timed once). */
function DungeonCard({ d, top }: { d: RecapDungeon; top?: RecapTopTime }) {
  return (
    <section className="card small">
      <h3>
        {d.dungeon} {d.bestLevel > 0 && <span className="muted">+{d.bestLevel}</span>}
      </h3>
      <div className="bar thin" role="img" aria-label={`${Math.round(d.successPct)}% timed`}>
        <div className="fill ahead" style={{ width: `${d.successPct}%` }} />
      </div>
      <p className="small">
        {counted(d.runs, 'run')}, {Math.round(d.successPct)}% timed
      </p>
      <p className="muted small">
        {d.bestLevel
          ? `Timed at +${d.bestLevel}: ${d.bestRuns === 1 ? 'only once' : `${d.bestRuns} times`}`
          : 'None timed'}
      </p>
      {d.bestLevel > 0 && (
        <dl className="recap-times">
          {d.bestRuns > 1 ? (
            <>
              <dt>Average</dt>
              <dd>{clock(d.avgMS)}</dd>
              <dt>Median</dt>
              <dd>{clock(d.medianMS)}</dd>
              <dt>Fastest</dt>
              <dd>{clock(d.bestMS)}</dd>
            </>
          ) : (
            <>
              <dt>Time</dt>
              <dd>{clock(d.medianMS)}</dd>
            </>
          )}
          {top && (
            <>
              <dt>Top runs median</dt>
              {top.medianMS > 0 ? (
                <dd>
                  {clock(top.medianMS)}{' '}
                  <span className={d.medianMS > top.medianMS ? 'bad' : 'good'}>
                    {versus(d.medianMS, top.medianMS)}
                  </span>
                </dd>
              ) : (
                <dd className="muted">too few ranked runs</dd>
              )}
            </>
          )}
        </dl>
      )}
    </section>
  )
}

/** The Raid tab: tiles and a card per boss, its first kill opening the pull (onOpen). */
function RaidRecap({ r, onOpen }: { r: RecapRaid; onOpen: Open }) {
  if (r.pulls === 0) return <p className="muted">No raid pulls in this period</p>
  return (
    <>
      <Tiles
        tiles={[
          [String(r.pulls), 'pulls'],
          [String(r.kills), 'kills'],
          [String(r.killed), 'bosses killed'],
          [`${(r.timeMS / 3_600_000).toFixed(1)} h`, 'in pulls'],
          [(r.deaths / r.pulls).toFixed(1), 'raid deaths a pull'],
          [r.yourPulls ? (r.yourDeaths / r.yourPulls).toFixed(2) : '–', 'your deaths a pull'],
        ]}
      />
      <div className="cards compact">
        {r.bosses.map((b) => (
          <section key={b.boss + b.difficulty} className="card small">
            <h3>
              {b.boss} <span className="muted">{b.difficulty}</span>
            </h3>
            <p className="small">
              {counted(b.pulls, 'pull')}, {counted(b.kills, 'kill')}
              {b.yourDeaths > 0 && <span className="muted"> · you died {counted(b.yourDeaths, 'time')}</span>}
            </p>
            {b.firstKill ? (
              <button className="link small" onClick={() => onOpen('raid', b.firstKill)}>
                First kill on pull {b.firstKillPull}, {new Date(b.firstKill).toLocaleDateString()}
              </button>
            ) : (
              <p className="small">{b.bestWipePct >= 0 ? `Best wipe ${b.bestWipePct.toFixed(1)}%` : 'No kill'}</p>
            )}
            {b.mostHit && <p className="muted small">Hit hardest by {b.mostHit}</p>}
          </section>
        ))}
      </div>
    </>
  )
}
