import type { KeyView, ReadyCheck } from './api'
import { ReadyPanel } from './ReadyPanel'
import { useLiveKey, useReady } from './useLiveKey'
import { KeyDetails } from './KeyDetails'

/** The live key, read from the combat log a few seconds behind the game; onEnded runs when a key ends. Between keys
 * the ready check shows on top, even while the last key's review stays up. */
export function MPlusLive({ onEnded }: { onEnded: () => void }) {
  const { loaded, key, clockMS, error, endedHere } = useLiveKey(onEnded)
  const ready = useReady()
  if (!loaded) return <p className="muted">{error ?? 'Connecting to the site…'}</p>
  if (key === null || (key.status !== 'running' && key.start !== endedHere) || key.quiet) {
    return <Waiting last={key} ready={ready} />
  }
  return (
    <>
      {error && <p className="warning">Lost the site: {error}. Showing the last answer.</p>}
      {ready && key.status !== 'running' && <ReadyPanel ready={ready} />}
      <KeyDetails keyView={key} clockMS={clockMS} justEnded={endedHere === key.start} />
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
      {last?.quiet && (
        <p className="warning">
          {last.dungeon} +{last.level} has had nothing in the log for a few minutes, so it looks abandoned.
        </p>
      )}
      {last && !last.quiet && (
        <p className="muted">
          Last key: {last.dungeon} +{last.level}, {last.status}. It's in the saved keys on the side.
        </p>
      )}
      {ready ? (
        <ReadyPanel ready={ready} />
      ) : (
        <p className="muted small">
          Once you're in a Mythic dungeon, the ready check shows who's missing a flask, food or party buff.
        </p>
      )}
    </section>
  )
}
