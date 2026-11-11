import { useCallback, useEffect, useRef, useState } from 'react'
import type { KeyAbility, KeyMeter, KeyMeterRow, KeyTrack, RaidPull, TrackPoint } from './api'
import { valueAt } from './format'

/** A move between two track points faster than this (yards per second) is a jump (a teleport, a release, a portal),
 * drawn as one, not as a walk. */
export const JUMP_YARDS_PER_S = 15

/** Track points further apart than this (ms) aren't walked between: the unit wasn't seen in the gap. */
const TRACK_GAP_MS = 3_000

/** How often (ms) the page around a playing replay (lanes, graph, meters, cards) follows its clock; the map follows
 * it every frame. */
const PAGE_EVERY_MS = 100

/** A replay's clock: the moment shown (from start), whether it plays and how fast, up to end. Playing, it moves every
 * frame by the real time since the last one: now() is that moment, for the map to draw every frame, and at follows it
 * PAGE_EVERY_MS, so the rest of the page isn't drawn again 60 times a second. */
export function useReplay(end: number, start: number, firstSpeed = 8) {
  const [at, setShown] = useState(start)
  const clock = useRef(start)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(firstSpeed)
  const setAt = useCallback((t: number) => {
    clock.current = t
    setShown(t)
  }, [])
  const now = useCallback(() => clock.current, [])
  useEffect(() => {
    if (!playing) return
    let frame = 0
    let before = performance.now()
    let shown = before
    const tick = (time: number) => {
      // the first frame's time can come before performance.now()
      clock.current = Math.min(clock.current + Math.max(0, time - before) * speed, end)
      before = time
      if (time - shown >= PAGE_EVERY_MS || clock.current >= end) {
        shown = time
        setShown(clock.current)
      }
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [playing, speed, end])
  const toggle = () => {
    if (!playing && clock.current >= end) setAt(0)
    setPlaying(!playing)
  }
  return { at, setAt, now, playing, toggle, speed, setSpeed }
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

/** A raider's pull up to a clock reading, from a raid pull's track: their abilities' damage and healing and what they
 * took damage from (a stretch the moment is in counts for the share of it that has passed, so the numbers grow
 * smoothly), and their casts counted as they finished; hits only over the whole pull, so 0. Schools come from the
 * pull's. Undefined for a track without them. */
export function abilitiesUpTo(
  track: KeyTrack,
  pull: RaidPull,
  player: string,
  now: number,
): { abilities: KeyAbility[]; taken: NonNullable<RaidPull['taken']> } | undefined {
  const series = track.abilities?.[player]
  const ends = track.abilityTimes
  if (!series || !ends) return undefined
  const passed = (end: number) => {
    if (end <= now) return 1
    const from = ends[ends.indexOf(end) - 1] ?? 0 // a stretch starts where the one before it ended
    return Math.max(0, (now - from) / Math.max(1, end - from))
  }
  const casts = new Map<string, number>()
  for (const c of track.casts?.[player] ?? []) {
    if ((c.e ?? c.t) <= now) casts.set(c.s, (casts.get(c.s) ?? 0) + 1)
  }
  const school = (ability: string) =>
    (pull.abilities ?? []).find((a) => a.player === player && a.ability === ability)?.school ??
    (pull.taken ?? []).find((h) => h.player === player && h.ability === ability)?.school
  const sums = new Map<string, [number, number, number]>([...casts.keys()].map((a) => [a, [0, 0, 0]]))
  for (const s of series) {
    const sum = sums.get(s.a) ?? [0, 0, 0]
    for (const [end, d, h, t] of s.p) {
      const share = passed(end)
      if (share === 0) break
      ;[sum[0], sum[1], sum[2]] = [sum[0] + d * share, sum[1] + h * share, sum[2] + t * share]
    }
    sums.set(s.a, sum)
  }
  const abilities: KeyAbility[] = []
  const taken: NonNullable<RaidPull['taken']> = []
  for (const [ability, [damage, healing, tk]] of sums) {
    if (damage || healing || casts.has(ability)) {
      const done = casts.get(ability) ?? 0
      abilities.push({ player, ability, damage, healing, casts: done, hits: 0, school: school(ability) })
    }
    if (tk) taken.push({ player, ability, damage: tk, school: school(ability) })
  }
  return { abilities, taken }
}
