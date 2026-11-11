// The site's API (backend/controller, spec at /swagger/index.html). This is the only file that knows its URLs.

/** A keystone affix. */
export interface Affix {
  id: number
  name: string
}

/** A player death on the key's clock (penalty included): what killed them, and what covered it or was still ready. */
export interface KeyDeath {
  player: string
  clockMS: number
  killer: string
  ability: string
  defensive: string
  ready: string[]
}

/** A boss fight on the key's clock; endMS is missing while it's on. */
export interface KeyBoss {
  name: string
  startMS: number
  endMS?: number
  kill: boolean
}

/** A player (or their pet) interrupting a mob's spell. */
export interface KeyInterrupt {
  player: string
  /** The player's spell that interrupted ("Wind Shear"); empty in keys saved before Nov 8. */
  kick?: string
  target: string
  spell: string
  clockMS: number
}

/** A defensive, healthstone, health potion or combat potion used, a kick that interrupted nothing, crowd control,
 * or a raid cooldown. */
export interface KeyCast {
  player: string
  ability: string
  kind: 'defensive' | 'healthstone' | 'health potion' | 'combat potion' | 'kick' | 'cc' | 'raid cooldown'
  clockMS: number
  /** A raid cooldown: when its buff last left a raider. */
  endMS?: number
}

/** A must-stop cast and what happened to it: interrupted (by a player, intoMS into the cast), stopped (by a debuff,
 * or "" when it just didn't finish), went off, or the mob died first. */
export interface KeyStop {
  pull: number
  clockMS: number
  mob: string
  spell: string
  outcome: 'interrupted' | 'stopped' | 'went off' | 'died'
  by: string
  intoMS?: number
}

/** A dispellable debuff on a party member: who removed it and after how long, or by "" when it ran out. */
export interface KeyDispel {
  player: string
  spell: string
  by: string
  afterMS: number
  clockMS: number
}

/** A 3 s burst of party damage: the party cooldowns used (overlap: two within 5 s) and each member's cover. */
export interface KeySpike {
  clockMS: number
  damage: number
  partyCDs: string[]
  overlap: boolean
  members: { player: string; taken: number; defensive: string; ready: string[] }[]
}

/** One player's numbers over a stretch of the key: damage and healing done, interrupts, dispels (off the party) and
 * purges (off enemies), damage taken and the avoidable part of it. */
export interface KeyMeterRow {
  player: string
  /** "Mage", "DeathKnight"; "" when unknown. */
  class: string
  damage: number
  healing: number
  dps: number
  hps: number
  interrupts: number
  dispels: number
  purges: number
  damageTaken: number
  avoidable: number
  /** Damage into each trash pull's minibosses. */
  priorityDamage: number
}

/** A trash pull with minibosses (twice the pull's median health, or its one big mob): the share of damage into them
 * in its first 10 s and in all, their share of the pack's health, where each fell in the kill order (order of of; 0 if
 * it didn't die), and whether one died after most of the pack. */
export interface KeyPull {
  pull: number
  clockMS: number
  openingPct: number
  priorityPct: number
  healthPct: number
  minibosses: { name: string; diedMS: number; order: number; of: number }[]
  late: boolean
}

/** Each player's numbers over a stretch of the key (a pull, or all of it), highest damage first. */
export interface KeyMeter {
  startMS: number
  endMS: number
  /** The pull is still being fought. */
  ongoing: boolean
  rows: KeyMeterRow[]
}

/** What a player lacked at the key's start. */
export interface KeyBuffCheck {
  player: string
  missing: string[]
}

/** A boss in your best timed run at this level: when you killed it then, and the latest you can kill it now to time
 * the key if the rest goes as it did then. */
export interface KeyGhost {
  name: string
  killMS: number
  needByMS: number
}

/** The expected final time from earlier runs of the dungeon: the median and 20th to 80th percentile, the upgrade the
 * median gives, the share of runs pointing at each result (index 0 depleted to 3), and the best run boss by boss. */
export interface KeyProjection {
  finalMS: number
  lowMS: number
  highMS: number
  chests: number
  odds: number[]
  runs: number
  /** "3 of your runs, forces and bosses". */
  source: string
  best: KeyGhost[]
}

/** The key's enemy forces: the share killed (it can pass 100), each kill's share, where earlier runs stood at this
 * clock (0 without any), and whether a finished key fell short enough that the dungeon file needs checking. */
export interface KeyForces {
  /** The count killed of the dungeon's total (0 in keys saved before Nov 8). */
  count: number
  total: number
  pct: number
  at: { clockMS: number; pct: number }[]
  referencePct: number
  needsCheck: boolean
  /** Enemies that died without being in the dungeon file (missing in keys saved before Nov 8). */
  uncounted?: { npc: number; name: string; deaths: number }[]
}

/** A Mythic+ key as the page shows it (models.KeyView in the Go). */
export interface KeyView {
  dungeon: string
  mapID: number
  level: number
  affixes: Affix[]
  start: number
  end?: number
  status: 'running' | 'timed' | 'depleted' | 'abandoned'
  finalMS?: number
  timerMS?: number
  deaths: KeyDeath[]
  bosses: KeyBoss[]
  players: string[]
  interrupts: KeyInterrupt[]
  casts: KeyCast[]
  stops: KeyStop[]
  wastedKicks: KeyCast[]
  debuffs: KeyDispel[]
  spikes: KeySpike[]
  pulls: KeyPull[]
  /** The season's dungeon file has this dungeon (avoidable list, forces). */
  reference: boolean
  /** Missing when the dungeon file doesn't have the dungeon. */
  forces?: KeyForces
  lastPull: KeyMeter
  overall: KeyMeter
  buffs: KeyBuffCheck[]
  clockMS: number
  /** Time the deaths added to the clock. */
  penaltyMS: number
  plus2MS: number
  plus3MS: number
  /** Most chests still in reach: 3, 2, 1, or 0 once over time; -1 without a timer. */
  chests: number
  projection?: KeyProjection
  /** How long ago a running key's last line was logged (0 once it's over): the game writes its log in batches as
   * fights start and end, so a long silence is a long fight, or a party that left. */
  silentMS: number
  /** Each player's abilities, most damage and healing first, and their damage taken by ability: there once the key is
   * over, and missing in keys saved before Nov 10. */
  abilities?: KeyAbility[]
  taken?: { player: string; ability: string; damage: number; school?: number }[]
  /** The party member whose game wrote the log; missing in keys saved before Nov 11. */
  logger?: string
}

/** One ability in a player's key (models.KeyAbility): its damage and healing as the meters count them (pets under
 * their owner), how often it was cast, and how often it hit or healed. */
export interface KeyAbility {
  player: string
  ability: string
  damage: number
  healing: number
  casts: number
  hits: number
  /** Its spell school as the combat log gives it, a bit per school (schoolColors.ts); missing before Nov 11. */
  school?: number
}

/** A place at a time on the key's clock: x and y in yards as the combat log gives them (x north, y west), the map
 * (floor) ID, and for a mob's death, the mob. */
export interface TrackPoint {
  t: number
  x: number
  y: number
  map: number
  name?: string
}

/** A player's damage, healing and damage taken so far at a time on the key's clock (tk is missing in keys saved
 * before Nov 8). */
export interface MeterPoint {
  t: number
  d: number
  h: number
  tk?: number
}

/** Where a key's party (or a raid pull's raid) went, each member about once a second with their class, where its mobs
 * died, and the meters over time. */
export interface KeyTrack {
  players: Record<string, TrackPoint[]>
  classes: Record<string, string>
  deaths: TrackPoint[]
  /** Each player's damage and healing so far, about every 2 s. */
  meters: Record<string, MeterPoint[]>
  /** Each pull's start and end on the key's clock (end 0 while fought); missing in keys saved before Nov 8. */
  pulls?: { startMS: number; endMS: number }[]
  /** A raid boss's units by GUID, about once a second, each point named; none in a key. */
  bosses?: Record<string, TrackPoint[]>
}

/** One number of a player's run: the median of their spec's top runs (0 without), their usual here (-1 without), and
 * their rank in the party (1 is best). */
export interface ReviewMetric {
  name: string
  value: number
  top: number
  usual: number
  partyRank: number
  /** Shown shortened ("2.1M"); missing in reviews made before Nov 8. */
  amount?: boolean
  /** What the metric counts (missing in reviews made before Nov 8). */
  help?: string
  /** DPS and HPS: the key's damage or healing in all, and what it is ("damage", "healing"). */
  total?: number
  totalName?: string
}

/** A finished key's review: each player's numbers, up to four things to work on and one done well. */
export interface KeyReview {
  start: number
  players: {
    player: string
    /** The spec and class, as the page shows them ("Blood Death Knight"). */
    spec: string
    metrics: ReviewMetric[]
    advice: string[]
    positive: string
  }[]
  party: string
  /** The party's must-stop casts and time between pulls (missing in reviews made before Nov 8). */
  notes?: string[]
  baseline: string
  points: number
}

/** A dungeon floor with Keystone.guru tiles: the combat log's map ID and its in-game bounds (Keystone.guru's x is the
 * log's -y, its y the log's x). */
export interface DungeonFloor {
  uiMapID: number
  minX: number
  maxX: number
  minY: number
  maxY: number
}

/** What the replay draws a dungeon on. */
export interface DungeonMap {
  floors: DungeonFloor[]
  walked: TrackPoint[]
}

/** A saved key in the list. */
export interface KeySummary {
  start: number
  dungeon: string
  level: number
  status: KeyView['status']
  finalMS: number
  /** The upgrade it gave: 3, 2, 1; 0 when depleted or abandoned. */
  chests: number
}

async function call(path: string, init?: RequestInit): Promise<Response> {
  const response = await fetch(path, init)
  if (!response.ok) throw await failure(response)
  return response
}

/** The site's own error message for a failed answer, or its status. */
async function failure(response: Response): Promise<Error> {
  const body = (await response.json().catch(() => null)) as { error?: string } | null
  return new Error(body?.error ?? `the site answered ${response.status}`)
}

/** The running key, or the last one that ended; null before any key is in the log. */
export async function fetchLiveKey(signal?: AbortSignal): Promise<KeyView | null> {
  const response = await call('/api/mplus/live', { signal })
  return response.status === 204 ? null : ((await response.json()) as KeyView)
}

/** Ends the running key as abandoned, for a party that left before the game's log says so; it runs again if the log
 * goes on in it. */
export async function abandonLiveKey(): Promise<void> {
  await call('/api/mplus/live/abandon', { method: 'POST' })
}

/** The ready check before the next key (models.ReadyCheck): each member's missing flask, food and party buffs, and
 * those not seen since the party zoned in. */
export interface ReadyCheck {
  sinceMS: number
  dungeon: string
  players: { player: string; missing: string[]; notSeen: string[] }[]
}

/** The ready check, or null outside a Mythic dungeon between keys. */
export async function fetchReady(signal?: AbortSignal): Promise<ReadyCheck | null> {
  const response = await call('/api/mplus/ready', { signal })
  return response.status === 204 ? null : ((await response.json()) as ReadyCheck)
}

/** The saved keys, newest first. */
export async function fetchSavedKeys(signal?: AbortSignal): Promise<KeySummary[]> {
  return (await (await call('/api/mplus/keys', { signal })).json()) as KeySummary[]
}

/** One saved key. */
export async function fetchSavedKey(start: number, signal?: AbortSignal): Promise<KeyView> {
  return (await (await call(`/api/mplus/keys/${start}`, { signal })).json()) as KeyView
}

/** Where a key's party went and its mobs died: the running key's, the last one's or a saved one's. */
export async function fetchTrack(start: number, signal?: AbortSignal): Promise<KeyTrack> {
  return (await (await call(`/api/mplus/keys/${start}/track`, { signal })).json()) as KeyTrack
}

/** A dungeon's map: its floors with Keystone.guru tiles, and where parties have walked in its saved runs. */
export async function fetchDungeonMap(mapID: number, signal?: AbortSignal): Promise<DungeonMap> {
  return (await (await call(`/api/mplus/dungeons/${mapID}/map`, { signal })).json()) as DungeonMap
}

/** One 384 by 256 map tile of a dungeon floor; a floor is TILES_PER_SIDE tiles wide and tall. */
export function tileURL(mapID: number, uiMapID: number, x: number, y: number): string {
  return `/api/mplus/dungeons/${mapID}/floors/${uiMapID}/tiles/${x}/${y}`
}
export const TILES_PER_SIDE = 4

/** A finished key's review, or null when none was made yet. */
export async function fetchReview(start: number, signal?: AbortSignal): Promise<KeyReview | null> {
  const response = await fetch(`/api/mplus/keys/${start}/review`, { signal })
  if (response.status === 404) return null
  if (!response.ok) throw await failure(response)
  return (await response.json()) as KeyReview
}

/** Reviews a finished key against Warcraft Logs (spends points); the kept review when there is one. */
export async function makeReview(start: number): Promise<KeyReview> {
  return (await (await call(`/api/mplus/keys/${start}/review`, { method: 'POST' })).json()) as KeyReview
}

/** Deletes one saved key, or every saved key without a start. */
export async function deleteSavedKeys(start?: number): Promise<void> {
  await call(start ? `/api/mplus/keys/${start}` : '/api/mplus/keys', {
    method: 'DELETE',
  })
}

/** How often the page reads the keys again while the site's event stream is down. */
export const FALLBACK_POLL_MS = 2000

/** The desktop app serves the page itself under this name and pushes changes with Wails events, since its asset
 * server can't stream. */
const APP_HOST = 'wails.localhost'

/** The part of the Wails runtime (served by the app at /wails/runtime.js) the page uses. */
interface WailsRuntime {
  Events: {
    On(name: string, callback: (event: { data: unknown }) => void): () => void
    Emit(name: string, data?: unknown): Promise<boolean>
  }
}

/** The Wails runtime, loaded from the app (only call it in the app). */
function appRuntime() {
  const runtime = '/wails/runtime.js'
  return import(/* @vite-ignore */ runtime) as Promise<WailsRuntime>
}

/** Calls callback with the data of each Wails event called name, in the desktop app; nothing in a browser. Returns
 * the unsubscribe. */
export function onAppEvent(name: string, callback: (data: unknown) => void): () => void {
  if (location.host !== APP_HOST) return () => undefined
  let off: (() => void) | undefined
  let stopped = false
  void appRuntime().then((wails) => {
    if (!stopped) off = wails.Events.On(name, (event) => callback(event.data))
  })
  return () => {
    stopped = true
    off?.()
  }
}

/** Calls onChange once now and whenever the site says the keys changed (and in a browser when the stream reopens). In the desktop app the notices are Wails
 * events; in a browser, the site's event stream, polling every FALLBACK_POLL_MS while it's down (the browser keeps
 * retrying it). Returns the unsubscribe. */
export function subscribe(onChange: () => void): () => void {
  if (location.host === APP_HOST) {
    const off = onAppEvent('change', onChange)
    onChange()
    return off
  }
  let timer: ReturnType<typeof setInterval> | undefined
  const stopPolling = () => {
    clearInterval(timer)
    timer = undefined
  }
  const source = new EventSource('/api/mplus/events')
  source.addEventListener('change', onChange)
  source.onopen = () => {
    stopPolling()
    onChange()
  }
  source.onerror = () => {
    timer ??= setInterval(onChange, FALLBACK_POLL_MS)
  }
  onChange()
  return () => {
    source.close()
    stopPolling()
  }
}

/** Sends the desktop app a Wails event; nothing in a browser. */
function emitAppEvent(name: string, data?: unknown): void {
  if (location.host === APP_HOST) void appRuntime().then((wails) => wails.Events.Emit(name, data))
}

/** Tells the desktop app to move the overlay (on: it takes clicks to be dragged and resized), or without on to say
 * again whether it's moving (the 'overlay-moving' event); resetOverlay puts it back in its first place. */
export function moveOverlay(on?: boolean): void {
  emitAppEvent('overlay-move', on)
}
export function resetOverlay(): void {
  emitAppEvent('overlay-reset')
}

/** The desktop app's settings, with whether the log folder exists, when the game last wrote a log there (0 for
 * never) and the choices to pick from; null outside the app. The app sends the saved settings as the 'settings'
 * event. */
export interface Settings {
  logsDir: string
  region: 'us' | 'eu' | 'kr' | 'tw'
  overlay: OverlaySettings
}
export interface OverlaySettings {
  show: 'keys' | 'always' | 'off'
  opacity: number
  /** The pull and the whole key as two cards, one above the other, instead of one. */
  split: boolean
}
export interface SettingsView extends Settings {
  logsDirFound: boolean
  lastLogMS: number
  regions: Settings['region'][]
  overlayShows: OverlaySettings['show'][]
  minOpacity: number
}

export async function fetchSettings(signal?: AbortSignal): Promise<SettingsView | null> {
  const response = await fetch('/api/settings', { signal })
  if (response.status === 404) return null
  if (!response.ok) throw await failure(response)
  return (await response.json()) as SettingsView
}

/** Saves the settings; the app reads the new log folder and changes the overlay right away. */
export async function saveSettings(settings: Settings): Promise<SettingsView> {
  const init = {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  }
  return (await (await call('/api/settings', init)).json()) as SettingsView
}

/** What the app shows about the saved Warcraft Logs keys (never the secret) and their last test. */
export interface KeyStatus {
  set: boolean
  idEnd: string
  testedMS: number
  works: boolean
  pointsLeft: number
  limitPerHour: number
  problem: string
}

export async function fetchKeyStatus(signal?: AbortSignal): Promise<KeyStatus> {
  return (await (await call('/api/settings/warcraftlogs', { signal })).json()) as KeyStatus
}

/** Tests and saves a Warcraft Logs client ID and secret; with force, saves them even when the test fails. */
export async function saveKeys(id: string, secret: string, force: boolean): Promise<KeyStatus> {
  const init = {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id, secret }),
  }
  return (await (await call(`/api/settings/warcraftlogs${force ? '?force=true' : ''}`, init)).json()) as KeyStatus
}

export async function testKeys(): Promise<KeyStatus> {
  return (await (await call('/api/settings/warcraftlogs/test', { method: 'POST' })).json()) as KeyStatus
}

export async function removeKeys(): Promise<void> {
  await call('/api/settings/warcraftlogs', { method: 'DELETE' })
}

/** Reads every combat log in the log folder and saves each finished key; says how many keys are saved. */
export async function readOldKeys(): Promise<number> {
  return ((await (await call('/api/settings/read-keys', { method: 'POST' })).json()) as { saved: number }).saved
}

/** A raid pull in a list (models.RaidPullSummary): when, which boss, its number tonight, and how far it got (bossPct
 * -1 without advanced logging; phase "" for a boss without phase triggers). */
export interface RaidPullSummary {
  start: number
  night: string
  boss: string
  difficulty: 'LFR' | 'Normal' | 'Heroic' | 'Mythic'
  number: number
  durationMS: number
  kill: boolean
  bossPct: number
  phase: string
}

/** A raider's death in a raid pull: the M+ death with whether it was early (before the wipe) and their last hits. */
export interface RaidDeath extends KeyDeath {
  early: boolean
  recap: { ability: string; source: string; amount: number; beforeMS: number }[]
}

/** A number for one raider. */
export interface PlayerCount {
  player: string
  count: number
}

/** One raid pull as the overview shows it (models.RaidPull). */
export interface RaidPull extends RaidPullSummary {
  end: number
  players: string[]
  deaths: RaidDeath[]
  massKills: { ability: string; killed: number; clockMS: number }[]
  avoidable: { ability: string; players: PlayerCount[] }[]
  soaks: { ability: string; casts: number; fewest: number; most: number; died: string[] }[]
  stacks: { ability: string; limit: number; players: PlayerCount[] }[]
  dispels: KeyDispel[]
  wentOff: KeyStop[]
  adds: { name: string; spawned: number; killed: number; aliveEnd: number; longestMS: number }[]
  priority: { target: string; damage: number }[]
  meter: KeyMeter
  lusts: number[]
  cooldowns: KeyCast[]
  comparisons?: { tonight?: RaidPullSummary; ever?: RaidPullSummary }
  /** The boss's rooms for the replay. */
  maps: RaidMap[]
  lanes: RaidLane[]
  spike?: { clockMS: number; perSecond: number; covered: string[] }
  /** What to fix on the next pull, most important first. */
  suggestions: string[]
  /** Each raider's abilities, their damage taken by ability and their damage into each enemy, most first: missing in
   * pulls saved before Nov 11. */
  abilities?: KeyAbility[]
  taken?: { player: string; ability: string; damage: number }[]
  targets?: { player: string; target: string; damage: number }[]
}

/** A raid room's map (the combat log's uiMapID) and its edges in the log's yards: x runs north (maxX at the top), y
 * west (maxY at the left). */
export interface RaidMap {
  uiMapID: number
  minX: number
  maxX: number
  minY: number
  maxY: number
}

/** One row of a pull's timeline: a mechanic (or phases, deaths, Bloodlust, cooldowns, an add kind) and each time it
 * came up; a mark with endMS is a stretch (a phase, an add's life, how long a buff lasted). */
export interface RaidLane {
  name: string
  kind: 'phases' | 'deaths' | 'lust' | 'cooldowns' | 'potions' | 'spike' | 'avoidable' | 'soak' | 'stacks' | 'adds'
  marks: { clockMS: number; endMS?: number; text: string; bad?: boolean }[]
}

/** The raid now: a pull on (its boss and start), the last pull, and whether the overlay shows it. */
export interface RaidLive {
  pulling: boolean
  betweenPulls: boolean
  boss?: string
  since?: number
  last?: RaidPull
}

/** One raid night's pulls, newest first. */
export interface RaidNight {
  night: string
  pulls: RaidPullSummary[]
}

/** The raid now. */
export async function fetchRaidLive(signal?: AbortSignal): Promise<RaidLive> {
  return (await (await call('/api/raid/live', { signal })).json()) as RaidLive
}

/** Every raid night's pulls, newest first. */
export async function fetchRaidNights(signal?: AbortSignal): Promise<RaidNight[]> {
  return (await (await call('/api/raid/nights', { signal })).json()) as RaidNight[]
}

/** One raid pull. */
export async function fetchRaidPull(start: number, signal?: AbortSignal): Promise<RaidPull> {
  return (await (await call(`/api/raid/pulls/${start}`, { signal })).json()) as RaidPull
}

/** One raid pull's replay track; null for a pull saved before replays were kept. */
export async function fetchRaidTrack(start: number, signal?: AbortSignal): Promise<KeyTrack | null> {
  const response = await fetch(`/api/raid/pulls/${start}/track`, { signal })
  if (response.status === 404) return null
  if (!response.ok) throw await failure(response)
  return (await response.json()) as KeyTrack
}

/** A raid room's map image. */
export function raidMapURL(uiMapID: number): string {
  return `/api/raid/maps/${uiMapID}`
}
