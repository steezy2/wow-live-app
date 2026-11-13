import { useEffect, useState } from 'react'
import { archiveLogs, deleteLogs, fetchLogFiles, type LogFile, type LogSource } from './api'
import { counted, sizeText } from './format'
import { track, useNote } from './note'
import { NoteLine } from './NoteLine'
import { tip } from './tipAttrs'

/** A row's key: an archived log can share its name with one in the folder, and an import with either. */
const rowKey = (f: LogFile) => `${f.source}:${f.archived ? 'zip' : 'log'}:${f.name}`

/** The game's combat logs, the archived ones and the Warcraft Logs imports, to pick: the game's logs to archive
 * (zipped into the log folder's WoW Live archive folder, which live reading and Read old logs skip), archived logs and
 * imports (already zips) to delete for good. Old, read logs come ticked; the log the game is writing can't be picked.
 * Listed again whenever a read (reading) or this list's own job ends, which also runs onChanged. */
export function CombatLogs({ reading, onChanged }: { reading: boolean; onChanged: () => void }) {
  const [files, setFiles] = useState<LogFile[] | null>(null)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [note, setNote] = useNote('combat logs')
  const busy = reading || !!note?.busy
  const show = (list: LogFile[]) => {
    setFiles(list)
    setPicked(new Set(list.filter((f) => f.suggested).map(rowKey)))
  }
  useEffect(() => {
    if (busy) return
    const controller = new AbortController()
    fetchLogFiles(controller.signal).then(show, (err: unknown) => {
      if (!controller.signal.aborted) setNote({ error: err instanceof Error ? err.message : String(err) })
    })
    return () => controller.abort()
  }, [busy, setNote])
  if (!files) {
    const waiting = reading ? 'Waiting for the read to finish…' : 'Listing the logs…'
    return <NoteLine note={note?.busy || note?.error ? note : { busy: waiting }} />
  }
  const chosen = (source: LogSource, archived: boolean) =>
    files.filter((f) => f.source === source && (source === 'wcl' || f.archived === archived) && picked.has(rowKey(f)))
  const bytes = (list: LogFile[]) => list.reduce((n, f) => n + f.bytes, 0)
  const [toArchive, toDelete, toDeleteImports] = [chosen('game', false), chosen('game', true), chosen('wcl', true)]
  const run = (call: Promise<LogFile[]>, doing: string) =>
    void track(call, setNote, doing, (list) => {
      const failed = list.filter((f) => f.error)
      return failed.length ? `${counted(failed.length, 'log')} failed: ${failed[0].error}` : 'Done'
    }).then(onChanged, () => undefined)
  const archive = () => run(archiveLogs(toArchive.map((f) => f.name)), 'Archiving…')
  const remove = () => {
    const ok = window.confirm(
      `Deletes ${counted(toDelete.length, 'archived log')} (${sizeText(bytes(toDelete))}) for good. ` +
        'Saved keys and pulls stay.',
    )
    if (ok) run(deleteLogs(toDelete.map((f) => f.name)), 'Deleting…')
  }
  const removeImports = () => {
    const ok = window.confirm(
      `Deletes ${counted(toDeleteImports.length, 'Warcraft Logs import')} (${sizeText(bytes(toDeleteImports))}) for ` +
        'good. Saved keys and pulls stay, but importing them again costs Warcraft Logs points.',
    )
    if (ok) run(deleteLogs(toDeleteImports.map((f) => f.name), 'wcl'), 'Deleting…')
  }
  const toggle = (key: string) =>
    setPicked((p) => {
      const next = new Set(p)
      if (!next.delete(key)) next.add(key)
      return next
    })
  return (
    <>
      <ul className="list log-files">
        {files.map((f) => (
          <li key={rowKey(f)} {...tip(f.error)}>
            <label className="inline">
              <input
                type="checkbox"
                disabled={f.active || busy}
                checked={picked.has(rowKey(f))}
                onChange={() => toggle(rowKey(f))}
              />{' '}
              {new Date(f.writtenMS).toLocaleDateString()} · {sizeText(f.bytes)}{' '}
              <span className="muted">
                · {counted(f.keys, 'key')} · {counted(f.pulls, 'pull')}
                {f.source === 'wcl' ? ' · Warcraft Logs import' : f.archived && ' · Archived'}
                {f.active && ' · Being written'}
                {!f.read && ' · Not read yet'}
              </span>
              {f.error && <span className="warning"> · {f.error}</span>}
            </label>
          </li>
        ))}
        {files.length === 0 && <li className="muted">No combat logs in the folder</li>}
      </ul>
      <div className="actions">
        <button type="button" disabled={!toArchive.length || busy} onClick={archive}>
          Archive selected{toArchive.length > 0 && ` (${sizeText(bytes(toArchive))})`}
        </button>
        <button type="button" disabled={!toDelete.length || busy} onClick={remove}>
          Delete selected archives
        </button>
        <button type="button" disabled={!toDeleteImports.length || busy} onClick={removeImports}>
          Delete selected imports
        </button>
      </div>
      <NoteLine note={note} />
    </>
  )
}
