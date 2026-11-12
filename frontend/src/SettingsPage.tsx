import { type FormEvent, useEffect, useState } from 'react'
import {
  moveOverlay,
  type OverlaySettings,
  readOldKeys,
  resetOverlay,
  savedSettings,
  saveSettings,
  type Settings,
  type SettingsView,
} from './api'
import { track, useNote } from './note'
import { NoteLine } from './NoteLine'
import { useNow, useOverlayMoving } from './useLiveKey'
import { CombatLogs } from './CombatLogs'
import { WCLImport } from './WCLImport'
import { WCLKeys } from './WCLKeys'

/** What each of the app's overlay choices is called. */
const SHOW_LABELS: Record<OverlaySettings['show'], string> = {
  keys: 'During keys and between raid pulls',
  always: 'Always',
  off: 'Off',
}

/** How long after the last change to the overlay it's saved, so dragging the opacity slider saves once. */
const OVERLAY_SAVE_MS = 300

/** The desktop app's settings in five sections: the game (combat log folder, with when the game last wrote a log
 * there, and region), the overlay (saved as it changes), the Warcraft Logs keys, importing a character's Warcraft
 * Logs reports, and reading and archiving old logs. Shown first when the log folder isn't found. */
export function SettingsPage({ view, onSaved }: { view: SettingsView; onSaved: (view: SettingsView) => void }) {
  const [logsDir, setLogsDir] = useState(view.logsDir)
  const [region, setRegion] = useState(view.region)
  const [overlay, setOverlay] = useState(view.overlay)
  const [game, setGame] = useNote('game')
  const [overlayNote, setOverlayNote] = useNote('overlay')
  const [oldKeys, setOldKeys] = useNote('read logs')
  const moving = useOverlayMoving()
  const now = useNow(60_000)
  useEffect(() => {
    if (
      overlay.show === view.overlay.show &&
      overlay.opacity === view.overlay.opacity &&
      overlay.split === view.overlay.split
    )
      return
    const timer = setTimeout(() => {
      const call = saveSettings({ ...savedSettings(view), overlay })
      track(call, setOverlayNote, 'Saving…', () => 'Saved').then(onSaved, () => undefined)
    }, OVERLAY_SAVE_MS)
    return () => clearTimeout(timer)
  }, [overlay, view, onSaved, setOverlayNote])
  const save = (e: FormEvent) => {
    e.preventDefault()
    const call = saveSettings({ ...savedSettings(view), logsDir: logsDir.trim(), region, overlay })
    track(call, setGame, 'Saving…', () => 'Saved').then(onSaved, () => undefined)
  }
  const readLogs = (all: boolean) =>
    void track(
      readOldKeys(all),
      setOldKeys,
      'Reading the log folder…',
      ({ read, saved }) => `${read} logs read · ${saved} keys saved`,
    ).catch(() => undefined)
  const minutes = view.lastLogMS ? Math.round((now - view.lastLogMS) / 60_000) : null
  return (
    <div className="settings-page">
      <form className="panel settings" onSubmit={save}>
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
      <section className="panel settings">
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
        <label className="inline">
          <input
            type="checkbox"
            checked={overlay.split}
            onChange={(e) => setOverlay({ ...overlay, split: e.target.checked })}
          />{' '}
          The pull and the whole key as two cards (taller)
        </label>
        <div className="actions">
          <button type="button" aria-pressed={moving} onClick={() => moveOverlay(!moving)}>
            {moving ? 'Done moving' : 'Move overlay'}
          </button>
          <button type="button" onClick={resetOverlay}>
            Reset position
          </button>
        </div>
        <p className="muted small">
          {moving
            ? 'Drag the overlay to move it, or an edge to resize it; Ctrl+Alt+O also locks it.'
            : 'Ctrl+Alt+O moves it too, even mid-key.'}
        </p>
        <NoteLine note={overlayNote} />
      </section>
      <WCLKeys />
      <WCLImport view={view} onSaved={onSaved} />
      <section className="panel settings">
        <h2>Your data</h2>
        <p className="muted">
          Saves every finished key and raid pull in the log folder, so the estimate and the best pulls have your
          earlier runs to go on. Logs already read are skipped unless you read everything again.
        </p>
        <div className="actions">
          <button type="button" disabled={!!oldKeys?.busy} onClick={() => readLogs(false)}>
            Read old logs
          </button>
          <button type="button" disabled={!!oldKeys?.busy} onClick={() => readLogs(true)}>
            Read everything again
          </button>
        </div>
        <NoteLine note={oldKeys} />
        <h3>Combat logs</h3>
        <p className="muted">
          Archiving zips a log into the log folder's WoW Live archive folder: it stops loading, takes about a tenth of
          the space, and Read everything again still reads it. Delete removes archived logs for good.
        </p>
        <CombatLogs key={view.logsDir} reading={!!oldKeys?.busy} />
      </section>
    </div>
  )
}
