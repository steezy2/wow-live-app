import { useCallback, useEffect, useRef, useState } from 'react'
import { deleteSavedKeys, fetchSavedKeys, type KeySummary } from './api'
import { chestLabel, clock, counted, errorText } from './format'
import { ReportTag } from './ReportLinks'
import { SideGroups } from './SideGroups'
import { Skeleton } from './Skeleton'
import { tip } from './tipAttrs'

/** The side panel: the live key, then every saved key by day, or by week past the last 7 days (pick one to open it,
 * × to delete it, or clear them all).
 * refresh changes when the list should be read again (a key ended); character is the top bar's pick ('' for all). */
export function SavedKeys({ selected, onSelect, refresh, character }: {
  selected: number | null
  onSelect: (start: number | null) => void
  refresh: number
  character: string
}) {
  // null until first read; then the last list stays up while it is read again
  const [keys, setKeys] = useState<KeySummary[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const asked = useRef(0)
  const load = useCallback(() => {
    const n = ++asked.current
    return fetchSavedKeys(character)
      .then((list) => n === asked.current && setKeys(list)) // an answer to an older ask never lands over a newer
      .catch((err: unknown) => setError(errorText(err)))
  }, [character])
  useEffect(() => void load(), [refresh, load])

  const remove = (start?: number) => {
    if (!start && !window.confirm('Delete every saved key?')) return
    deleteSavedKeys(start)
      .then(() => {
        if (!start || start === selected) onSelect(null)
        return load()
      })
      .catch((err: unknown) => setError(errorText(err)))
  }

  return (
    <aside className="side">
      <button className={selected === null ? 'item active' : 'item'} onClick={() => onSelect(null)}>
        Live key
      </button>
      <h2>Saved keys</h2>
      {error && <p className="warning">{error}</p>}
      <SideGroups
        items={keys ?? []}
        time={(k) => k.start}
        count={(g) => counted(g.length, 'key')}
        isSelected={(k) => k.start === selected}
      >
        {(group) =>
          group.map((k) => (
            <div key={k.start} className={selected === k.start ? 'item active' : 'item'}>
              <button className="open" onClick={() => onSelect(k.start)}>
                <b>
                  {k.dungeon} +{k.level}
                </b>
                <span className="muted small">
                  {new Date(k.start).toLocaleString([], { weekday: 'short', hour: 'numeric', minute: '2-digit' })}
                  {' · '}
                  {k.status === 'abandoned' ? 'abandoned' : `${clock(k.finalMS)} ${chestLabel(k.chests)}`}
                </span>
              </button>
              {k.report && <ReportTag link={k.report} imported={k.imported} />}
              <button
                className="delete"
                aria-label="Delete this key"
                {...tip('Delete this key')}
                onClick={() => remove(k.start)}
              >
                ×
              </button>
            </div>
          ))
        }
      </SideGroups>
      {keys === null && !error && <Skeleton height={120} />}
      {keys?.length === 0 && <p className="muted small">None yet: finished keys are saved here.</p>}
      {keys && keys.length > 0 && (
        <button className="clear" onClick={() => remove()}>
          Clear all
        </button>
      )}
    </aside>
  )
}
