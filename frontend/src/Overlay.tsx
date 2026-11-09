import { type CSSProperties, useEffect, useState } from 'react'
import { fetchSettings, onAppEvent, type KeyView, type OverlaySettings, type Settings } from './api'
import { chestLabel, clock, pullTitle, resultText } from './format'
import { ForcesBar, TimerBar } from './KeyDetails'
import { type Column } from './meterColumns'
import { MeterCard } from './MeterCards'
import { ReadyPanel } from './ReadyPanel'
import { RaidOverlayCard } from './RaidPage'
import { useLiveKey, useOverlayMoving, useRaidLive, useReady } from './useLiveKey'

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
  const [settings, setSettings] = useState<OverlaySettings>({ show: 'keys', opacity: 100, split: false })
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
 * the estimate, deaths and the pull's DPS and HPS during a key, and the last raid pull between raid pulls. Everything else stays in the full window; when it's made very
 * short only the clock, forces and deaths stay (index.css). While the app says it's being moved, it shows a border
 * and how to move and resize it. */
export function Overlay() {
  const { key, clockMS } = useLiveKey()
  const moving = useOverlayMoving()
  const settings = useOverlaySettings()
  const ready = useReady()
  const raid = useRaidLive()
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

/** The overlay's view of the live key. */
function OverlayKey({ keyView: key, clockMS, split }: { keyView: KeyView; clockMS: number; split: boolean }) {
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
      {key.forces && <ForcesBar forces={key.forces} />}
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
        {split ? (
          <>
            <MeterCard title={pullTitle(key, clockMS)} meter={key.lastPull} columns={SPLIT_COLUMNS} />
            <MeterCard title="Whole key" meter={key.overall} columns={SPLIT_COLUMNS} />
          </>
        ) : (
          <MeterCard
            title={`${pullTitle(key, clockMS)} · whole key`}
            meter={key.overall}
            columns={overlayColumns(key.lastPull)}
          />
        )}
      </div>
    </>
  )
}
