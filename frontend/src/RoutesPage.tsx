import { useCallback, useEffect, useState } from 'react'
import {
  createRoute,
  deleteRoute,
  exportRoute,
  fetchRoute,
  fetchRoutes,
  fetchSavedRoutes,
  importRoute,
  importSavedRoute,
  starRoute,
  type RouteView,
  type SavedRoute,
} from './api'
import { counted } from './format'
import { RouteMap } from './RouteMap'

/** The share of the dungeon's forces a route needs. */
const FULL_PCT = 100

/** The Routes section: a dungeon's planned routes (the starred one is what the live page will follow) with a map of
 * the picked route's pulls and their forces, a box to import a Mythic Dungeon Tools string, the routes Mythic Dungeon
 * Tools saved and a new empty route. A route copies back as a string for Mythic Dungeon Tools' Import. */
export function RoutesPage() {
  const [routes, setRoutes] = useState<RouteView[]>([])
  const [saves, setSaves] = useState<SavedRoute[]>([])
  const [mapID, setMapID] = useState<number | null>(null)
  const [picked, setPicked] = useState<string | null>(null)
  const [route, setRoute] = useState<RouteView | null>(null)
  const [code, setCode] = useState('')
  const [copied, setCopied] = useState('')
  const [notice, setNotice] = useState('')
  const [error, setError] = useState<string | null>(null)
  const fail = (err: unknown) => setError(err instanceof Error ? err.message : String(err))

  const load = useCallback(
    () => Promise.all([fetchRoutes(), fetchSavedRoutes()]).then(([r, s]) => (setRoutes(r), setSaves(s))),
    [],
  )
  useEffect(() => void load().catch(fail), [load])

  // The dungeons the site tells of: the ones with a route or a saved one (it serves no list of the season's).
  const dungeons = new Map([
    ...routes.map((r) => [r.mapID, `Dungeon ${r.mapID}`] as const),
    ...saves.map((s) => [s.mapID, s.dungeon] as const),
  ])
  const dungeon = mapID !== null && dungeons.has(mapID) ? mapID : ([...dungeons.keys()][0] ?? null)
  const here = routes.filter((r) => r.mapID === dungeon)
  const shown = here.find((r) => r.id === picked) ?? here.find((r) => r.starred) ?? here[0]
  const [shownID, updated] = [shown?.id, shown?.updated] // an import of the same route changes it
  useEffect(() => {
    if (!shownID) return
    const controller = new AbortController()
    fetchRoute(shownID, controller.signal).then(setRoute, () => undefined)
    return () => controller.abort()
  }, [shownID, updated])

  const act = (work: Promise<unknown>) => {
    setError(null)
    setNotice('')
    return work.then(load).catch(fail)
  }
  const imported = (r: RouteView) => {
    setMapID(r.mapID)
    setPicked(r.id)
    if (r.dropped) setNotice(`${counted(r.dropped, 'enemy')} MDT knows aren't on this map.`)
  }
  const add = (work: Promise<RouteView>) => act(work.then(imported))
  const copy = (id: string) =>
    exportRoute(id)
      .then((text) => navigator.clipboard.writeText(text))
      .then(() => setCopied(id))
      .catch(fail)

  return (
    <div className="layout">
      <aside className="side">
        <select value={dungeon ?? ''} onChange={(e) => setMapID(Number(e.target.value))} aria-label="dungeon">
          {[...dungeons].map(([id, name]) => (
            <option key={id} value={id}>
              {name}
            </option>
          ))}
        </select>
        <h2>Routes</h2>
        {here.length === 0 && <p className="muted small">No route for this dungeon yet.</p>}
        {here.map((r) => (
          <div key={r.id} className={r.id === shown?.id ? 'item active' : 'item'}>
            <button className="open" onClick={() => setPicked(r.id)}>
              <span>{r.name}</span>
              <span className="muted small">
                {counted(r.pulls.length, 'pull')} · {r.totalPct.toFixed(1)}%
              </span>
            </button>
            <button
              className="route-star"
              aria-pressed={r.starred}
              disabled={r.starred}
              title={r.starred ? 'The route the live page follows' : 'Make it the route the live page follows'}
              onClick={() => void act(starRoute(r.id))}
            >
              {r.starred ? '★' : '☆'}
            </button>
            <button className="delete" title="Delete route" onClick={() => void act(deleteRoute(r.id))}>
              ×
            </button>
          </div>
        ))}
        {dungeon !== null && (
          <button className="clear" onClick={() => void add(createRoute(dungeon, 'New route'))}>
            New route
          </button>
        )}
        <h2>Import</h2>
        <textarea
          rows={3}
          value={code}
          onChange={(e) => setCode(e.target.value)}
          placeholder="Paste a Mythic Dungeon Tools export string"
          aria-label="export string"
        />
        <button disabled={!code.trim()} onClick={() => void add(importRoute(code.trim())).then(() => setCode(''))}>
          Import string
        </button>
        <h2>From MDT's saves</h2>
        {saves.length === 0 && <p className="muted small">No saved routes found.</p>}
        {saves.map((s) => (
          <div key={s.uid} className="item">
            <span className="open">
              <span>{s.name}</span>
              <span className="muted small">
                {s.dungeon} · {counted(s.pulls, 'pull')}
              </span>
            </span>
            {s.imported ? (
              <span className="muted small">imported</span>
            ) : (
              <button onClick={() => void add(importSavedRoute(s.uid))}>Import</button>
            )}
          </div>
        ))}
        <p className="muted small">MDT writes this file on /reload or logout.</p>
      </aside>
      <main className="page">
        {error && <p className="warning">{error}</p>}
        {notice && <p className="muted">{notice}</p>}
        {route && route.id === shown?.id ? (
          <>
            <div className="title">
              <h1>{route.name}</h1>
              <button onClick={() => void copy(route.id)}>{copied === route.id ? 'Copied' : 'Copy MDT string'}</button>
            </div>
            <div className="route-view">
              <RouteMap mapID={route.mapID} pulls={route.pulls} />
              <ol className="route-pulls">
                <li className="muted small">
                  <span>Pull</span>
                  <span>Forces</span>
                  <span>Of total</span>
                  <span>Running</span>
                </li>
                {route.pulls.map((p, i) => (
                  <li key={i}>
                    <span className="route-swatch" style={{ background: `#${p.color}` }}>
                      {i + 1}
                    </span>
                    <span>{p.forces}</span>
                    <span className="muted">{p.pct.toFixed(1)}%</span>
                    <span className={p.totalPct >= FULL_PCT ? 'good' : undefined}>{p.totalPct.toFixed(1)}%</span>
                  </li>
                ))}
              </ol>
            </div>
          </>
        ) : (
          <p className="muted">Import a route from Mythic Dungeon Tools, or make a new one.</p>
        )}
      </main>
    </div>
  )
}
