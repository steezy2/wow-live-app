import { useEffect, useMemo, useState } from 'react'
import {
  createRoute,
  deleteRoute,
  fetchKeyRoute,
  fetchRecordedRoute,
  fetchRoutes,
  saveRoute,
  type KeyTrack,
  type KeyView,
  type RouteCompare,
} from './api'
import { counted } from './format'
import { RouteMap } from './RouteMap'
import { enemyKey, PULL_COLORS, pullMobs, useDungeonMap, useMobs } from './routes'
import { When, type Seeker } from './Seek'
import { useFetched } from './useFetched'

/** Makes a route of the pulls the key made (a new route, the dungeon's starred one when it has none), then reports it. */
function RecordRoute({ keyView: key, onRecorded }: { keyView: KeyView; onRecorded: () => void }) {
  const [state, setState] = useState<'idle' | 'busy' | string>('idle')
  const record = async () => {
    setState('busy')
    try {
      const pulls = await fetchRecordedRoute(key.start)
      if (pulls.length === 0) return setState("No kills on the map to record: read this key's log again.")
      const name = `${key.dungeon} +${key.level} recorded`
      const route = await createRoute(key.mapID, name)
      const colored = pulls.map((enemies, i) => ({ color: PULL_COLORS[i % PULL_COLORS.length], enemies }))
      await saveRoute(route.id, { name, drawings: [], pulls: colored }).catch((err: Error) =>
        deleteRoute(route.id).then(() => Promise.reject(err)),
      ) // no empty route is left behind
      setState(`Recorded as “${name}” (see the Routes tab).`)
      onRecorded()
    } catch (err) {
      setState((err as Error).message)
    }
  }
  return (
    <p>
      <button onClick={record} disabled={state === 'busy' || state.startsWith('Recorded')}>
        Record route
      </button>{' '}
      {state !== 'idle' && state !== 'busy' && <span className="muted">{state}</span>}
    </p>
  )
}

/** A key next to a route of its dungeon (the starred one until another is picked): the route's map with the enemies
 * the key killed ticked, each route pull's planned forces, when the key pulled it and how much of it died, then the
 * kills off the route by key pull and the route pulls with nothing killed. With seek, a time moves the replay. A
 * running key's comparison is read again as its track changes (a null track, not at all). */
export function KeyRoute({ keyView: key, track, seek }: { keyView: KeyView; track: KeyTrack | null; seek?: Seeker }) {
  const [id, setID] = useState<string>()
  const [compare, setCompare] = useState<RouteCompare | null>()
  const [error, setError] = useState('')
  const [fetched, setFetched] = useState(0) // up one when a route is recorded, to read the comparison again
  const routes = useFetched(String(key.mapID), (signal) => fetchRoutes(key.mapID, signal)).data ?? []
  const dungeon = useDungeonMap(key.mapID)
  const mobs = useMobs(key.mapID, key.level, '')
  const byNPC = useMemo(() => new Map(mobs?.mobs.map((m) => [m.npc, m])), [mobs])
  const enemyByKey = useMemo(() => new Map(dungeon?.enemies.map((e) => [enemyKey(e), e])), [dungeon])
  useEffect(() => {
    const controller = new AbortController()
    fetchKeyRoute(key.start, id, controller.signal).then(
      (c) => (setCompare(c), setError('')),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [key.start, id, track, fetched])

  if (error) return <p className="warning">{error}</p>
  if (compare === undefined) return <p className="muted">Loading…</p>
  if (compare === null) {
    return (
      <>
        <p className="muted">
          No route for {key.dungeon} yet: import one from Mythic Dungeon Tools on the Routes tab, or record one from this
          key.
        </p>
        <RecordRoute keyView={key} onRecorded={() => setFetched((n) => n + 1)} />
      </>
    )
  }
  const { route, noMobs } = compare
  const running = key.status === 'running' && !noMobs
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
              {key.status === 'running' ? 'Not pulled yet' : 'Skipped'}: pull{compare.skipped.length > 1 && 's'}{' '}
              {compare.skipped.map((i) => i + 1).join(', ')}
            </p>
          )}
        </>
      )}
    </div>
  )
}
