import { useEffect, useState } from 'react'
import { abandonLiveKey, fetchSavedKeys, type KeySummary, type KeyView } from './api'
import { clock } from './format'
import { useLiveKey, useNow } from './useLiveKey'
import { KeyDetails } from './KeyDetails'
import { Skeleton } from './Skeleton'
import { WaitingCard } from './WaitingCard'
import { keysTonight } from './waiting'

/** The live key, read from the combat log as the game writes it: in batches as fights start and end, so a pull's
 * numbers arrive when it's over, and a line says how long ago the last batch came. The game logs no end for a key the
 * party left, and can write the line that shows it minutes late, so that line has a button to end it. onEnded runs
 * when a key ends. Between keys, character (the top bar's, '' for all) picks today's keys the waiting card lists, and
 * onOpen opens one. */
export function MPlusLive({ onEnded, character, onOpen }: {
  onEnded: () => void
  character: string
  onOpen: (start: number) => void
}) {
  const { loaded, key, clockMS, silentMS, error, endedHere } = useLiveKey(onEnded)
  if (!loaded) return error ? <p className="muted">{error}</p> : <Skeleton height={320} />
  if (key === null || (key.status !== 'running' && key.start !== endedHere)) {
    return <Waiting last={key} character={character} onOpen={onOpen} />
  }
  return (
    <>
      {error && <p className="warning">Lost the site: {error}. Showing the last answer.</p>}
      {key.status === 'running' && (
        <p className="muted small log-age">
          The game last wrote its log {clock(silentMS)} ago (it writes as fights start and end).
          {/* A key that ended meanwhile answers 409, and the next reading shows it. */}
          <button className="action" onClick={() => void abandonLiveKey().catch(() => undefined)}>
            End as abandoned
          </button>
        </p>
      )}
      <KeyDetails key={key.start} keyView={key} clockMS={clockMS} justEnded={endedHere === key.start} />
    </>
  )
}

/** Between keys: the waiting card, with what happened to the last key and today's keys, read again when a key ends. */
function Waiting({ last, character, onOpen }: { last: KeyView | null; character: string; onOpen: (start: number) => void }) {
  const [keys, setKeys] = useState<KeySummary[]>([])
  const lastStart = last?.start
  useEffect(() => {
    const controller = new AbortController()
    fetchSavedKeys(character, controller.signal).then(setKeys, () => undefined)
    return () => controller.abort()
  }, [character, lastStart])
  const now = useNow(60_000)
  const midnight = new Date(now).setHours(0, 0, 0, 0)
  return (
    <WaitingCard
      title="Waiting for the next key"
      hint="It shows here when the key starts."
      rows={keysTonight(keys, midnight, onOpen)}
    >
      {last && (
        <p className="muted">
          Last key: {last.dungeon} +{last.level}, {last.status}.
        </p>
      )}
    </WaitingCard>
  )
}
