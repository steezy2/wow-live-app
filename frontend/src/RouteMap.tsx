import { useEffect, useState } from 'react'
import { fetchDungeonMap, type DungeonMap, type KeyTrack, type RoutePull } from './api'
import { MapCanvas, type Overlay } from './KeyMap'
import { enemyKey, pullCentres, pullIndexes } from './routes'

/** A dungeon's map draws no party. */
const NO_TRACK: KeyTrack = { players: {}, classes: {}, deaths: [], meters: {} }
/** What an enemy in no pull is filled with. */
const UNPULLED = '#8a93a6'

/** A dungeon's floors with every enemy a route can pull as a dot (a boss larger): the ones in a pull filled with its
 * color, and the pull's number where its enemies are. A switcher picks the floor. */
export function RouteMap({ mapID, pulls }: { mapID: number; pulls: RoutePull[] }) {
  const [dungeon, setDungeon] = useState<DungeonMap | null>(null)
  const [floor, setFloor] = useState<number | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchDungeonMap(mapID, controller.signal).then(setDungeon, () => undefined)
    return () => controller.abort()
  }, [mapID])

  const floors = dungeon?.floors.filter((f) => dungeon.enemies.some((e) => e.map === f.uiMapID)) ?? []
  const here = floors.find((f) => f.uiMapID === floor) ?? floors[0]
  const overlay: Overlay = (ctx, at, dot) => {
    const enemies = dungeon?.enemies.filter((e) => e.map === here?.uiMapID) ?? []
    const pullOf = pullIndexes(pulls)
    ctx.strokeStyle = '#000'
    ctx.lineWidth = dot / 5
    enemies.forEach((e) => {
      const pull = pullOf.get(enemyKey(e))
      const [x, y] = at(e)
      ctx.fillStyle = pull === undefined ? UNPULLED : `#${pulls[pull].color}`
      ctx.beginPath()
      ctx.arc(x, y, e.boss ? dot * 1.4 : dot * 0.8, 0, 2 * Math.PI)
      ctx.fill()
      ctx.stroke()
    })
    ctx.font = `bold ${dot * 3}px system-ui, sans-serif`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.lineWidth = dot / 2
    pullCentres(enemies, pulls).forEach((c) => {
      const [x, y] = at(c)
      ctx.strokeText(String(c.pull + 1), x, y)
      ctx.fillStyle = '#fff'
      ctx.fillText(String(c.pull + 1), x, y)
    })
  }

  if (!here) return <p className="muted">{dungeon ? 'No map for this dungeon.' : 'Loading…'}</p>
  return (
    <section className="map">
      <div className="map-controls">
        {floors.length > 1 &&
          floors.map((f, i) => (
            <button key={f.uiMapID} aria-pressed={f === here} onClick={() => setFloor(f.uiMapID)}>
              Floor {i + 1}
            </button>
          ))}
      </div>
      <MapCanvas
        key={here.uiMapID}
        track={NO_TRACK}
        backdrop={{ kind: 'tiles', floor: here, mapID }}
        floor={here.uiMapID}
        now={0}
        trailMS={0}
        deaths={[]}
        overlay={overlay}
      />
      <p className="muted small">
        Scroll to zoom, drag to move, double click for the whole floor · map:{' '}
        <a href="https://keystone.guru" target="_blank" rel="noreferrer">
          Keystone.guru
        </a>
      </p>
    </section>
  )
}
