import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { fetchStorage, readOldKeys, savedSettings, saveSettings, type SettingsView, type Storage } from './api'
import { CombatLogs } from './CombatLogs'
import { sizeText } from './format'
import { track, useNote } from './note'
import { NoteLine } from './NoteLine'
import { Skeleton } from './Skeleton'

/** What each folder the app stores is called. */
const FOLDER_LABELS: Record<Storage['folders'][number]['name'], string> = {
  localKeys: 'Saved keys',
  raidPulls: 'Saved raid pulls',
  wclImport: 'Warcraft Logs imports',
  archive: 'Archived game logs',
  gameLogs: "The game's combat logs",
}

/** The days the Home tab suggests for the two auto rules; shown as hints, never applied. */
const SUGGESTED_DAYS = { archiveGameLogsDays: 30, deleteImportsDays: 90 }

/** The auto rules' fields, label and key. */
const AUTO_FIELDS: [string, keyof SettingsView['auto'], string][] = [
  ['Archive the game\'s logs once read and this many days old', 'archiveGameLogsDays', 'Archiving keeps them readable by Read everything again.'],
  ['Delete Warcraft Logs imports this many days after they were imported', 'deleteImportsDays', 'Importing again costs Warcraft Logs points.'],
]

/** What the app stores: its size by folder, Read old logs, the combat log list (archive and delete) and the two auto
 * rules (days, 0 for off, saved with the settings). Sizes are read again after a read or a list job. */
export function StoragePanel({ view, onSaved }: { view: SettingsView; onSaved: (view: SettingsView) => void }) {
  const [storage, setStorage] = useState<Storage | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [days, setDays] = useState(view.auto)
  const [oldKeys, setOldKeys] = useNote('read logs')
  const [autoNote, setAutoNote] = useNote('auto rules')
  const [stamp, setStamp] = useState(0)
  const changed = useCallback(() => setStamp((n) => n + 1), [])
  useEffect(() => {
    const controller = new AbortController()
    fetchStorage(controller.signal).then(setStorage, (err: unknown) => {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
    })
    return () => controller.abort()
  }, [stamp])
  const readLogs = (all: boolean) =>
    void track(
      readOldKeys(all),
      setOldKeys,
      'Reading the log folder…',
      ({ read, saved }) => `${read} logs read · ${saved} keys saved`,
    ).then(changed, () => undefined)
  const saveAuto = (e: FormEvent) => {
    e.preventDefault()
    const call = saveSettings({ ...savedSettings(view), auto: days })
    track(call, setAutoNote, 'Saving…', () => 'Saved').then(onSaved, () => undefined)
  }
  return (
    <section className="panel home-data">
      <h2>Your data</h2>
      {storage ? (
        <dl className="storage-sizes">
          {storage.folders.map((f) => (
            <div key={f.name}>
              <dt>{FOLDER_LABELS[f.name]}</dt>
              <dd>{sizeText(f.bytes)}</dd>
            </div>
          ))}
        </dl>
      ) : error ? (
        <p className="warning">{error}</p>
      ) : (
        <Skeleton height={96} />
      )}
      <h3>Read old logs</h3>
      <p className="muted">
        Saves every finished key and raid pull in the log folder, so the estimate and the best pulls have your earlier
        runs to go on. Logs already read are skipped unless you read everything again.
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
        Archiving zips a game log into the log folder's WoW Live archive folder: it stops loading, takes about a tenth
        of the space, and Read everything again still reads it. Delete removes archived logs and Warcraft Logs imports
        for good; the saved keys and pulls stay.
      </p>
      <CombatLogs key={view.logsDir} reading={!!oldKeys?.busy} onChanged={changed} />
      <h3>Clean up on its own</h3>
      <form className="settings" onSubmit={saveAuto}>
        {AUTO_FIELDS.map(([label, field, note]) => (
          <label key={field}>
            {label} (0 for off)
            <input
              type="number"
              min={0}
              max={view.maxAutoDays}
              value={days[field]}
              onChange={(e) => setDays({ ...days, [field]: Math.min(view.maxAutoDays, Math.max(0, Math.floor(Number(e.target.value)))) })}
            />
            <span className="muted small">
              Up to {view.maxAutoDays} days; suggested {SUGGESTED_DAYS[field]}. {note}
            </span>
          </label>
        ))}
        <div className="actions">
          <button type="submit" disabled={!!autoNote?.busy || (days.archiveGameLogsDays === view.auto.archiveGameLogsDays && days.deleteImportsDays === view.auto.deleteImportsDays)}>
            Save
          </button>
        </div>
        <NoteLine note={autoNote} />
      </form>
    </section>
  )
}
