import { useEffect, useState } from 'react'
import type { KeyAbility, KeyMeter, KeyMeterRow, KeyTrack, TrackPoint } from './api'
import { valueAt } from './format'

/** A move between two track points faster than this (yards per second) is a jump (a teleport, a release, a portal),
 * drawn as one, not as a walk. */
export const JUMP_YARDS_PER_S = 15

/** Track points further apart than this (ms) aren't walked between: the unit wasn't seen in the gap. */
const TRACK_GAP_MS = 3_000

/** A replay's clock: the moment shown (from start), whether it plays and how fast, up to end. It moves every frame
 * by the real time since the last one, so the map moves smoothly at any speed. */
export function useReplay(end: number, start: number, firstSpeed = 8) {
  const [at, setAt] = useState(start)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(firstSpeed)
  useEffect(() => {
    if (!playing) return
    let frame = 0
    let before = performance.now()
    const tick = (time: number) => {
      const elapsed = Math.max(0, time - before) // the first frame's time can come before performance.now()
      before = time
      setAt((t) => Math.min(t + elapsed * speed, end))
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [playing, speed, end])
  const toggle = () => {
    if (!playing && at >= end) setAt(0)
    setPlaying(!playing)
  }
  return { at, setAt, playing, toggle, speed, setSpeed }
}

/** A mark under the replay's slider: a boss kill or a death. */
export interface SliderMark {
  clockMS: number
  className: string
  title: string
}

/** How far apart a timeline's time ticks are for a replay this long: 15 s up to 4 minutes, 30 s up to 10, else 60. */
export function tickStep(end: number): number {
  if (end <= 240_000) return 15_000
  return end <= 600_000 ? 30_000 : 60_000
}

/** The floors a track's players stood on, most stood on first. */
export function floorsOf(track: KeyTrack | null): number[] {
  const counts = new Map<number, number>()
  Object.values(track?.players ?? {}).forEach((ps) =>
    ps.forEach((p) => counts.set(p.map, (counts.get(p.map) ?? 0) + 1)),
  )
  return [...counts].sort((a, b) => b[1] - a[1]).map(([m]) => m)
}

/** A meter row with nothing in it, for the fields the replay's track doesn't keep. */
const EMPTY_ROW: KeyMeterRow = {
  player: '',
  class: '',
  damage: 0,
  healing: 0,
  dps: 0,
  hps: 0,
  interrupts: 0,
  dispels: 0,
  purges: 0,
  damageTaken: 0,
  avoidable: 0,
  priorityDamage: 0,
}

/** A track's meters between two moments: each player's damage and healing, and per second. */
export function meterBetween(track: KeyTrack, from: number, to: number): KeyMeter {
  return {
    startMS: from,
    endMS: to,
    ongoing: false,
    rows: Object.entries(track.meters ?? {}).map(([player, points]) => {
      const seconds = Math.max(1, (to - from) / 1000)
      const damage = valueAt(points, to, 'd') - valueAt(points, from, 'd')
      const healing = valueAt(points, to, 'h') - valueAt(points, from, 'h')
      const role = { player, class: track.classes[player] ?? '', damage, healing }
      return { ...EMPTY_ROW, ...role, dps: damage / seconds, hps: healing / seconds }
    }),
  }
}

/** A meter with only its top n rows by a column, highest first. */
export function topMeter(meter: KeyMeter, by: 'dps' | 'hps', n: number): KeyMeter {
  return { ...meter, rows: [...meter.rows].sort((a, b) => b[by] - a[by]).slice(0, n) }
}

/** A player's last position at or before a clock reading. */
export function lastAt(points: TrackPoint[], now: number): TrackPoint | undefined {
  let last: TrackPoint | undefined
  for (const p of points) {
    if (p.t > now) break
    last = p
  }
  return last
}

/** Where a unit was at a clock reading: between its points around it, as if it walked straight, unless they're on
 * different maps, far apart in time (TRACK_GAP_MS) or a jump; else its last point. */
export function positionAt(points: TrackPoint[], now: number): TrackPoint | undefined {
  const i = points.findIndex((p) => p.t > now)
  const last = i < 0 ? points[points.length - 1] : points[i - 1]
  const next = i < 0 ? undefined : points[i]
  if (!last || !next || next.map !== last.map || next.t - last.t > TRACK_GAP_MS) return last
  const seconds = Math.max(1, next.t - last.t) / 1000
  if (Math.hypot(next.x - last.x, next.y - last.y) / seconds > JUMP_YARDS_PER_S) return last
  const share = (now - last.t) / (next.t - last.t)
  return { ...last, x: last.x + (next.x - last.x) * share, y: last.y + (next.y - last.y) * share }
}

/** A raider's abilities summed up to a clock reading from a raid pull's track: damage, healing and casts (hits only
 * over the whole pull, so 0), each with its school from the whole pull's; undefined for a track without them. */
export function abilitiesUpTo(
  track: KeyTrack,
  player: string,
  now: number,
  whole: KeyAbility[],
): KeyAbility[] | undefined {
  const series = track.abilities?.[player]
  if (!series) return undefined
  return series.map((s) => {
    let [damage, healing, casts] = [0, 0, 0]
    for (const [t, d, h, c] of s.p) {
      if (t > now) break
      ;[damage, healing, casts] = [damage + d, healing + h, casts + c]
    }
    const school = whole.find((a) => a.player === player && a.ability === s.a)?.school
    return { player, ability: s.a, damage, healing, casts, hits: 0, school }
  })
}
