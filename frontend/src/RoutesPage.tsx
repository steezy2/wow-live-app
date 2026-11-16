import { useCallback, useEffect, useMemo, useReducer, useState, type MouseEvent } from 'react'
import {
  createRoute,
  deleteRoute,
  exportRoute,
  fetchRoute,
  fetchRouteDungeons,
  fetchRoutes,
  fetchSavedRoutes,
  importRoute,
  importSavedRoute,
  saveRoute,
  starRoute,
  type RouteDungeon,
  type RouteEnemy,
  type RouteView,
  type SavedRoute,
} from './api'
import { amount, counted, errorText } from './format'
import { MobMenu, PullMenu } from './MobMenu'
import { RouteMap } from './RouteMap'
import { tip } from './tipAttrs'
import {
  draftOf,
  edit,
  editorOf,
  enemyKey,
  mobActions,
  packOf,
  patrolOf,
  pullForces,
  pullMobs,
  useDungeonMap,
  useMobs,
} from './routes'

/** The share of the dungeon's forces a route needs. */
const FULL_PCT = 100

/** An open menu: a mob's card or a pull row's, at a place in the window. */
type Menu = { x: number; y: number } & ({ kind: 'mob'; enemy: RouteEnemy } | { kind: 'pull'; pull: number })

/** Where a menu opened by a right click (or the keyboard's menu key, which has no pointer) goes. */
function menuAt(e: MouseEvent<HTMLElement>): { x: number; y: number } {
  if (e.clientX || e.clientY) return { x: e.clientX, y: e.clientY }
  const box = e.currentTarget.getBoundingClientRect()
  return { x: box.left, y: box.bottom }
}

/** A percent with one decimal, or a dash when it can't be known yet (a route never saved with forces). */
const percent = (n: number) => (Number.isFinite(n) ? `${n.toFixed(1)}%` : '–')

/** The Routes section: a dungeon's planned routes (the starred one is what the live page will follow) with a map of
 * the picked route's pulls and their forces, a box to import a Mythic Dungeon Tools string, the routes Mythic Dungeon
 * Tools saved and a new empty route. The picked route is edited in place (pulls picked on the map, added, moved,
 * deleted, renamed, with Undo) and saved; leaving it unsaved asks first. Its mobs are read at a key level the toolbar
 * picks (the owner's newest key there to start): each pull row lists its mobs by name with their health, a click on
 * one or a right click on the map opens the mob's card, and a right click on a row its menu (the hovered row's pull
 * is highlighted on the map). A route copies back as a string for Mythic
 * Dungeon Tools' Import. The page stays mounted while hidden, and reads the routes and MDT's saves again each time it
 * becomes visible. */
export function RoutesPage({ visible, character }: { visible: boolean; character: string }) {
  const [dungeons, setDungeons] = useState<RouteDungeon[]>([])
  const [routes, setRoutes] = useState<RouteView[]>([])
  const [saves, setSaves] = useState<SavedRoute[]>([])
  const [mapID, setMapID] = useState<number | null>(null)
  const [picked, setPicked] = useState<string | null>(null)
  const [route, setRoute] = useState<RouteView | null>(null)
  const [editor, dispatch] = useReducer(edit, { name: '', pulls: [], drawings: [] }, editorOf)
  const [code, setCode] = useState('')
  const [copied, setCopied] = useState('')
  const [notice, setNotice] = useState('')
  const [error, setError] = useState<string | null>(null)
  const fail = (err: unknown) => setError(errorText(err))

  const load = useCallback(
    () => Promise.all([fetchRoutes(), fetchSavedRoutes()]).then(([r, s]) => (setRoutes(r), setSaves(s))),
    [],
  )
  useEffect(() => void fetchRouteDungeons().then(setDungeons).catch(fail), [])
  useEffect(() => {
    if (visible) void load().catch(fail)
  }, [visible, load])

  const dungeon = dungeons.find((d) => d.mapID === mapID)?.mapID ?? dungeons[0]?.mapID ?? null
  const here = routes.filter((r) => r.mapID === dungeon)
  const shown = here.find((r) => r.id === picked) ?? here.find((r) => r.starred) ?? here[0]
  const opened = useCallback((r: RouteView) => {
    setRoute(r)
    dispatch({ kind: 'load', route: r })
  }, [])
  const shownID = shown?.id
  useEffect(() => {
    if (!shownID) return
    const controller = new AbortController()
    fetchRoute(shownID, controller.signal).then(opened, () => undefined)
    return () => controller.abort()
  }, [shownID, opened])
  const map = useDungeonMap(dungeon)
  const [levelPick, setLevelPick] = useState<{ mapID: number; level: number } | null>(null)
  const mobs = useMobs(dungeon, levelPick?.mapID === dungeon ? levelPick.level : null, character)
  const byNPC = useMemo(() => new Map(mobs?.mobs.map((m) => [m.npc, m])), [mobs])
  const enemyByKey = useMemo(() => new Map(map?.enemies.map((e) => [enemyKey(e), e])), [map])
  const [menu, setMenu] = useState<Menu | null>(null)
  const closeMenu = () => setMenu(null)
  const [hovered, setHovered] = useState(-1)

  const { draft, selected } = editor
  const dirty = route !== null && route.id === shownID && JSON.stringify(draft) !== JSON.stringify(draftOf(route))
  useEffect(() => {
    if (!dirty) return
    const stay = (e: BeforeUnloadEvent) => e.preventDefault()
    window.addEventListener('beforeunload', stay)
    return () => window.removeEventListener('beforeunload', stay)
  }, [dirty])
  const leave = (go: () => unknown) => {
    if (!dirty || window.confirm(`Leave ${route?.name} without saving its changes?`)) go()
  }

  const act = (work: Promise<unknown>) => {
    setError(null)
    setNotice('')
    return work.then(load).catch(fail)
  }
  const imported = (r: RouteView) => {
    setMapID(r.mapID)
    setPicked(r.id)
    opened(r) // an import of the shown route replaces it
    const notes = [
      r.dropped ? `${counted(r.dropped, 'enemy')} MDT knows aren't on this map.` : '',
      r.droppedDrawings ? `${counted(r.droppedDrawings, 'drawing')} can't be shown here; the export keeps them.` : '',
    ]
    setNotice(notes.filter(Boolean).join(' '))
  }
  const add = (work: () => Promise<RouteView>) => leave(() => act(work().then(imported)))
  const copy = (id: string) =>
    exportRoute(id)
      .then((text) => navigator.clipboard.writeText(text))
      .then(() => setCopied(id))
      .catch(fail)
  const total = dungeons.find((d) => d.mapID === dungeon)?.forcesTotal
  const rows = dirty ? pullForces(draft.pulls, map?.enemies ?? [], total) : route?.pulls
  const last = rows?.at(-1)

  return (
    <div className="layout">
      <aside className="side">
        <select
          value={dungeon ?? ''}
          onChange={(e) => leave(() => setMapID(Number(e.target.value)))}
          aria-label="dungeon"
        >
          {dungeons.map((d) => (
            <option key={d.mapID} value={d.mapID}>
              {d.name}
            </option>
          ))}
        </select>
        <h2>Routes</h2>
        {here.length === 0 && <p className="muted small">No route for this dungeon yet.</p>}
        {here.map((r) => (
          <div key={r.id} className={r.id === shown?.id ? 'item active' : 'item'}>
            <button className="open" onClick={() => r.id !== shown?.id && leave(() => setPicked(r.id))}>
              <span>{r.name}</span>
              <span className="muted small">
                {counted(r.pulls.length, 'pull')} · {r.totalPct.toFixed(1)}%
              </span>
            </button>
            <button
              className="route-star"
              aria-pressed={r.starred}
              disabled={r.starred}
              {...tip(r.starred ? 'The route the live page follows' : 'Make it the route the live page follows')}
              onClick={() => {
                setPicked(shown?.id ?? null) // the route shown stays shown
                void act(starRoute(r.id))
              }}
            >
              {r.starred ? '★' : '☆'}
            </button>
            <button
              className="delete"
              {...tip('Delete route')}
              onClick={() => {
                const remove = () => act(deleteRoute(r.id))
                if (r.id === shown?.id) leave(remove)
                else void remove()
              }}
            >
              ×
            </button>
          </div>
        ))}
        {dungeon !== null && (
          <button className="clear" onClick={() => add(() => createRoute(dungeon, 'New route'))}>
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
        <button
          disabled={!code.trim()}
          onClick={() => add(() => importRoute(code.trim()).then((r) => (setCode(''), r)))}
        >
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
              <button onClick={() => add(() => importSavedRoute(s.uid))}>Import</button>
            )}
          </div>
        ))}
        <p className="muted small">MDT writes this file on /reload or logout.</p>
      </aside>
      <main className="page">
        {error && <p className="warning">{error}</p>}
        {notice && <p className="muted">{notice}</p>}
        {route && rows && route.id === shown?.id ? (
          <>
            <div className="title">
              <input
                className="route-name"
                value={draft.name}
                onChange={(e) => dispatch({ kind: 'rename', name: e.target.value })}
                aria-label="route name"
              />
              <button
                disabled={!dirty}
                onClick={() => void act(saveRoute(route.id, draft).then(opened))}
              >
                Save
              </button>
              <button disabled={editor.undo.length === 0} onClick={() => dispatch({ kind: 'undo' })}>
                Undo
              </button>
              <button
                disabled={dirty}
                {...tip(dirty ? 'Save first: the string is the saved route' : undefined)}
                onClick={() => void copy(route.id)}
              >
                {copied === route.id ? 'Copied' : 'Copy MDT string'}
              </button>
            </div>
            <div className="route-view">
              <RouteMap
                mapID={route.mapID}
                dungeon={map}
                pulls={draft.pulls}
                selected={selected}
                highlight={hovered}
                onEnemy={(e, shift) =>
                  dispatch({ kind: 'toggle', enemies: shift ? [e] : packOf(e, map?.enemies ?? []) })
                }
                onContext={(enemy, x, y) => setMenu({ kind: 'mob', enemy, x, y })}
                mobs={byNPC}
                level={mobs?.level}
                drawings={draft.drawings}
                onDraw={dispatch}
                limits={route.limits}
              />
              <div>
                <div className="route-tools">
                  {mobs && (
                    <label className="route-level" {...tip('The key level the health and damage are at')}>
                      +
                      <input
                        key={dungeon}
                        type="number"
                        min={mobs.minLevel}
                        max={mobs.maxLevel}
                        defaultValue={mobs.level}
                        aria-label="key level"
                        onChange={(e) =>
                          dungeon !== null &&
                          e.target.value &&
                          e.target.validity.valid &&
                          setLevelPick({ mapID: dungeon, level: e.target.valueAsNumber })
                        }
                      />
                    </label>
                  )}
                  <button onClick={() => dispatch({ kind: 'new' })}>New pull</button>
                  <button disabled={selected < 0} onClick={() => dispatch({ kind: 'delete' })}>
                    Delete pull
                  </button>
                  <button
                    disabled={selected <= 0}
                    onClick={() => dispatch({ kind: 'move', by: -1 })}
                    aria-label="Move pull up"
                  >
                    ↑
                  </button>
                  <button
                    disabled={selected < 0 || selected >= draft.pulls.length - 1}
                    onClick={() => dispatch({ kind: 'move', by: 1 })}
                    aria-label="Move pull down"
                  >
                    ↓
                  </button>
                </div>
                {last && (
                  <p className={last.totalPct >= FULL_PCT ? 'good' : undefined}>
                    Route forces: {percent(last.totalPct)}
                    {total ? ` (${last.total} of ${total})` : ''}
                  </p>
                )}
                <ol className="route-pulls">
                  <li className="muted small">
                    <span>Pull</span>
                    <span>Forces</span>
                    <span>Of total</span>
                    <span>Running</span>
                  </li>
                  {rows.map((p, i) => {
                    const { groups, health } = pullMobs(draft.pulls[i]?.enemies ?? [], enemyByKey, byNPC)
                    return (
                      <li
                        key={i}
                        onMouseEnter={() => setHovered(i)}
                        onMouseLeave={() => setHovered(-1)}
                        onClick={() => dispatch({ kind: 'select', pull: i })}
                        onContextMenu={(e) => {
                          e.preventDefault()
                          dispatch({ kind: 'select', pull: i })
                          setMenu({ kind: 'pull', pull: i, ...menuAt(e) })
                        }}
                      >
                        <button
                          className="route-swatch"
                          style={{ background: `#${p.color}` }}
                          aria-pressed={i === selected}
                        >
                          {i + 1}
                        </button>
                        <span>{p.forces}</span>
                        <span className="muted">{percent(p.pct)}</span>
                        <span className={p.totalPct >= FULL_PCT ? 'good' : undefined}>{percent(p.totalPct)}</span>
                        <span className="route-mobs small">
                          {groups.map((g) => (
                            <button
                              key={g.name}
                              className="link"
                              onClick={(e) => {
                                e.stopPropagation()
                                setMenu({ kind: 'mob', enemy: g.enemies[0], ...menuAt(e) })
                              }}
                            >
                              {g.enemies.length}× {g.name}
                            </button>
                          ))}
                          {health > 0 && <span className="muted">{amount(health)} health</span>}
                        </span>
                      </li>
                    )
                  })}
                </ol>
                {dirty && <p className="muted small">Unsaved: forces summed here, the site counts them on Save.</p>}
                <p className="muted small">
                  Click a pull to select it, right click it to move, delete or recolor it; click a mob for its card.
                  {mobs?.mobs.length === 0 && ' No mob data for this dungeon: names and health need a newer reference.'}
                </p>
              </div>
            </div>
            {menu?.kind === 'mob' && (
              <MobMenu
                enemy={menu.enemy}
                mob={byNPC.get(menu.enemy.npc)}
                level={mobs?.level}
                damageMult={mobs?.damageMult}
                patrol={patrolOf(menu.enemy, map?.patrols ?? [])}
                actions={mobActions(menu.enemy, editor, map?.enemies ?? [])}
                x={menu.x}
                y={menu.y}
                onEdit={dispatch}
                onClose={closeMenu}
              />
            )}
            {menu?.kind === 'pull' && draft.pulls[menu.pull] && (
              <PullMenu
                pull={menu.pull}
                pulls={draft.pulls.length}
                color={draft.pulls[menu.pull].color}
                x={menu.x}
                y={menu.y}
                onEdit={dispatch}
                onClose={closeMenu}
              />
            )}
          </>
        ) : (
          <p className="muted">Import a route from Mythic Dungeon Tools, or make a new one.</p>
        )}
      </main>
    </div>
  )
}
