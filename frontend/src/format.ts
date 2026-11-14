import type {
  ImportKind,
  ImportKindPlan,
  ImportPlan,
  ImportStatus,
  KeySpike,
  KeyView,
  MeterPoint,
  RaidPullSummary,
  RouteLive,
} from './api'

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

/** A realm as the log runs it together, spaced out: "MoonGuard" -> "Moon Guard", "Area52" -> "Area 52". */
export function realmName(realm: string): string {
  return realm.replace(/([a-z])([A-Z0-9])/g, '$1 $2')
}

/** A player with their realm: "Testmage · Moon Guard"; the name alone when the realm isn't known. */
export function withRealm(name: string, realm: string): string {
  return realm ? `${name} · ${realmName(realm)}` : name
}

/** A time against another as a signed clock: "+1:05" slower, "-0:30" faster. */
export function versus(ms: number, other: number): string {
  return `${ms > other ? '+' : ''}${clock(ms - other)}`
}

/** A time against the best one, as what leads "your best": "2:00 faster than", "0:30 slower than", "level with"; empty
 * when there is no best. */
export function versusBest(ms: number, bestMS: number): string {
  if (bestMS <= 0) return ''
  if (ms === bestMS) return 'level with'
  return `${clock(Math.abs(ms - bestMS))} ${ms < bestMS ? 'faster' : 'slower'} than`
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

/** What covered a death or a hit: "used" Ice Cold (pressed it), "had" Pain Suppression (put on them), "still had"
 * Barkskin and Renewal, or "nothing ready"; ready is missing in raid pulls saved before it was kept. */
export function coverOf(c: { defensive: string; used?: boolean; ready?: string[] }): { lead: string; spells: string[] } {
  if (c.defensive) return { lead: c.used ? 'used' : 'had', spells: [c.defensive] }
  return c.ready?.length ? { lead: 'still had', spells: c.ready } : { lead: 'nothing ready', spells: [] }
}

/** What hit the party in a spike, with each one's share of it: "Crushing Slam (Bonebreaker) 58%, Volley 22%". */
export function causesText(s: Pick<KeySpike, 'damage' | 'causes'>): string {
  return (s.causes ?? [])
    .map((c) => `${c.ability}${c.source ? ` (${c.source})` : ''} ${Math.round((100 * c.damage) / s.damage)}%`)
    .join(', ')
}

/** The pull meter's title: always "Last pull", with how long ago once it's over during a key. */
export function pullTitle(key: KeyView, clockMS: number): string {
  const pull = key.lastPull
  return key.status === 'running' && !pull.ongoing && pull.rows.length > 0
    ? `Last pull · ${clock(clockMS - pull.endMS)} ago`
    : 'Last pull'
}

/** The live key's place on its route in one line: "Next: pull 7 (12.4%, to 58.1%)", with " · on pull 9" when the
 * party's last kill on the route was past an unfinished pull, or "Route done (102.3%)". */
export function routeLine({ current, next, forcesPct }: RouteLive): string {
  if (!next) return `Route done (${forcesPct.toFixed(1)}%)`
  const past = current && current.number > next.number ? ` · on pull ${current.number}` : ''
  return `Next: pull ${next.number} (${next.pct.toFixed(1)}%, to ${next.totalPct.toFixed(1)}%)${past}`
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
/** Past this many days the side panel groups by month instead of by week, so a season's lists stay short. */
const MONTH_LABEL_DAYS = 56

/** The side panel's group for a time: its day ("Today", "Yesterday", "Mon, Nov 9") within the last week, the week it
 * fell in ("Week of Nov 2", weeks starting Monday) within the last 8, else its month ("September 2026"). */
export function periodLabel(ms: number, now = Date.now()): string {
  const day = new Date(ms)
  day.setHours(0, 0, 0, 0)
  const today = new Date(now)
  today.setHours(0, 0, 0, 0)
  const ago = Math.round((today.getTime() - day.getTime()) / DAY_MS)
  if (ago <= 0) return 'Today'
  if (ago === 1) return 'Yesterday'
  if (ago < 7) return day.toLocaleDateString([], { weekday: 'short', month: 'short', day: 'numeric' })
  if (ago >= MONTH_LABEL_DAYS) return day.toLocaleDateString([], { month: 'long', year: 'numeric' })
  day.setDate(day.getDate() - ((day.getDay() + 6) % 7))
  return `Week of ${day.toLocaleDateString([], { month: 'short', day: 'numeric' })}`
}

/** An import's state in a line at now (ms): how far it got, what it saved, the points, and why it stopped or waits. */
export function importProgress(s: ImportStatus, now: number): string {
  if (s.state === 'idle' && s.fights === 0) return ''
  if (s.state === 'listing') return `Listing ${s.character}'s reports…`
  const done = `${s.fightsDone + s.fightsFailed} of ${counted(s.fights, 'fight')}`
  const saved = `${s.imported} keys and pulls imported`
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
      return `Done: ${saved}${points}${failed}`
    default:
      return `${done} · ${saved}${points}${failed}${s.state === 'idle' ? ' · stopped' : ''}`
  }
}

/** What the page calls a kind of fight to import. */
function importKindLabel(kind: ImportKind): string {
  return { keys: 'Keys', raidKills: 'Raid kills', raidWipes: 'Raid wipes' }[kind]
}

/** An estimate of points, rounded to the nearest 100 from 1,000 and to 10 below: "~2,500 points". */
function approxPoints(points: number): string {
  const step = points >= 1000 ? 100 : 10
  return `~${(Math.round(points / step) * step).toLocaleString('en-US')} points`
}

/** Hours to the nearest half hour, at least half an hour when there are any: "31 h", "2.5 h". */
function approxHours(hours: number): string {
  return `${hours > 0 ? Math.max(0.5, Math.round(hours * 2) / 2) : 0} h`
}

/** A kind's checkbox label, with its hours of fights: "Keys 86 · 31 h · ~2,500 points". */
export function importKindText(k: ImportKindPlan): string {
  return `${importKindLabel(k.kind)} ${k.fights} · ${approxHours(k.minutes / 60)} · ${approxPoints(k.points)}`
}

/** What importing the kinds ticked takes at the plan's points an hour: "Import 112 fights: ~3,300 points, about
 * 1 h". */
export function importTotal(plan: ImportPlan, kinds: ImportKind[]): string {
  const ticked = plan.kinds.filter((k) => kinds.includes(k.kind))
  const fights = ticked.reduce((n, k) => n + k.fights, 0)
  const points = ticked.reduce((n, k) => n + k.points, 0)
  if (fights === 0) return 'Nothing to import'
  const hours = approxHours(points / plan.pointsPerHour)
  return `Import ${counted(fights, 'fight')}: ${approxPoints(points)}, about ${hours}`
}

/** A player review's number with its unit: "1.2M/s", "64%", "3.5 per min", "5 casts". */
export function rowValue(value: number, unit: string): string {
  switch (unit) {
    case '/s':
      return `${amount(value)}/s`
    case '%':
      return `${value.toFixed(0)}%`
    case 'per min':
      return `${value.toFixed(1)} per min`
    case 'casts':
      return `${value.toFixed(0)} ${value === 1 ? 'cast' : 'casts'}`
    default:
      return value.toFixed(0)
  }
}
