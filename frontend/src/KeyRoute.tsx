import { useEffect, useMemo, useState } from 'react'
import {
  fetchKeyRoute,
  fetchRoutes,
  type KeyView,
  type RouteCompare,
  type RunPull,
} from './api'
import { counted } from './format'
import { RouteMap } from './RouteMap'
import { enemyKey, PULL_COLORS, pullMobs, recordKeyRoute, useDungeonMap, useMobs } from './routes'
import { When, type Seeker } from './Seek'
import { useFetched } from './useFetched'

/** How a pull of the key sits against the plan, in words. */
function runLabel(p: RunPull): string {
  if (p.state === 'off') return 'Not in the route'
  if (p.state === 'matches') return `Plan pull ${p.plan + 1}`
  const differences = [p.extra && `${p.extra} extra`, p.missing && `${p.missing} not killed`].filter(Boolean)
  return `Plan pull ${p.plan + 1}: ${differences.join(', ')}`
}

/** Makes a route of the pulls the key made (a new route, the dungeon's starred one when it has none), then reports
 * it. */
function RecordRoute({ keyView: key, onRecorded }: { keyView: KeyView; onRecorded: () => void }) {
  const [state, setState] = useState<'idle' | 'busy' | string>('idle')
  const record = async () => {
    setState('busy')
    try {
      const { recorded, message } = await recordKeyRoute(key)
      setState(message)
      if (recorded) onRecorded()
    } catch (err) {
      setState((err as Error).message)
    }
  }
  return (
    <p>
      <button className="action" onClick={record} disabled={state === 'busy' || state.startsWith('Recorded')}>
        Record route
      </button>{' '}
      {state !== 'idle' && state !== 'busy' && <span className="muted">{state}</span>}
    </p>
  )
}

/** A key next to a route of its dungeon (the starred one until another is picked). A finished key shows its own pulls
 * on the map over the plan's faint outlines, each against the route pull it is closest to (matches, differs by some
 * enemies, or not in the route), then the planned enemies the log never showed dying. A running key shows the route's
 * map with the enemies killed ticked, each route pull's planned forces, when it was pulled and how much of it died,
 * then the kills off the route and the route pulls not pulled. With seek, a time moves the replay. A running key's
 * comparison is read again as the key changes while live is set. */
export function KeyRoute({ keyView: key, live, seek }: { keyView: KeyView; live: boolean; seek?: Seeker }) {
  const [id, setID] = useState<string>()
  const [compare, setCompare] = useState<RouteCompare | null>()
  const [error, setError] = useState('')
  const [fetched, setFetched] = useState(0) // up one when a route is recorded, to read the comparison again
  const routes = useFetched(String(key.mapID), (signal) => fetchRoutes(key.mapID, signal)).data ?? []
  const dungeon = useDungeonMap(key.mapID)
  const mobs = useMobs(key.mapID, key.level, '')
  const byNPC = useMemo(() => new Map(mobs?.mobs.map((m) => [m.npc, m])), [mobs])
  const mine = useMemo(
    () =>
      (compare?.run.pulls ?? []).map((p, i) => ({ color: PULL_COLORS[i % PULL_COLORS.length], enemies: p.enemies })),
    [compare],
  )
  const enemyByKey = useMemo(() => new Map(dungeon?.enemies.map((e) => [enemyKey(e), e])), [dungeon])
  const liveKey = live ? key : null // a new reading of a live key asks again
  useEffect(() => {
    const controller = new AbortController()
    fetchKeyRoute(key.start, id, controller.signal).then(
      (c) => (setCompare(c), setError('')),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [key.start, id, fetched, liveKey])

  if (error) return <p className="warning">{error}</p>
  if (compare === undefined) return <p className="muted">Loading…</p>
  if (compare === null) {
    return (
      <>
        <p className="muted">
          No route for {key.dungeon} yet: import one from Mythic Dungeon Tools on the Routes tab, or record one from
          this key.
        </p>
        <RecordRoute keyView={key} onRecorded={() => setFetched((n) => n + 1)} />
      </>
    )
  }
  const { route, noMobs } = compare
  const running = key.status === 'running' && !noMobs
  const showRun = !running && !noMobs
  return (
    <div>
      <p>
        {routes.length > 1 ? (
          <select value={route.id} onChange={(e) => setID(e.target.value)} aria-label="route">
            {routes.map((r) => (
              <option key={r.id} value={r.id}>
                {r.starred ? '★ ' : ''}
                {r.name}
              </option>
            ))}
          </select>
        ) : (
          route.name
        )}{' '}
        <span className="muted">
          · {counted(route.pulls.length, 'pull')} · {route.totalPct.toFixed(1)}% planned
        </span>
      </p>
      {!noMobs && <RecordRoute keyView={key} onRecorded={() => setFetched((n) => n + 1)} />}
      {noMobs && (
        <p className="warning">This key was saved before mob positions were kept; read its log again to compare.</p>
      )}
      {route.totalPct < 100 && (
        <p className="warning">
          This route plans only {route.totalPct.toFixed(1)}% forces: you cannot time the key on it.
        </p>
      )}
      {running && compare.behind && (
        <p className="warning">
          Forces {compare.behind.actualPct.toFixed(1)}% vs {compare.behind.plannedPct.toFixed(1)}% planned.
        </p>
      )}
      {running &&
        compare.missed.map((m) => (
          <p key={m.pull} className="warning">
            Pull {m.pull + 1} missed ({m.pct.toFixed(1)}%):{' '}
            {pullMobs(m.enemies, enemyByKey, byNPC)
              .groups.map((g) => `${g.enemies.length}× ${g.name}`)
              .join(', ')}
          </p>
        ))}
      {showRun ? (
        <>
          <div className="route-view">
            <RouteMap
              mapID={route.mapID}
              dungeon={dungeon}
              pulls={mine}
              ghost={route.pulls}
              drawings={route.drawings}
            />
            <ol className="route-pulls">
              <li className="muted small">
                <span>Pull</span>
                <span>Forces</span>
                <span>Pulled at</span>
                <span>Against the plan</span>
              </li>
              {compare.run.pulls.map((p, i) => (
                <li key={p.keyPull}>
                  <span className="route-swatch" style={{ background: `#${mine[i].color}` }}>
                    {i + 1}
                  </span>
                  <span className="muted">{p.pct.toFixed(1)}%</span>
                  <span>
                    <When ms={p.clockMS} seek={seek} />
                  </span>
                  <span className={p.state === 'matches' ? 'good' : undefined}>{runLabel(p)}</span>
                </li>
              ))}
            </ol>
          </div>
          <h3>Planned, not seen in the log</h3>
          <ul className="list">
            {compare.run.unseen.map((m) => (
              <li key={m.pull}>
                <span>
                  Plan pull {m.pull + 1}:{' '}
                  {pullMobs(m.enemies, enemyByKey, byNPC)
                    .groups.map((g) => `${g.enemies.length}× ${g.name}`)
                    .join(', ')}
                </span>
                <span className="muted">{m.pct.toFixed(1)}%</span>
              </li>
            ))}
            {compare.run.unseen.length === 0 && <li className="muted">None</li>}
          </ul>
          <p className="muted small">The log can fail to show a mob dying, so these may have been killed.</p>
        </>
      ) : (
        <>
          <div className="route-view">
            <RouteMap
              mapID={route.mapID}
              dungeon={dungeon}
              pulls={route.pulls}
              drawings={route.drawings}
              killed={noMobs ? undefined : compare.kills}
              states={running ? compare.pulls.map((p) => p.state) : undefined}
            />
            <ol className="route-pulls">
              <li className="muted small">
                <span>Pull</span>
                <span>Planned</span>
                <span>Pulled at</span>
                <span>Killed</span>
              </li>
              {route.pulls.map((p, i) => {
                const done = noMobs ? undefined : compare.pulls[i]
                return (
                  <li key={i} className={running && done?.state === 'missed' ? 'warning' : undefined}>
                    <span className="route-swatch" style={{ background: `#${p.color}` }}>
                      {i + 1}
                    </span>
                    <span className="muted">
                      {p.pct.toFixed(1)}%{running && done?.state === 'next' && ' · next'}
                    </span>
                    <span>{done && done.keyPull >= 0 && <When ms={done.clockMS} seek={seek} />}</span>
                    <span className={done?.done ? 'good' : undefined}>{done && `${done.killed}/${done.of}`}</span>
                  </li>
                )
              })}
            </ol>
          </div>
          {!noMobs && (
            <>
              <h3>Off the route</h3>
              <ul className="list">
                {compare.off.map((o) => (
                  <li key={o.keyPull}>
                    <span>
                      {o.keyPull >= 0 ? `Pull ${o.keyPull + 1}` : 'Before the first pull'}:{' '}
                      {counted(o.enemies.length + o.offMap, 'kill')}
                      {o.offMap > 0 && <span className="muted"> ({o.offMap} not on the map)</span>}
                    </span>
                    <span className="muted">
                      {o.pct.toFixed(1)}% · <When ms={o.clockMS} seek={seek} />
                    </span>
                  </li>
                ))}
                {compare.off.length === 0 && <li className="muted">None</li>}
              </ul>
              {compare.skipped.length > 0 && (
                <p className="muted">
                  {key.status === 'running' ? 'Not pulled yet' : 'Skipped'}: pull
                  {compare.skipped.length > 1 && 's'} {compare.skipped.map((i) => i + 1).join(', ')}
                </p>
              )}
            </>
          )}
        </>
      )}
    </div>
  )
}
