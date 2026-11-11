import { useEffect, useState } from 'react'
import type { KeyMeter, KeyMeterRow, KeyTrack, TrackPoint } from './api'
import { valueAt } from './format'

/** A replay's clock: the moment shown (from start), whether it plays and how fast, up to end. */
export function useReplay(end: number, start: number, firstSpeed = 8) {
  const [at, setAt] = useState(start)
  const [playing, setPlaying] = useState(false)
  const [speed, setSpeed] = useState(firstSpeed)
  useEffect(() => {
    if (!playing) return
    const step = 100
    const timer = setInterval(() => setAt((t) => Math.min(t + step * speed, end)), step)
    return () => clearInterval(timer)
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
