import { useState } from 'react'
import type { DungeonMap, EnemyRef, KeyTrack, RouteEnemy, RoutePull } from './api'
import { MapCanvas, type Overlay } from './KeyMap'
import { enemyKey, pullCentres, pullIndexes } from './routes'

/** A dungeon's map draws no party. */
const NO_TRACK: KeyTrack = { players: {}, classes: {}, deaths: [], meters: {} }
/** What an enemy in no pull is filled with. */
const UNPULLED = '#8a93a6'

/** A dungeon's floors with every enemy a route can pull as a dot (a boss larger): the ones in a pull filled with its
 * color (the selected pull's ringed in white), and the pull's number where its enemies are. A switcher picks the
 * floor. With onEnemy, a click on an enemy's dot gives it and whether Shift was held. The killed enemies (a key's)
 * are ticked. */
export function RouteMap(props: {
  mapID: number
  dungeon: DungeonMap | null
  pulls: RoutePull[]
  selected?: number
  onEnemy?: (enemy: RouteEnemy, shift: boolean) => void
  killed?: EnemyRef[]
}) {
  const { mapID, dungeon, pulls, selected, onEnemy, killed } = props
  const [floor, setFloor] = useState<number | null>(null)

  const floors = dungeon?.floors.filter((f) => dungeon.enemies.some((e) => e.map === f.uiMapID)) ?? []
  const here = floors.find((f) => f.uiMapID === floor) ?? floors[0]
  const enemies = dungeon?.enemies.filter((e) => e.map === here?.uiMapID) ?? []
  const overlay: Overlay = (ctx, at, dot, hit) => {
    const pullOf = pullIndexes(pulls)
    ctx.lineWidth = dot / 5
    enemies.forEach((e) => {
      const pull = pullOf.get(enemyKey(e))
      const [x, y] = at(e)
      const r = e.boss ? dot * 1.4 : dot * 0.8
      hit(enemyKey(e), x, y, r)
      ctx.fillStyle = pull === undefined ? UNPULLED : `#${pulls[pull].color}`
      ctx.strokeStyle = pull !== undefined && pull === selected ? '#fff' : '#000'
      ctx.beginPath()
      ctx.arc(x, y, r, 0, 2 * Math.PI)
      ctx.fill()
      ctx.stroke()
    })
    const dead = new Set(killed?.map(enemyKey))
    ctx.strokeStyle = '#000'
    ctx.lineWidth = dot / 3
    enemies
      .filter((e) => dead.has(enemyKey(e)))
      .forEach((e) => {
        const [x, y] = at(e)
        const s = dot * 0.6
        ctx.beginPath()
        ctx.moveTo(x - s, y)
        ctx.lineTo(x - s / 3, y + s * 0.7)
        ctx.lineTo(x + s, y - s * 0.7)
        ctx.stroke()
      })
    ctx.font =`bold ${dot * 3}px system-ui, sans-serif`
    ctx.textAlign = 'center'
    ctx.textBaseline = 'middle'
    ctx.strokeStyle = '#000'
    ctx.lineWidth = dot / 2
    pullCentres(enemies, pulls).forEach((c) => {
      const [x, y] = at(c)
      ctx.strokeText(String(c.pull + 1), x, y)
      ctx.fillStyle = '#fff'
      ctx.fillText(String(c.pull + 1), x, y)
    })
  }
  const pick = (key: string, shift: boolean) => {
    const enemy = enemies.find((e) => enemyKey(e) === key)
    if (enemy) onEnemy?.(enemy, shift)
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
        onPickEnemy={onEnemy && pick}
      />
      <p className="muted small">
        Scroll to zoom, drag to move, double click for the whole floor
        {onEnemy && ' · click an enemy to put it in the selected pull or take it out, Shift+click for its pack'} ·
        map:{' '}
        <a href="https://keystone.guru" target="_blank" rel="noreferrer">
          Keystone.guru
        </a>
      </p>
    </section>
  )
}
