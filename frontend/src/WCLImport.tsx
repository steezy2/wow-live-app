import { type FormEvent, useEffect, useState } from 'react'
import {
  fetchImportStatus,
  findFights,
  type ImportKind,
  type ImportStatus,
  removeImported,
  savedSettings,
  saveSettings,
  type SettingsView,
  startImport,
  stopImport,
} from './api'
import { importKindText, importProgress, importTotal } from './format'
import { track, useNote } from './note'
import { NoteLine } from './NoteLine'
import { useNow } from './useLiveKey'

/** How often the status is read while an import runs. */
const POLL_MS = 2000

/** Whether an import is still going (listing, fetching or waiting for points). */
const going = (s: ImportStatus) => s.state === 'listing' || s.state === 'running' || s.state === 'waiting'

/** Import from Warcraft Logs: the character (saved with the settings), Find fights (what an import would fetch, by
 * kind, with its points, listed in the background so leaving the page doesn't lose it), the kinds to tick and Import,
 * the progress while it runs, Stop, and Remove imported (which asks first). */
export function WCLImport({ view, onSaved }: { view: SettingsView; onSaved: (view: SettingsView) => void }) {
  const [character, setCharacter] = useState(view.character)
  const [status, setStatus] = useState<ImportStatus | null>(null)
  const [picked, setPicked] = useState<ImportKind[] | null>(null)
  const [removing, setRemoving] = useState(false)
  const [note, setNote] = useNote('warcraft logs import')
  const busy = !!note?.busy
  const now = useNow(60_000)
  const running = status !== null && going(status)
  const plan = !running && status?.plan?.character === view.character ? status.plan : null
  const kinds = picked ?? plan?.kinds.filter((k) => k.default).map((k) => k.kind) ?? []
  useEffect(() => {
    const controller = new AbortController()
    const read = () => fetchImportStatus(controller.signal).then(setStatus, () => undefined)
    void read()
    const timer = running ? setInterval(read, POLL_MS) : undefined
    return () => {
      controller.abort()
      clearInterval(timer)
    }
  }, [running, busy]) // read again once a call left running ends
  const saveCharacter = (e: FormEvent) => {
    e.preventDefault()
    const call = saveSettings({ ...savedSettings(view), character: character.trim() })
    track(call, setNote, 'Checking the realm with Warcraft Logs…', () => 'Saved').then(onSaved, () => undefined)
  }
  const find = () =>
    track(findFights(), setNote, 'Checking the character…', () => '').then(
      (s) => {
        setNote(null)
        setPicked(null)
        setStatus(s)
      },
      () => undefined,
    )
  const toggle = (kind: ImportKind) =>
    setPicked(kinds.includes(kind) ? kinds.filter((k) => k !== kind) : [...kinds, kind])
  const act = (call: Promise<ImportStatus | void>, busy: string, done: string) =>
    track(call, setNote, busy, () => done).then(
      () => {
        setRemoving(false)
        return fetchImportStatus().then(setStatus)
      },
      () => undefined,
    )
  if (!status) return null
  const saved = view.character !== '' && view.character === character.trim()
  return (
    <section className="panel settings">
      <h2>Import from Warcraft Logs</h2>
      <p className="muted">
        Saves the Mythic+ keys and raid boss pulls of this season and raid that a character's Warcraft Logs reports
        have, as if the app had read their logs: also a quick way to see the app on many runs. Keys and pulls you
        have in your own logs are skipped. It spends Warcraft Logs points, so it waits when the hour's points run low:
        Find fights shows what it would fetch and about how many points that takes.
      </p>
      <form className="settings" onSubmit={saveCharacter}>
        <label>
          Character (Name-Realm)
          <input
            value={character}
            placeholder={view.characterGuess ? `${view.characterGuess}-Realm` : 'Name-Realm'}
            onChange={(e) => setCharacter(e.target.value)}
            spellCheck={false}
          />
        </label>
        <div className="actions">
          <button type="submit" disabled={busy || character.trim() === view.character}>
            Save character
          </button>
          {running ? (
            <button type="button" disabled={busy} onClick={() => void act(stopImport(), 'Stopping…', 'Stopped')}>
              Stop
            </button>
          ) : (
            <button
              type="button"
              disabled={busy || !saved}
              title={saved ? "Lists the character's reports: about 2 points a new report" : 'Save the character first'}
              onClick={() => void find()}
            >
              Find fights
            </button>
          )}
          {status.imported > 0 && !removing && (
            <button type="button" className="danger" onClick={() => setRemoving(true)}>
              Remove imported ({status.imported})
            </button>
          )}
        </div>
      </form>
      {plan && (
        <div className="settings">
          {plan.kinds.map((k) => (
            <label key={k.kind} className="inline">
              <input type="checkbox" checked={kinds.includes(k.kind)} onChange={() => toggle(k.kind)} />{' '}
              {importKindText(k)}
            </label>
          ))}
          <div className="actions">
            <span>{importTotal(plan, kinds)}</span>
            <span className="muted">Listing took {Math.round(plan.listedPoints)} points</span>
            <button
              type="button"
              disabled={busy || !plan.kinds.some((k) => kinds.includes(k.kind) && k.fights > 0)}
              onClick={() => void act(startImport(kinds), 'Checking the character…', 'Started')}
            >
              Import
            </button>
          </div>
        </div>
      )}
      {removing && (
        <div className="actions">
          <span>Remove every imported key and pull? Importing them again spends the points again.</span>
          <button
            type="button"
            className="danger"
            disabled={busy}
            onClick={() => void act(removeImported(), 'Removing…', 'Removed')}
          >
            Remove
          </button>
          <button type="button" onClick={() => setRemoving(false)}>
            Keep
          </button>
        </div>
      )}
      {status.state === 'listing' && <div className="busy-bar" role="progressbar" aria-label="Listing the reports" />}
      <p className={status.state === 'failed' ? 'warning' : 'muted'}>{importProgress(status, now)}</p>
      <NoteLine note={note} />
    </section>
  )
}
