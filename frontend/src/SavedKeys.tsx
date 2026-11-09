import { useEffect, useState } from 'react'
import { deleteSavedKeys, fetchSavedKeys, type KeySummary } from './api'
import { chestLabel, clock } from './format'

/** The side panel: the live key, then every saved key (pick one to open it, × to delete it, or clear them all).
 * refresh changes when the list should be read again (a key ended). */
export function SavedKeys({ selected, onSelect, refresh }: {
  selected: number | null
  onSelect: (start: number | null) => void
  refresh: number
}) {
  const [keys, setKeys] = useState<KeySummary[]>([])
  const [error, setError] = useState<string | null>(null)
  const load = () =>
    fetchSavedKeys()
      .then(setKeys)
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
  useEffect(() => void load(), [refresh])

  const remove = (start?: number) => {
    if (!start && !window.confirm('Delete every saved key?')) return
    deleteSavedKeys(start)
      .then(() => {
        if (!start || start === selected) onSelect(null)
        return load()
      })
      .catch((err: unknown) => setError(err instanceof Error ? err.message : String(err)))
  }

  return (
    <aside className="side">
      <button className={selected === null ? 'item active' : 'item'} onClick={() => onSelect(null)}>
        Live key
      </button>
      <h2>Saved keys</h2>
      {error && <p className="warning">{error}</p>}
      {keys.map((k) => (
        <div key={k.start} className={selected === k.start ? 'item active' : 'item'}>
          <button className="open" onClick={() => onSelect(k.start)}>
            <b>
              {k.dungeon} +{k.level}
            </b>
            <span className="muted small">
              {new Date(k.start).toLocaleString()} ·{' '}
              {k.status === 'abandoned' ? 'abandoned' : `${clock(k.finalMS)} ${chestLabel(k.chests)}`}
            </span>
          </button>
          <button className="delete" title="Delete this key" onClick={() => remove(k.start)}>
            ×
          </button>
        </div>
      ))}
      {keys.length === 0 && <p className="muted small">None yet: finished keys are saved here.</p>}
      {keys.length > 0 && (
        <button className="clear" onClick={() => remove()}>
          Clear all
        </button>
      )}
    </aside>
  )
}
