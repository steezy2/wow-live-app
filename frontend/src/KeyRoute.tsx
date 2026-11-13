import { useEffect, useState } from 'react'
import { fetchKeyRoute, fetchRoutes, type KeyTrack, type KeyView, type RouteCompare, type RouteView } from './api'
import { counted } from './format'
import { RouteMap } from './RouteMap'
import { useDungeonMap } from './routes'
import { When, type Seeker } from './Seek'

/** A key next to a route of its dungeon (the starred one until another is picked): the route's map with the enemies
 * the key killed ticked, each route pull's planned forces, when the key pulled it and how much of it died, then the
 * kills off the route by key pull and the route pulls with nothing killed. With seek, a time moves the replay. A
 * running key's comparison is read again as its track changes. */
export function KeyRoute({ keyView: key, track, seek }: { keyView: KeyView; track: KeyTrack | null; seek?: Seeker }) {
  const [routes, setRoutes] = useState<RouteView[]>([])
  const [id, setID] = useState<string>()
  const [compare, setCompare] = useState<RouteCompare | null>()
  const [error, setError] = useState('')
  const dungeon = useDungeonMap(key.mapID)
  useEffect(() => {
    const controller = new AbortController()
    fetchRoutes(key.mapID, controller.signal).then(setRoutes, () => undefined)
    return () => controller.abort()
  }, [key.mapID])
  useEffect(() => {
    const controller = new AbortController()
    fetchKeyRoute(key.start, id, controller.signal).then(
      (c) => (setCompare(c), setError('')),
      (err: Error) => controller.signal.aborted || setError(err.message),
    )
    return () => controller.abort()
  }, [key.start, id, track])

  if (error) return <p className="warning">{error}</p>
  if (compare === undefined) return <p className="muted">Loading…</p>
  if (compare === null) {
    return (
      <p className="muted">No route for {key.dungeon} yet: import one from Mythic Dungeon Tools on the Routes tab.</p>
    )
  }
  const { route, noMobs } = compare
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
      {noMobs && (
        <p className="warning">This key was saved before mob positions were kept; read its log again to compare.</p>
      )}
      <div className="route-view">
        <RouteMap
          mapID={route.mapID}
          dungeon={dungeon}
          pulls={route.pulls}
          killed={noMobs ? undefined : compare.kills}
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
              <li key={i}>
                <span className="route-swatch" style={{ background: `#${p.color}` }}>
                  {i + 1}
                </span>
                <span className="muted">{p.pct.toFixed(1)}%</span>
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
