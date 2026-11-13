import type { KeySummary, RaidNight, SettingsView } from './api'
import { chestLabel, clock, counted, groupBy } from './format'

/** How long the game may go without writing a log before the card says logging may be off. */
export const WAITING_STALE_MS = 30 * 60_000

/** One line of tonight so far: what it was, how it went, and what opens it. */
export interface TonightRow {
  id: string
  label: string
  result: string
  good: boolean
  onOpen: () => void
}

/** What the card says about the combat log: where it watches and when the game last wrote it; warn when the
 * player has something to fix. Null while the settings aren't known (outside the desktop app). */
export function stateLine(view: Pick<SettingsView, 'logsDir' | 'logsDirFound' | 'lastLogMS'> | null, now: number) {
  if (!view) return null
  if (!view.logsDirFound) return { text: `Folder not found: ${view.logsDir}`, warn: true }
  if (!view.lastLogMS) {
    return { text: 'No combat log yet: turn on Advanced Combat Logging and type /combatlog', warn: true }
  }
  const ms = Math.max(0, now - view.lastLogMS)
  const ago = ms < 60_000 ? 'just now' : `${Math.round(ms / 60_000)} min ago`
  const text = `Watching ${view.logsDir} · The game last wrote a log ${ago}`
  return ms > WAITING_STALE_MS ? { text: `${text}. Logging may be off`, warn: true } : { text, warn: false }
}

/** Tonight's raid pulls by boss: a kill, else the best wipe's percentage, each opening the pull that shows it. */
export function raidTonight(nights: RaidNight[], today: string, open: (start: number) => void): TonightRow[] {
  const pulls = nights.find((n) => n.night === today)?.pulls ?? []
  const byBoss = groupBy(pulls, (p) => `${p.boss} ${p.difficulty}`)
  return [...byBoss].map(([boss, group]) => {
    const kill = group.find((p) => p.kill)
    const best = kill ?? group.filter((p) => p.bossPct >= 0).sort((a, b) => a.bossPct - b.bossPct)[0] ?? group[0]
    const result = kill ? 'Kill' : best.bossPct >= 0 ? `best ${Math.round(best.bossPct)}%` : 'wipe'
    return { id: boss, label: boss, result: `${result} · ${counted(group.length, 'pull')}`, good: !!kill, onOpen: () => open(best.start) }
  })
}

/** Today's keys with their level and result, each opening that key. */
export function keysTonight(keys: KeySummary[], midnight: number, open: (start: number) => void): TonightRow[] {
  return keys
    .filter((k) => k.start >= midnight)
    .map((k) => ({
      id: String(k.start),
      label: `${k.dungeon} +${k.level}`,
      result: k.status === 'abandoned' ? 'abandoned' : `${clock(k.finalMS)} ${chestLabel(k.chests)}`,
      good: k.status === 'timed',
      onOpen: () => open(k.start),
    }))
}

/** The local date as the raid nights name it ("2026-11-13"). */
export function localDay(time: number): string {
  const d = new Date(time)
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}
