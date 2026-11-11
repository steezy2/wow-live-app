import { abandonLiveKey, type KeyView, type ReadyCheck } from './api'
import { clock } from './format'
import { ReadyPanel } from './ReadyPanel'
import { useLiveKey, useReady } from './useLiveKey'
import { KeyDetails } from './KeyDetails'

/** The live key, read from the combat log as the game writes it: in batches as fights start and end, so a pull's
 * numbers arrive when it's over, and a line says how long ago the last batch came. The game logs no end for a key the
 * party left, and can write the line that shows it minutes late, so that line has a button to end it. onEnded runs
 * when a key ends. Between keys the ready check shows on top, even while the last key's review stays up. */
export function MPlusLive({ onEnded }: { onEnded: () => void }) {
  const { loaded, key, clockMS, silentMS, error, endedHere } = useLiveKey(onEnded)
  const ready = useReady()
  if (!loaded) return <p className="muted">{error ?? 'Connecting to the site…'}</p>
  if (key === null || (key.status !== 'running' && key.start !== endedHere)) {
    return <Waiting last={key} ready={ready} />
  }
  return (
    <>
      {error && <p className="warning">Lost the site: {error}. Showing the last answer.</p>}
      {ready && key.status !== 'running' && <ReadyPanel ready={ready} />}
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

/** Between keys: waiting for the next one, with what happened to the last, and the ready check while the party is in
 * a Mythic dungeon. */
function Waiting({ last, ready }: { last: KeyView | null; ready: ReadyCheck | null }) {
  return (
    <section className="waiting">
      <h1>Waiting for the next key</h1>
      <p className="muted">
        It shows here when the key starts, with combat logging on (Advanced Combat Logging, then /combatlog in game).
      </p>
      {last && (
        <p className="muted">
          Last key: {last.dungeon} +{last.level}, {last.status}. It's in the saved keys on the side.
        </p>
      )}
      {ready ? (
        <ReadyPanel ready={ready} />
      ) : (
        <p className="muted small">
          In a Mythic dungeon, the ready check shows who's missing a flask, food or party buff once the game writes its
          log, which is often only as the key starts; the key's start checks everyone either way.
        </p>
      )}
    </section>
  )
}
