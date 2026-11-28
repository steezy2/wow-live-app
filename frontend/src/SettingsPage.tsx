import { type FormEvent, useEffect, useState } from 'react'
import {
  moveOverlay,
  type OverlayPreview,
  type OverlaySettings,
  type RankBy,
  previewOverlay,
  resetOverlay,
  savedSettings,
  saveSettings,
  type Settings,
  type SettingsView,
} from './api'
import { track, useNote } from './note'
import { NoteLine } from './NoteLine'
import { OVERLAY_LABELS } from './meterColumns'
import { useNow, useOverlayMoving, useOverlayPreview } from './useLiveKey'
import { WCLImport } from './WCLImport'
import { WCLKeys } from './WCLKeys'

/** What each of the app's overlay choices is called. */
const SHOW_LABELS: Record<OverlaySettings['show'], string> = {
  keys: 'During keys and between raid pulls',
  always: 'Always',
  off: 'Off',
}

/** The seconds a death's recap can be set to, within what the API allows. */
const RECAP_CHOICES = [5, 10, 20, 30, 60]

/** The overlay's previews with dummy data and their buttons' labels. */
const PREVIEWS: [OverlayPreview, string][] = [
  ['mplus', 'Preview Mythic+'],
  ['raid', 'Preview raid'],
]

/** How long after the last change to the overlay it's saved, so dragging the opacity slider saves once. */
const OVERLAY_SAVE_MS = 300

/** The desktop app's settings in five panels: the game (combat log folder, with when the game last wrote a log
 * there, and region), the overlay (saved as it changes), the Warcraft Logs keys, importing a character's Warcraft
 * Logs reports, and a pointer to Home, where the stored data and the setup guide (shown again when onHome gets true)
 * are. */
export function SettingsPage({
  view,
  onSaved,
  onHome,
}: {
  view: SettingsView
  onSaved: (view: SettingsView) => void
  onHome: (guide: boolean) => void
}) {
  const [logsDir, setLogsDir] = useState(view.logsDir)
  const [region, setRegion] = useState(view.region)
  const [overlay, setOverlay] = useState(view.overlay)
  const [game, setGame] = useNote('game')
  const [overlayNote, setOverlayNote] = useNote('overlay')
  const moving = useOverlayMoving()
  const previewing = useOverlayPreview()
  const now = useNow(60_000)
  useEffect(() => {
    const edited = { ...overlay, clickThrough: view.overlay.clickThrough }
    if (JSON.stringify(edited) === JSON.stringify(view.overlay)) return
    const timer = setTimeout(() => {
      const call = saveSettings({ ...savedSettings(view), overlay: edited })
      track(call, setOverlayNote, 'Saving…', () => 'Saved').then(onSaved, () => undefined)
    }, OVERLAY_SAVE_MS)
    return () => clearTimeout(timer)
  }, [overlay, view, onSaved, setOverlayNote])
  const columns = view.overlayColumns.filter((id) => overlay.oneCard && !overlay.splitCards || id !== 'pullDps')
  const allowed = RECAP_CHOICES.filter((n) => n >= view.minRecapSeconds && n <= view.maxRecapSeconds)
  const recapSeconds = [...new Set([0, ...allowed, overlay.deathRecapSeconds])].sort((a, b) => a - b)
  const save = (e: FormEvent) => {
    e.preventDefault()
    const call = saveSettings({ ...savedSettings(view), logsDir: logsDir.trim(), region, overlay })
    track(call, setGame, 'Saving…', () => 'Saved').then(onSaved, () => undefined)
  }
  const minutes = view.lastLogMS ? Math.round((now - view.lastLogMS) / 60_000) : null
  return (
    <div className="settings-page">
      <form id="settings-game" className="panel settings" onSubmit={save}>
        <h2>Game</h2>
        <label>
          Combat log folder
          <input value={logsDir} onChange={(e) => setLogsDir(e.target.value)} spellCheck={false} />
        </label>
        <p className={view.logsDirFound ? 'muted' : 'warning'}>
          {!view.logsDirFound
            ? 'Folder not found: it is Logs, inside _retail_, where the game is installed.'
            : minutes !== null
              ? `The game last wrote a combat log here ${minutes} min ago.`
              : 'No combat log here yet: turn on Advanced Combat Logging and type /combatlog in game.'}
        </p>
        <label>
          Region, for affix names (from the next start)
          <select value={region} onChange={(e) => setRegion(e.target.value as Settings['region'])}>
            {view.regions.map((r) => (
              <option key={r} value={r}>
                {r.toUpperCase()}
              </option>
            ))}
          </select>
        </label>
        <div className="actions">
          <button type="submit" disabled={!!game?.busy}>
            Save
          </button>
        </div>
        <NoteLine note={game} />
      </form>
      <section id="settings-overlay" className="panel settings">
        <h2>Overlay</h2>
        <p className="muted small">Saved as you change it.</p>
        <label>
          Show the overlay
          <select
            value={overlay.show}
            onChange={(e) => setOverlay({ ...overlay, show: e.target.value as OverlaySettings['show'] })}
          >
            {view.overlayShows.map((s) => (
              <option key={s} value={s}>
                {SHOW_LABELS[s]}
              </option>
            ))}
          </select>
        </label>
        <label>
          Background opacity {overlay.opacity}%
          <input
            type="range"
            min={view.minOpacity}
            max={100}
            value={overlay.opacity}
            onChange={(e) => setOverlay({ ...overlay, opacity: Number(e.target.value) })}
          />
        </label>
        <label>
          Layout
          <select
            value=""
            onChange={({ target }) => {
              const preset = view.overlayPresets.find((p) => p.name === target.value)!
              setOverlay({
                ...overlay,
                oneCard: preset.oneCard,
                splitCards: preset.splitCards,
                raidOneCard: preset.raidOneCard,
                rows: preset.rows,
                columns: preset.columns,
              })
            }}
          >
            <option value="" disabled>
              Pick a layout to fill in the options below…
            </option>
            {view.overlayPresets.map((p) => (
              <option key={p.name}>{p.name}</option>
            ))}
          </select>
        </label>
        <label>
          Rank players and fill the bars by
          <select
            value={overlay.rankBy}
            onChange={(e) => setOverlay({ ...overlay, rankBy: e.target.value as RankBy })}
          >
            {view.overlayRanks.map((r) => (
              <option key={r} value={r}>
                {r.toUpperCase()}
              </option>
            ))}
          </select>
        </label>
        <label className="inline">
          <input
            type="checkbox"
            checked={overlay.splitCards}
            onChange={(e) => setOverlay({ ...overlay, splitCards: e.target.checked })}
          />{' '}
          Mythic+: a DPS card and an HPS card (healers only) of the key
        </label>
        <label className="inline">
          <input
            type="checkbox"
            checked={overlay.oneCard}
            disabled={overlay.splitCards}
            onChange={(e) => setOverlay({ ...overlay, oneCard: e.target.checked })}
          />{' '}
          Mythic+: one card for the pull and the whole key
        </label>
        <label>
          Raid: players on the Damage card (0 for everyone)
          <input
            type="number"
            min={0}
            value={overlay.raidDpsRows}
            onChange={(e) => setOverlay({ ...overlay, raidDpsRows: Math.max(0, Math.floor(Number(e.target.value))) })}
          />
        </label>
        <label>
          Raid: players on the Healing card (0 for everyone)
          <input
            type="number"
            min={0}
            value={overlay.raidHpsRows}
            onChange={(e) => setOverlay({ ...overlay, raidHpsRows: Math.max(0, Math.floor(Number(e.target.value))) })}
          />
        </label>
        <label className="inline">
          <input
            type="checkbox"
            checked={overlay.raidSideBySide}
            onChange={(e) => setOverlay({ ...overlay, raidSideBySide: e.target.checked })}
          />{' '}
          Raid: the Damage and Healing cards side by side
        </label>
        <label className="inline">
          <input
            type="checkbox"
            checked={overlay.raidOneCard}
            onChange={(e) => setOverlay({ ...overlay, raidOneCard: e.target.checked })}
          />{' '}
          Raid: one ranked card instead of Damage and Healing cards
        </label>
        <label>
          Mythic+: show a new death's recap for (seconds)
          <select
            value={overlay.deathRecapSeconds}
            onChange={(e) => setOverlay({ ...overlay, deathRecapSeconds: Number(e.target.value) })}
          >
            {recapSeconds.map((n) => (
              <option key={n} value={n}>
                {n === 0 ? 'Off' : `${n} s`}
              </option>
            ))}
          </select>
        </label>
        <label className="inline">
          <input
            type="checkbox"
            checked={overlay.chatButtons}
            onChange={(e) => setOverlay({ ...overlay, chatButtons: e.target.checked })}
          />{' '}
          Mythic+: Copy DPS / HPS for chat buttons once a key ends (needs clicks on the overlay)
        </label>
        <label>
          Players per card (0 for everyone)
          <input
            type="number"
            min={0}
            value={overlay.rows}
            onChange={(e) => setOverlay({ ...overlay, rows: Math.max(0, Math.floor(Number(e.target.value))) })}
          />
        </label>
        <fieldset>
          <legend>Columns</legend>
          {columns.map((id) => (
            <label key={id} className="inline">
              <input
                type="checkbox"
                checked={overlay.columns.includes(id)}
                onChange={({ target: { checked } }) =>
                  setOverlay({
                    ...overlay,
                    columns: view.overlayColumns.filter((c) => (c === id ? checked : overlay.columns.includes(c))),
                  })
                }
              />{' '}
              {OVERLAY_LABELS[id]}
            </label>
          ))}
        </fieldset>
        <label className="inline">
          <input
            type="checkbox"
            checked={!view.overlay.clickThrough}
            onChange={(e) => {
              const next = { ...view.overlay, clickThrough: !e.target.checked }
              const call = saveSettings({ ...savedSettings(view), overlay: next })
              track(call, setOverlayNote, 'Saving…', () => 'Saved').then(onSaved, () => undefined)
            }}
          />{' '}
          The overlay takes clicks (Ctrl+Alt+K): a click on a player shows their spells in the pull, but the game
          gets no clicks where the overlay is
        </label>
        <div className="actions">
          <button type="button" aria-pressed={moving} onClick={() => moveOverlay(!moving)}>
            {moving ? 'Done moving' : 'Move overlay'}
          </button>
          <button type="button" onClick={resetOverlay}>
            Reset position
          </button>
          {PREVIEWS.map(([kind, label]) => (
            <button
              key={kind}
              type="button"
              aria-pressed={previewing === kind}
              onClick={() => previewOverlay(previewing === kind ? '' : kind)}
            >
              {previewing === kind ? 'Stop preview' : label}
            </button>
          ))}
        </div>
        <p className="muted small">
          {moving
            ? 'Drag the overlay to move it, or a corner to resize it; Ctrl+Alt+O also locks it.'
            : 'Ctrl+Alt+O moves it too, even mid-key. Ctrl+Alt+H hides it until the next key or pull.'}
        </p>
        <NoteLine note={overlayNote} />
      </section>
      <WCLKeys />
      <WCLImport view={view} onSaved={onSaved} />
      <section className="panel settings">
        <h2>Home</h2>
        <p className="muted">Storage, reading old logs and archiving are on Home.</p>
        <div className="actions">
          <button type="button" onClick={() => onHome(false)}>
            Open Home
          </button>
          <button type="button" onClick={() => onHome(true)}>
            Show the setup guide again
          </button>
        </div>
      </section>
    </div>
  )
}
