import { useEffect, useState } from 'react'
import { archiveLogs, deleteLogs, fetchLogFiles, type LogFile, type LogSource } from './api'
import { counted, errorText, groupLogsByDay, sizeText } from './format'
import { track, useNote } from './note'
import { useNow } from './useLiveKey'
import { NoteLine } from './NoteLine'
import { tip } from './tipAttrs'

/** A row's key: an archived log can share its name with one in the folder, and an import with either. */
const rowKey = (f: LogFile) => `${f.source}:${f.archived ? 'zip' : 'log'}:${f.name}`

/** The game's combat logs, the archived ones and the Warcraft Logs imports, to pick: the game's logs to archive
 * (zipped into the log folder's WoW Live archive folder, which live reading and Read old logs skip), archived logs and
 * imports (already zips) to delete for good. Logs are listed by the day they were written (the last 5 days one by
 * one, older ones together); a group's box picks all of its logs. Old, read logs come ticked; the log the game is
 * writing can't be picked.
 * Listed again whenever a read (reading) or this list's own job ends, which also runs onChanged. */
export function CombatLogs({ reading, onChanged }: { reading: boolean; onChanged: () => void }) {
  const [files, setFiles] = useState<LogFile[] | null>(null)
  const [picked, setPicked] = useState<Set<string>>(new Set())
  const [note, setNote] = useNote('combat logs')
  const busy = reading || !!note?.busy
  const now = useNow(60_000)
  const show = (list: LogFile[]) => {
    setFiles(list)
    setPicked(new Set(list.filter((f) => f.suggested).map(rowKey)))
  }
  useEffect(() => {
    if (busy) return
    const controller = new AbortController()
    fetchLogFiles(controller.signal).then(show, (err: unknown) => {
      if (!controller.signal.aborted) setNote({ error: errorText(err) })
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
  /** Ticks every log in the group, or clears them all when they were all ticked. */
  const toggle = (group: LogFile[], tick: boolean) =>
    setPicked((p) => {
      const next = new Set(p)
      for (const f of group) {
        if (tick) next.add(rowKey(f))
        else next.delete(rowKey(f))
      }
      return next
    })
  return (
    <>
      <ul className="list log-files">
        {groupLogsByDay(files, now).map(({ label, files: day }) => {
          const pickable = day.filter((f) => !f.active)
          const count = pickable.filter((f) => picked.has(rowKey(f))).length
          const archived = day.filter((f) => f.archived && f.source === 'game').length
          const imports = day.filter((f) => f.source === 'wcl').length
          const unread = day.filter((f) => !f.read).length
          return (
            <li key={label} {...tip(day.map((f) => f.error).find(Boolean))}>
              <label className="inline">
                <input
                  type="checkbox"
                  disabled={!pickable.length || busy}
                  checked={pickable.length > 0 && count === pickable.length}
                  ref={(el) => {
                    if (el) el.indeterminate = count > 0 && count < pickable.length
                  }}
                  onChange={() => toggle(pickable, count < pickable.length)}
                />{' '}
                {label} · {counted(day.length, 'log')} · {sizeText(day.reduce((n, f) => n + f.bytes, 0))}{' '}
                <span className="muted">
                  · {counted(day.reduce((n, f) => n + f.keys, 0), 'key')} ·{' '}
                  {counted(day.reduce((n, f) => n + f.pulls, 0), 'pull')}
                  {archived > 0 && ` · ${archived} archived`}
                  {imports > 0 && ` · ${counted(imports, 'Warcraft Logs import')}`}
                  {day.some((f) => f.active) && ' · Being written'}
                  {unread > 0 && ` · ${unread} not read yet`}
                </span>
                {day.map((f) => f.error && <span key={rowKey(f)} className="warning"> · {f.error}</span>)}
              </label>
            </li>
          )
        })}
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
