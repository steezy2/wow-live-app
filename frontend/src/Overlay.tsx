import { type CSSProperties, useCallback, useEffect, useState } from 'react'
import { abilityViews, raidTargets } from './abilities'
import { fetchSettings, onAppEvent, type KeyView, type OverlaySettings, type Settings } from './api'
import { classColors } from './classColors'
import { AbilityBars } from './FightMeters'
import { chestLabel, clock, pullTitle, resultText } from './format'
import { ForcesBar, TimerBar } from './KeyDetails'
import { type Column } from './meterColumns'
import { MeterCard } from './MeterCards'
import { ReadyPanel } from './ReadyPanel'
import { RaidOverlayCard } from './RaidPage'
import { abilitiesBetween, useTrack } from './replay'
import { useLiveKey, useOverlayMoving, useRaidLive, useReady } from './useLiveKey'

/** How many of a player's abilities (and enemies) the overlay's breakdown lists. */
const OVERLAY_SPELLS = 8

/** How long into a key the overlay shows who started without a flask, food, rune or party buff (the countdown and
 * a little after, while there's still time to fix it). */
const START_CHECK_MS = 30_000

/** The overlay's two-card columns: DPS (sorted by it), HPS and kicks, for the pull or the whole key. */
const SPLIT_COLUMNS: Column[] = [
  { label: 'DPS', value: (r) => r.dps },
  { label: 'HPS', value: (r) => r.hps },
  { label: 'Kicks', value: (r) => r.interrupts, show: (r) => String(r.interrupts) },
]

/** The overlay's one-card meter: one row per player with the pull's DPS (sorted by it), and the whole key's DPS, HPS
 * and interrupts. */
function overlayColumns(pull: KeyView['lastPull']): Column[] {
  const byPlayer = new Map(pull.rows.map((r) => [r.player, r]))
  return [
    { label: 'Pull DPS', value: (r) => byPlayer.get(r.player)?.dps ?? 0 },
    { label: 'DPS', value: (r) => r.dps },
    { label: 'HPS', value: (r) => r.hps },
    { label: 'Kicks', value: (r) => r.interrupts, show: (r) => String(r.interrupts) },
  ]
}

/** The overlay's settings (background opacity, one card or two), from the app's settings and each save of them. */
function useOverlaySettings(): OverlaySettings {
  const [settings, setSettings] = useState<OverlaySettings>({
    show: 'keys',
    opacity: 100,
    split: false,
    clickThrough: false,
  })
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal).then((view) => view && setSettings(view.overlay), () => undefined)
    const off = onAppEvent('settings', (s) => setSettings((s as Settings).overlay))
    return () => {
      controller.abort()
      off()
    }
  }, [])
  return settings
}

/** The overlay over the game (/overlay, the desktop app's second window): the clock against the timer, forces,
 * the estimate, deaths and the pull's DPS and HPS during a key, and the last raid pull between raid pulls. Everything
 * else stays in the full window; when it's made very short only the clock, forces and deaths stay (index.css). While
 * the app says it's being moved, it shows a border and how to move and resize it. */
export function Overlay() {
  const { key, clockMS } = useLiveKey()
  const moving = useOverlayMoving()
  const settings = useOverlaySettings()
  const ready = useReady()
  const raid = useRaidLive(key?.status !== 'running')
  return (
    <div
      className={moving ? 'overlay moving' : 'overlay'}
      style={{ '--overlay-bg': `${settings.opacity}%` } as CSSProperties}
    >
      {moving && <p className="small">Drag to move, drag an edge to resize; Ctrl+Alt+O or Settings when done.</p>}
      {raid?.betweenPulls && raid.last ? (
        <RaidOverlayCard pull={raid.last} />
      ) : ready && key?.status !== 'running' ? (
        <ReadyPanel ready={ready} compact />
      ) : key ? (
        <OverlayKey keyView={key} clockMS={clockMS} split={settings.split} />
      ) : (
        <p className="muted">Waiting for the next key</p>
      )}
    </div>
  )
}

/** The overlay's view of the live key; a click on a player's row (when the overlay takes clicks) shows their
 * abilities in that card's span: the last pull's until a new pull starts, or the whole key's so far (the one-card
 * meter's rows are the whole key's). */
function OverlayKey({ keyView: key, clockMS, split }: { keyView: KeyView; clockMS: number; split: boolean }) {
  const [picked, setPicked] = useState({ player: '', pull: 0, whole: false })
  const pull = key.lastPull.startMS
  const pickPull = useCallback((player: string) => setPicked({ player, pull, whole: false }), [pull])
  const pickWhole = useCallback((player: string) => setPicked({ player, pull, whole: true }), [pull])
  const back = useCallback(() => setPicked({ player: '', pull: 0, whole: false }), [])
  const focus = picked.whole || picked.pull === pull ? picked.player : ''
  const timer = key.timerMS ?? 0
  const projection = key.status === 'running' ? key.projection : undefined
  const latest = key.deaths.length > 0 ? key.deaths[key.deaths.length - 1] : undefined
  return (
    <>
      <div className="overlay-clock">
        <span className="big">{clock(clockMS)}</span>
        {timer > 0 && <span className="muted"> / {clock(timer)}</span>}{' '}
        {key.status !== 'running' ? (
          <span>{resultText(key.status, key.chests, clockMS, timer)}</span>
        ) : (
          projection && (
            <span className={`chests c${projection.chests}`}>
              {chestLabel(projection.chests)} · {clock(projection.finalMS)}
              {projection.runs > 1 && (
                <span className="muted small">
                  {' '}
                  ({clock(projection.lowMS)}–{clock(projection.highMS)})
                </span>
              )}
            </span>
          )
        )}
      </div>
      {timer > 0 && <TimerBar clockMS={clockMS} timerMS={timer} keyView={key} best={projection?.best ?? []} />}
      {key.forces && <ForcesBar forces={key.forces} level={key.level} />}
      {key.status === 'running' && clockMS < START_CHECK_MS && key.buffs.length > 0 && (
        <p className="warning small overlay-extra">
          Missing: {key.buffs.map((b) => `${b.player} ${b.missing.join(', ')}`).join(' · ')}
        </p>
      )}
      <p className="small">
        {key.deaths.length} deaths · {clock(key.penaltyMS)} lost
        {latest && ` · last: ${latest.player} to ${latest.ability || latest.killer} at ${clock(latest.clockMS)}`}
      </p>
      <div className="overlay-extra">
        {focus ? (
          <Breakdown
            keyView={key}
            player={focus}
            title={picked.whole ? 'Whole key' : pullTitle(key, clockMS)}
            from={picked.whole ? 0 : key.lastPull.startMS}
            to={picked.whole ? clockMS : key.lastPull.endMS}
            onBack={back}
          />
        ) : split ? (
          <>
            <MeterCard title={pullTitle(key, clockMS)} meter={key.lastPull} columns={SPLIT_COLUMNS} onPick={pickPull} />
            <MeterCard title="Whole key" meter={key.overall} columns={SPLIT_COLUMNS} onPick={pickWhole} />
          </>
        ) : (
          <MeterCard
            title={`${pullTitle(key, clockMS)} · whole key`}
            meter={key.overall}
            columns={overlayColumns(key.lastPull)}
            onPick={pickWhole}
          />
        )}
      </div>
    </>
  )
}

/** A player's top abilities from the key's clock from to to (the last pull, or the whole key so far), from the live
 * track's ability stretches: their damage over their damage into each enemy, or their healing when they healed more.
 * A click on the title, a right click or Esc goes back. */
function Breakdown(props: {
  keyView: KeyView
  player: string
  title: string
  from: number
  to: number
  onBack: () => void
}) {
  const { keyView: key, player, title, from, to, onBack } = props
  const track = useTrack(key.start, key.status === 'running')
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onBack()
    window.addEventListener('keydown', esc)
    return () => window.removeEventListener('keydown', esc)
  }, [onBack])
  const between = track ? abilitiesBetween(track, key, player, from, to) : undefined
  const views = between && abilityViews(player, between.abilities, between.taken)
  const sum = (rows: { value: number }[]) => rows.reduce((n, r) => n + r.value, 0)
  const healer = views !== undefined && sum(views.healing) > sum(views.damage)
  const color = classColors[key.overall.rows.find((r) => r.player === player)?.class ?? ''] ?? 'var(--accent)'
  const targets = healer ? [] : raidTargets(between?.targets, player)
  return (
    <div
      className="card focus drill"
      onContextMenu={(e) => {
        e.preventDefault()
        onBack()
      }}
    >
      <h2>
        <button className="back" onClick={onBack}>
          ‹ {title}
        </button>{' '}
        · {player} · {healer ? 'Healing' : 'Damage'}
      </h2>
      {views ? (
        <>
          <AbilityBars rows={healer ? views.healing : views.damage} color={color} limit={OVERLAY_SPELLS} />
          {targets.length > 0 && <AbilityBars rows={targets} color={color} label="Enemy" limit={OVERLAY_SPELLS} />}
        </>
      ) : (
        <p className="muted">{track ? 'No ability timeline for this key' : 'Loading…'}</p>
      )}
    </div>
  )
}
