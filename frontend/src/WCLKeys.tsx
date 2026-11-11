import { type FormEvent, useEffect, useState } from 'react'
import { fetchKeyStatus, removeKeys, saveKeys, testKeys, type KeyStatus } from './api'
import { type Note, track } from './note'
import { NoteLine } from './NoteLine'

const CLIENTS_URL = 'https://www.warcraftlogs.com/api/clients'

/** No keys saved. */
const NO_KEYS: KeyStatus = { set: false, idEnd: '', testedMS: 0, works: false, pointsLeft: 0, limitPerHour: 0, problem: '' }

/** The Warcraft Logs client ID and secret the app reviews keys with: how to make them, a test before they're saved
 * (or "Save anyway"), and once saved only the ID's last 4 characters and the last test, with Test keys, Replace (which
 * can be cancelled) and Remove (which asks first). */
export function WCLKeys() {
  const [status, setStatus] = useState<KeyStatus | null>(null)
  const [replacing, setReplacing] = useState(false)
  const [removing, setRemoving] = useState(false)
  const [id, setId] = useState('')
  const [secret, setSecret] = useState('')
  const [showSecret, setShowSecret] = useState(false)
  const [note, setNote] = useState<Note>(null)
  useEffect(() => {
    fetchKeyStatus().then(setStatus, () => undefined)
  }, [])
  const run = (call: Promise<KeyStatus | void>, busy: string, done: string) =>
    track(call, setNote, busy, () => done).then(
      (next) => {
        setStatus(next ?? NO_KEYS)
        setReplacing(false)
        setRemoving(false)
        setSecret('')
      },
      () => undefined,
    )
  const save = (force: boolean) => (e?: FormEvent) => {
    e?.preventDefault()
    void run(saveKeys(id.trim(), secret.trim(), force), 'Testing with Warcraft Logs…', 'Saved')
  }
  if (!status) return null
  const busy = !!note?.busy
  const tested = status.testedMS ? ` · tested ${new Date(status.testedMS).toLocaleTimeString()}` : ''
  return (
    <section className="panel settings">
      <h2>Warcraft Logs keys</h2>
      <p className="muted">
        Only the key review and a dungeon's first estimate need them; everything else works without.
      </p>
      {status.set && !replacing ? (
        <>
          <p>Client ID ending {status.idEnd}</p>
          <p className={status.testedMS === 0 ? 'muted' : status.works ? 'good' : 'warning'}>
            {status.testedMS === 0
              ? 'Not tested since the app started'
              : status.works
                ? `Works · ${Math.round(status.pointsLeft).toLocaleString()} of ${status.limitPerHour.toLocaleString()} points left this hour${tested}`
                : `Failed: ${status.problem}${tested}`}
          </p>
          {removing ? (
            <div className="actions">
              <span>Remove the keys? Reviews stop until new ones are saved.</span>
              <button
                type="button"
                className="danger"
                disabled={busy}
                onClick={() => void run(removeKeys(), 'Removing…', 'Removed')}
              >
                Remove
              </button>
              <button type="button" onClick={() => setRemoving(false)}>
                Keep
              </button>
            </div>
          ) : (
            <div className="actions">
              <button type="button" disabled={busy} onClick={() => void run(testKeys(), 'Testing…', 'Tested')}>
                Test keys
              </button>
              <button type="button" onClick={() => setReplacing(true)}>
                Replace
              </button>
              <button type="button" className="danger" onClick={() => setRemoving(true)}>
                Remove
              </button>
            </div>
          )}
        </>
      ) : (
        <form className="settings" onSubmit={save(false)}>
          <p className="muted">These are Warcraft Logs credentials, not your Battle.net login.</p>
          <ol className="muted">
            <li>
              Sign in to Warcraft Logs with your Battle.net account linked to it (link it first in your Warcraft Logs
              settings if it isn't, or the keys won't work).
            </li>
            <li>
              Open{' '}
              <a href={CLIENTS_URL} target="_blank" rel="noreferrer">
                {CLIENTS_URL}
              </a>{' '}
              and Create Client: any name, redirect URL <code>http://localhost</code>, and leave Public Client
              unchecked.
            </li>
            <li>
              Don't close that page yet: copy its client ID and client secret into the boxes below, then Test and save.
            </li>
          </ol>
          <label>
            Client ID
            <input value={id} onChange={(e) => setId(e.target.value)} spellCheck={false} />
          </label>
          <label>
            Client secret
            <input
              type={showSecret ? 'text' : 'password'}
              value={secret}
              onChange={(e) => setSecret(e.target.value)}
              spellCheck={false}
            />
          </label>
          <label className="inline">
            <input type="checkbox" checked={showSecret} onChange={(e) => setShowSecret(e.target.checked)} /> Show
            the secret
          </label>
          <div className="actions">
            <button type="submit" disabled={busy}>
              Test and save
            </button>
            {note?.error && (
              <button type="button" disabled={busy} onClick={save(true)}>
                Save anyway
              </button>
            )}
            {status.set && (
              <button type="button" onClick={() => setReplacing(false)}>
                Cancel
              </button>
            )}
          </div>
        </form>
      )}
      <NoteLine note={note} />
    </section>
  )
}
