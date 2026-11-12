import type { ImportStatus, KeyView, MeterPoint, RaidPullSummary } from './api'

/** A duration as the game shows it: "18:42", "1:05:09" past an hour, "-0:12" when negative. */
export function clock(ms: number): string {
  const sign = ms < 0 ? '-' : ''
  const total = Math.floor(Math.abs(ms) / 1000)
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = String(total % 60).padStart(2, '0')
  return hours > 0 ? `${sign}${hours}:${String(minutes).padStart(2, '0')}:${seconds}` : `${sign}${minutes}:${seconds}`
}

/** How far a pull got: "Kill", "71% P2", "P2" without advanced logging, or "71%" for a boss without phases. */
export function progressText(p: RaidPullSummary): string {
  if (p.kill) return 'Kill'
  const pct = p.bossPct >= 0 ? `${Math.round(p.bossPct)}%` : ''
  return [pct, p.phase].filter(Boolean).join(' ') || 'Wipe'
}

/** A share of the timer as a percentage for the bar, kept between 0 and 100. */
export function share(ms: number, timerMS: number): number {
  if (timerMS <= 0) return 0
  return Math.min(100, Math.max(0, (ms / timerMS) * 100))
}

/** The upgrade a chest count means: "+3", "+2", "+1", or "depleted"; "no timer" without one. */
export function chestLabel(chests: number): string {
  if (chests < 0) return 'no timer'
  return chests === 0 ? 'depleted' : `+${chests}`
}

/** The odds of each result, best first, leaving out the ones no run points at: "+2 35% · +1 55% · depleted 10%". */
export function oddsText(odds: number[]): string {
  return odds
    .map((share, chests) => ({ share, chests }))
    .filter((o) => o.share > 0)
    .reverse()
    .map((o) => `${chestLabel(o.chests)} ${Math.round(o.share * 100)}%`)
    .join(' · ')
}

/** A count with its word: "1 death", "3 deaths". */
export function counted(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`
}

/** A short time in seconds: "6.3 s". */
export function seconds(ms: number): string {
  return `${(ms / 1000).toFixed(1)} s`
}

/** A big number shortened for a meter: "413.0M", "265k", "950". */
export function amount(n: number): string {
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)}M`
  return n >= 1e3 ? `${Math.round(n / 1e3)}k` : String(Math.round(n))
}

/** A finished key's result: "Timed +2 · 21:52 / 32:00 · 10:08 under", "Depleted · …· 2:10 over", "Abandoned at 12:03". */
export function resultText(status: string, chests: number, clockMS: number, timerMS: number): string {
  if (status === 'abandoned') return `Abandoned at ${clock(clockMS)}`
  const head = status === 'timed' ? `Timed${chests > 0 ? ` +${chests}` : ''}` : 'Depleted'
  if (timerMS <= 0) return `${head} · ${clock(clockMS)}`
  const left = timerMS - clockMS
  return `${head} · ${clock(clockMS)} / ${clock(timerMS)} · ${clock(Math.abs(left))} ${left >= 0 ? 'under' : 'over'}`
}

/** What covered a death or a hit: "had Ice Block", "still had Barkskin, Renewal", or "nothing ready". */
export function coverText(defensive: string, ready: string[]): string {
  if (defensive) return `had ${defensive}`
  return ready.length ? `still had ${ready.join(', ')}` : 'nothing ready'
}

/** The pull meter's title: always "Last pull", with how long ago once it's over during a key. */
export function pullTitle(key: KeyView, clockMS: number): string {
  const pull = key.lastPull
  return key.status === 'running' && !pull.ongoing && pull.rows.length > 0
    ? `Last pull · ${clock(clockMS - pull.endMS)} ago`
    : 'Last pull'
}

/** One of a player's numbers so far at a clock reading (damage 'd', healing 'h', damage taken 'tk', and so on), from a
 * track's meter points. */
export function valueAt(points: MeterPoint[], t: number, kind: Exclude<keyof MeterPoint, 't'>): number {
  let value = 0
  for (const p of points) {
    if (p.t > t) break
    value = p[kind] ?? 0
  }
  return value
}

/** The middle value, or the mean of the two middle ones for an even count (0 for none). */
export function median(values: number[]): number {
  const sorted = [...values].sort((a, b) => a - b)
  return sorted.length === 0 ? 0 : (sorted[Math.floor((sorted.length - 1) / 2)] + sorted[Math.floor(sorted.length / 2)]) / 2
}

/** A file size: "40 KB", "820 MB", "1.4 GB". */
export function sizeText(bytes: number): string {
  if (bytes >= 1e9) return `${(bytes / 1e9).toFixed(1)} GB`
  return bytes >= 1e6 ? `${Math.round(bytes / 1e6)} MB` : `${Math.ceil(bytes / 1e3)} KB`
}

/** Items grouped by a key, the groups in the order their keys first came (Map.groupBy, past this build's ES2020). */
export function groupBy<T>(items: T[], key: (item: T) => string): Map<string, T[]> {
  const groups = new Map<string, T[]>()
  items.forEach((item) => groups.get(key(item))?.push(item) ?? groups.set(key(item), [item]))
  return groups
}

const DAY_MS = 86_400_000

/** The side panel's group for a time: its day ("Today", "Yesterday", "Mon, Nov 9") within the last week, else the
 * week it fell in ("Week of Nov 2", weeks starting Monday). */
export function periodLabel(ms: number, now = Date.now()): string {
  const day = new Date(ms)
  day.setHours(0, 0, 0, 0)
  const today = new Date(now)
  today.setHours(0, 0, 0, 0)
  const ago = Math.round((today.getTime() - day.getTime()) / DAY_MS)
  if (ago <= 0) return 'Today'
  if (ago === 1) return 'Yesterday'
  if (ago < 7) return day.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
  day.setDate(day.getDate() - ((day.getDay() + 6) % 7))
  return `Week of ${day.toLocaleDateString([], { month: 'short', day: 'numeric' })}`
}

/** An import's state in a line at now (ms): how far it got, what it saved, the points, and why it stopped or waits. */
export function importProgress(s: ImportStatus, now: number): string {
  if (s.state === 'idle' && s.fights === 0) return ''
  if (s.state === 'listing') return `Listing ${s.character}'s reports…`
  const done = `${s.fightsDone + s.fightsFailed} of ${counted(s.fights, 'fight')}`
  const saved = `${counted(s.keysSaved, 'key')}, ${counted(s.pullsSaved, 'pull')}`
  const points = s.pointsPerFight ? ` · about ${Math.round(s.pointsPerFight)} points a fight` : ''
  const failed = s.fightsFailed ? ` · ${s.fightsFailed} failed (${s.problem})` : ''
  switch (s.state) {
    case 'waiting': {
      const minutes = Math.max(1, Math.round((s.resumeAtMS - now) / 60_000))
      return `${done} · ${saved} · waiting ${minutes} min for the hour's points${failed}`
    }
    case 'failed':
      return `Stopped: ${s.problem}`
    case 'done':
      return `Done: ${saved} imported${points}${failed}`
    default:
      return `${done} · ${saved}${points}${failed}${s.state === 'idle' ? ' · stopped' : ''}`
  }
}
