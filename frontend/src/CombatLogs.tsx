import { useEffect, useState } from 'react'
import { archiveLogs, deleteArchivedLogs, fetchLogFiles, type LogFile } from './api'
import { counted, sizeText } from './format'
import { type Note, track } from './note'
import { NoteLine } from './NoteLine'

/** A row's key: an archived log can share its name with one in the folder. */
const rowKey = (f: LogFile) => `${f.archived ? 'zip' : 'log'}:${f.name}`

/** The game's combat logs and the archived ones, to pick and archive (zipped into the log folder's WoW Live archive
 * folder, which live reading and Read old logs skip) or, once archived, delete for good. Old, read logs come ticked;
 * the log the game is writing can't be picked. */
export function CombatLogs() {
  const [files, setFiles] = useState<LogFile[] | null>(null)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [note, setNote] = useState<Note>(null)
  const show = (list: LogFile[]) => {
    setFiles(list)
    setPicked(new Set(list.filter((f) => f.suggested).map(rowKey)))
  }
  useEffect(() => {
    const controller = new AbortController()
    fetchLogFiles(controller.signal).then(show, (err: unknown) => {
      if (!controller.signal.aborted) setNote({ error: err instanceof Error ? err.message : String(err) })
    })
    return () => controller.abort()
  }, [])
  if (!files) return <NoteLine note={note} />
  const chosen = (archived: boolean) => files.filter((f) => f.archived === archived && picked.has(rowKey(f)))
  const bytes = (list: LogFile[]) => list.reduce((n, f) => n + f.bytes, 0)
  const [toArchive, toDelete] = [chosen(false), chosen(true)]
  const run = (call: Promise<LogFile[]>, busy: string) =>
    void track(call, setNote, busy, (list) => {
      const failed = list.filter((f) => f.error)
      return failed.length ? `${counted(failed.length, 'log')} failed: ${failed[0].error}` : 'Done'
    }).then(show, () => undefined)
  const archive = () => run(archiveLogs(toArchive.map((f) => f.name)), 'Archiving…')
  const remove = () => {
    const ok = window.confirm(
      `Deletes ${counted(toDelete.length, 'archived log')} (${sizeText(bytes(toDelete))}) for good. ` +
        'Saved keys and pulls stay.',
    )
    if (ok) run(deleteArchivedLogs(toDelete.map((f) => f.name)), 'Deleting…')
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
          <li key={rowKey(f)} title={f.error}>
            <label className="inline">
              <input
                type="checkbox"
                disabled={f.active || !!note?.busy}
                checked={picked.has(rowKey(f))}
                onChange={() => toggle(rowKey(f))}
              />{' '}
              {new Date(f.writtenMS).toLocaleDateString()} · {sizeText(f.bytes)}{' '}
              <span className="muted">
                · {counted(f.keys, 'key')} · {counted(f.pulls, 'pull')}
                {f.archived && ' · Archived'}
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
        <button type="button" disabled={!toArchive.length || !!note?.busy} onClick={archive}>
          Archive selected{toArchive.length > 0 && ` (${sizeText(bytes(toArchive))})`}
        </button>
        <button type="button" disabled={!toDelete.length || !!note?.busy} onClick={remove}>
          Delete selected archives
        </button>
      </div>
      <NoteLine note={note} />
    </>
  )
}
