import { useCallback, useEffect, useRef, useState } from 'react'
import {
  fetchRaidTrack,
  fetchTrack,
  subscribe,
  type KeyAbility,
  type KeyMeter,
  type KeyMeterRow,
  type KeyTrack,
  type RaidPull,
  type TrackPoint,
} from './api'
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
 * PAGE_EVERY_MS, so the rest of the page isn't drawn again 60 times a second. It starts at 1×. */
export function useReplay(end: number, start: number) {
  const [at, setShown] = useState(start)
  const clock = useRef(start)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(1)
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
      if (clock.current >= end) {
        setShown(end)
        setPlaying(false)
        return
      }
      if (time - shown >= PAGE_EVERY_MS) {
        shown = time
        setShown(clock.current)
      }
      frame = requestAnimationFrame(tick)
    }
    frame = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(frame)
  }, [playing, speed, end])
  // A replay left at its end stays there as the end moves on: a key that ends while the page watches lands at its end.
  const last = useRef(end)
  useEffect(() => {
    if (clock.current >= last.current) setAt(end)
    last.current = end
  }, [end, setAt])
  const toggle = () => {
    if (playing) setShown(clock.current) // the page catches up with the map
    else if (clock.current >= end) setAt(0)
    setPlaying(!playing)
  }
  return { at, setAt, now, playing, toggle, speed, setSpeed }
}

/** A key's track (positions and meters over time), read again on each change while the key runs. */
export function useTrack(start: number, live: boolean): KeyTrack | null {
  const [track, setTrack] = useState<KeyTrack | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    const read = () => fetchTrack(start, controller.signal).then(setTrack, () => undefined)
    if (!live) {
      void read()
      return () => controller.abort()
    }
    const unsubscribe = subscribe(() => void read())
    return () => {
      controller.abort()
      unsubscribe()
    }
  }, [start, live])
  return track
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

/** A floor needs this many player points (about 15 s of one player) to be one: the game logs a stray point or two on
 * a map the party never walked (Voidscar Arena's map 0, Temple of Sethraliss's 875). */
export const MIN_FLOOR_POINTS = 15

/** The floors a track's players stood on, most stood on first, without a floor of fewer than MIN_FLOOR_POINTS points
 * (but always the most stood on, so a track just started has one). */
export function floorsOf(track: KeyTrack | null): number[] {
  const counts = new Map<number, number>()
  Object.values(track?.players ?? {}).forEach((ps) =>
    ps.forEach((p) => counts.set(p.map, (counts.get(p.map) ?? 0) + 1)),
  )
  return [...counts]
    .sort((a, b) => b[1] - a[1])
    .filter(([, n], i) => i === 0 || n >= MIN_FLOOR_POINTS)
    .map(([m]) => m)
}

/** An enemy's name by its GUID: a boss's from its points; a mob's (its trail is unnamed) from its death, which is
 * where its trail ends (the nearest, when several died that moment); 'Enemy' for a mob that never died. */
export function enemyName(track: KeyTrack, guid: string): string {
  const boss = track.bosses?.[guid]?.[0]?.name
  const mob = track.mobs?.[guid]
  const end = mob?.p[mob.p.length - 1]
  if (boss || !mob?.died || !end) return boss || 'Enemy'
  const near = (d: TrackPoint) => Math.hypot(d.x - end.x, d.y - end.y)
  const [death] = track.deaths.filter((d) => d.t === mob.died).sort((a, b) => near(a) - near(b))
  return death?.name ?? 'Enemy'
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

/** A raid pull's track, read once: undefined while it loads, null for a pull saved without one. */
export function useRaidTrack(start: number): { track: KeyTrack | null | undefined; error: string | null } {
  const [track, setTrack] = useState<KeyTrack | null | undefined>(undefined)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchRaidTrack(start, controller.signal).then(setTrack, (err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
    })
    return () => controller.abort()
  }, [start])
  return { track, error }
}

/** A track's meters between two moments: each player's damage and healing (and per second), damage taken, and from
 * tracks that keep them (hasCounts) their avoidable damage, damage into priority targets, kicks and dispels. */
export function meterBetween(track: KeyTrack, from: number, to: number): KeyMeter {
  return {
    startMS: from,
    endMS: to,
    ongoing: false,
    rows: Object.entries(track.meters ?? {}).map(([player, points]) => {
      const seconds = Math.max(1, (to - from) / 1000)
      const delta = (kind: Parameters<typeof valueAt>[2]) => valueAt(points, to, kind) - valueAt(points, from, kind)
      const [damage, healing] = [delta('d'), delta('h')]
      return {
        ...EMPTY_ROW,
        player,
        class: track.classes[player] ?? '',
        damage,
        healing,
        dps: damage / seconds,
        hps: healing / seconds,
        damageTaken: delta('tk'),
        avoidable: delta('av'),
        priorityDamage: delta('pd'),
        interrupts: delta('k'),
        dispels: delta('ds'),
      }
    }),
  }
}

/** Whether a track's meters keep avoidable damage, priority damage, kicks and dispels (saved from Nov 11). */
export function hasCounts(track: KeyTrack): boolean {
  return Object.values(track.meters ?? {}).some((ps) =>
    ps.some((p) => p.av !== undefined || p.pd !== undefined || p.k !== undefined || p.ds !== undefined),
  )
}

/** A meter with only its top n rows by a column, highest first. */
export function topMeter(meter: KeyMeter, by: 'dps' | 'hps', n: number): KeyMeter {
  return { ...meter, rows: [...meter.rows].sort((a, b) => b[by] - a[by]).slice(0, n) }
}

/** A boss unit not seen for this long (gone, or submerged) isn't drawn. */
export const BOSS_GONE_MS = 5_000

/** A mob is drawn until it dies, even while stunned or idle; one that never died (a reset, or a key left) isn't
 * drawn this long after it was last seen. */
export const MOB_GONE_MS = 30_000

/** The index of the first point after a clock reading (points.length if none); points are in time order. */
export function indexAfter(points: TrackPoint[], now: number): number {
  let [lo, hi] = [0, points.length]
  while (lo < hi) {
    const mid = (lo + hi) >> 1
    if (points[mid].t > now) hi = mid
    else lo = mid + 1
  }
  return lo
}

/** A player's last position at or before a clock reading. */
export function lastAt(points: TrackPoint[], now: number): TrackPoint | undefined {
  return points[indexAfter(points, now) - 1]
}

/** Where a unit was at a clock reading: between its points around it, as if it walked straight, unless they're on
 * different maps, far apart in time (TRACK_GAP_MS) or a jump; else its last point. */
export function positionAt(points: TrackPoint[], now: number): TrackPoint | undefined {
  const i = indexAfter(points, now)
  const last = points[i - 1]
  const next = points[i]
  if (!last || !next || next.map !== last.map || next.t - last.t > TRACK_GAP_MS) return last
  const seconds = Math.max(1, next.t - last.t) / 1000
  if (Math.hypot(next.x - last.x, next.y - last.y) / seconds > JUMP_YARDS_PER_S) return last
  const share = (now - last.t) / (next.t - last.t)
  return { ...last, x: last.x + (next.x - last.x) * share, y: last.y + (next.y - last.y) * share }
}

/** A raider's pull up to a clock reading: their abilities, what they took damage from and (from tracks that have
 * the series) their damage into each enemy. */
export interface AbilitiesUpTo {
  abilities: KeyAbility[]
  taken: NonNullable<RaidPull['taken']>
  targets?: NonNullable<RaidPull['targets']>
}

type Schools = Map<string, number | undefined>
const schoolCache = new WeakMap<object, { dealt: Schools; taken: Schools }>()

/** A pull's schools by `player|ability`, from its abilities and from what its players took (the first of each wins);
 * built once per pull. */
function schoolsOf(pull: Pick<RaidPull, 'abilities' | 'taken'>) {
  let schools = schoolCache.get(pull)
  if (!schools) {
    const index = (rows: { player: string; ability: string; school?: number }[] | undefined) => {
      const map: Schools = new Map()
      for (const r of rows ?? []) {
        const key = `${r.player}|${r.ability}`
        if (!map.has(key)) map.set(key, r.school)
      }
      return map
    }
    schools = { dealt: index(pull.abilities), taken: index(pull.taken) }
    schoolCache.set(pull, schools)
  }
  return schools
}

/** A raider's pull up to a clock reading, from a raid pull's track: their abilities' damage and healing, what they
 * took damage from and what they hit (a stretch the moment is in counts for the share of it that has passed, so the
 * numbers grow smoothly), and their casts counted as they finished (not stopped ones); hits only over the whole pull,
 * so 0 (hidden). Nothing at 0. Schools come from the pull's. Undefined for a track without the series; targets
 * undefined for one saved before they were kept. */
export function abilitiesUpTo(
  track: KeyTrack,
  pull: Pick<RaidPull, 'abilities' | 'taken'>,
  player: string,
  now: number,
): AbilitiesUpTo | undefined {
  const series = track.abilities?.[player]
  const ends = track.abilityTimes
  if (!series || !ends) return undefined
  const passed = (end: number) => {
    if (end <= now) return 1
    const from = ends[ends.indexOf(end) - 1] ?? 0 // a stretch starts where the one before it ended
    return Math.max(0, (now - from) / Math.max(1, end - from))
  }
  const sumUp = (points: [number, number, number, number][]): [number, number, number] => {
    const sum: [number, number, number] = [0, 0, 0]
    for (const [end, d, h, t] of points) {
      const share = passed(end)
      if (share === 0) break
      ;[sum[0], sum[1], sum[2]] = [sum[0] + d * share, sum[1] + h * share, sum[2] + t * share]
    }
    return sum
  }
  const casts = new Map<string, number>()
  for (const c of track.casts?.[player] ?? []) {
    if (!c.x && (c.e ?? c.t) <= now) casts.set(c.s, (casts.get(c.s) ?? 0) + 1)
  }
  const schools = schoolsOf(pull)
  const school = (ability: string) =>
    schools.dealt.get(`${player}|${ability}`) ?? schools.taken.get(`${player}|${ability}`)
  const sums = new Map<string, [number, number, number]>([...casts.keys()].map((a) => [a, [0, 0, 0]]))
  for (const s of series) {
    const [d, h, t] = sumUp(s.p)
    const [d0, h0, t0] = sums.get(s.a) ?? [0, 0, 0]
    sums.set(s.a, [d0 + d, h0 + h, t0 + t])
  }
  const abilities: KeyAbility[] = []
  const taken: AbilitiesUpTo['taken'] = []
  for (const [ability, [damage, healing, tk]] of sums) {
    if (damage || healing || casts.has(ability)) {
      const done = casts.get(ability) ?? 0
      abilities.push({ player, ability, damage, healing, casts: done, hits: 0, school: school(ability) })
    }
    if (tk) taken.push({ player, ability, damage: tk, school: school(ability) })
  }
  const targets = track.targetSeries?.[player]
    ?.map((s) => ({ player, target: s.a, damage: sumUp(s.p)[0] }))
    .filter((t) => t.damage > 0)
  return { abilities, taken, targets }
}

/** A raider's fight between two clock readings (abilitiesUpTo at to, less at from), dropping what did nothing in it;
 * undefined for a track without the series. */
export function abilitiesBetween(
  track: KeyTrack,
  fight: Pick<RaidPull, 'abilities' | 'taken'>,
  player: string,
  from: number,
  to: number,
): AbilitiesUpTo | undefined {
  const end = abilitiesUpTo(track, fight, player, to)
  if (!end || from <= 0) return end
  const start = abilitiesUpTo(track, fight, player, from)
  const before = <T,>(rows: T[] | undefined, key: (r: T) => string) => new Map((rows ?? []).map((r) => [key(r), r]))
  const done = before(start?.abilities, (a) => a.ability)
  const took = before(start?.taken, (h) => h.ability)
  const hit = before(start?.targets, (t) => t.target)
  return {
    abilities: end.abilities
      .map((a) => {
        const was = done.get(a.ability)
        const damage = a.damage - (was?.damage ?? 0)
        return { ...a, damage, healing: a.healing - (was?.healing ?? 0), casts: a.casts - (was?.casts ?? 0) }
      })
      .filter((a) => a.damage > 0 || a.healing > 0 || a.casts > 0),
    taken: end.taken
      .map((h) => ({ ...h, damage: h.damage - (took.get(h.ability)?.damage ?? 0) }))
      .filter((h) => h.damage > 0),
    targets: end.targets
      ?.map((t) => ({ ...t, damage: t.damage - (hit.get(t.target)?.damage ?? 0) }))
      .filter((t) => t.damage > 0),
  }
}
