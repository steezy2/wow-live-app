import { useEffect, useRef, useState } from 'react'
import {
  fetchKeyStatus,
  fetchRaidNights,
  fetchRecap,
  fetchRecapPeriods,
  fetchSavedKeys,
  savedSettings,
  saveSettings,
  type KeySummary,
  type RaidPullSummary,
  type Recap,
  type SettingsView,
} from './api'
import { guideMet, guideSteps, showGuide } from './guide'
import { Skeleton } from './Skeleton'
import { StoragePanel } from './StoragePanel'
import { Tiles } from './Tiles'
import { stateLine } from './waiting'
import { useLiveKey, useNow, useRaidLive } from './useLiveKey'

/** Opens a saved key or raid pull, or a Settings panel (an element id; none for the top). */
export interface HomeActions {
  onOpen: (kind: 'mplus' | 'raid', start: number) => void
  onSettings: (anchor?: string) => void
  onRecap: () => void
}

/** The Home tab: the setup guide (a new user's, or reopened from Settings), the status of the game and the live key or
 * pull, quick links to the last key and pull, this period's numbers at a glance and the data the app stores. savedKeys
 * is how many keys are saved (null until known); reopened shows the guide again for a user who has keys. */
export function Home({
  view,
  onSaved,
  savedKeys,
  reopened,
  character,
  ...actions
}: HomeActions & {
  view: SettingsView
  onSaved: (view: SettingsView) => void
  savedKeys: number | null
  reopened: boolean
  character: string
}) {
  return (
    <>
      <header className="title">
        <h1>Home</h1>
      </header>
      <div className="home">
        {showGuide(view.setupDone, savedKeys, reopened) && (
          <Guide view={view} onSaved={onSaved} onSettings={actions.onSettings} />
        )}
        <Status view={view} />
        <QuickLinks character={character} {...actions} />
        <AtAGlance character={character} />
        <StoragePanel view={view} onSaved={onSaved} />
      </div>
    </>
  )
}

/** The setup steps with their checks, each linking to its Settings panel. Done, or every required step met after the
 * guide opened with one missing, saves the setup as done. */
function Guide({
  view,
  onSaved,
  onSettings,
}: {
  view: SettingsView
  onSaved: (view: SettingsView) => void
  onSettings: (anchor?: string) => void
}) {
  const [keysSet, setKeysSet] = useState(false)
  useEffect(() => {
    fetchKeyStatus().then((s) => setKeysSet(s.set), () => undefined)
  }, [])
  const steps = guideSteps(view, keysSet)
  const met = guideMet(steps)
  const done = () => void saveSettings({ ...savedSettings(view), setupDone: true }).then(onSaved, () => undefined)
  const wasMet = useRef(met)
  useEffect(() => {
    if (met && !wasMet.current) done()
    wasMet.current = met
  })
  return (
    <section className="panel">
      <h2>Set up WoW Live</h2>
      <ol className="guide">
        {steps.map((s) => (
          <li key={s.label} className={s.met ? 'met' : undefined}>
            <span aria-hidden>{s.met ? '✓' : '○'}</span> {s.label}
            {s.optional && <span className="muted"> (optional)</span>}
            {!s.met && (
              <>
                {' '}
                <button className="link small" onClick={() => onSettings(s.anchor)}>
                  Open in Settings
                </button>
              </>
            )}
          </li>
        ))}
      </ol>
      <div className="actions">
        <button type="button" onClick={done}>
          Done
        </button>
      </div>
    </section>
  )
}

/** The live key or raid pull now, when the game last wrote a log, and the character picked in Settings. */
function Status({ view }: { view: SettingsView }) {
  const key = useLiveKey().key
  const raid = useRaidLive()
  const now = useNow(60_000)
  const line = stateLine(view, now)
  const live =
    key?.status === 'running'
      ? `${key.dungeon} +${key.level} is running`
      : raid?.pulling
        ? `Pulling ${raid.boss ?? 'a boss'} now`
        : 'No key or raid pull running'
  return (
    <section className="panel">
      <h2>Status</h2>
      <p>{live}</p>
      {line && <p className={line.warn ? 'warning' : 'muted'}>{line.text}</p>}
      <p className="muted">{view.character ? `Character: ${view.character}` : 'No character picked in Settings.'}</p>
    </section>
  )
}

/** Links to the last saved key and raid pull, the Recap and Settings' overlay panel. */
function QuickLinks({ character, onOpen, onSettings, onRecap }: HomeActions & { character: string }) {
  const [key, setKey] = useState<KeySummary | null>(null)
  const [pull, setPull] = useState<RaidPullSummary | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchSavedKeys(character, controller.signal).then((keys) => setKey(keys[0] ?? null), () => undefined)
    fetchRaidNights(character, controller.signal).then(
      (nights) => setPull(nights[0]?.pulls[0] ?? null),
      () => undefined,
    )
    return () => controller.abort()
  }, [character])
  return (
    <section className="panel">
      <h2>Quick links</h2>
      <div className="actions">
        {key && (
          <button onClick={() => onOpen('mplus', key.start)}>
            Last key: {key.dungeon} +{key.level}
          </button>
        )}
        {pull && <button onClick={() => onOpen('raid', pull.start)}>Last raid pull: {pull.boss}</button>}
        <button onClick={onRecap}>Recap</button>
        <button onClick={() => onSettings('settings-overlay')}>Overlay settings</button>
      </div>
    </section>
  )
}

/** The current period's keys, timed share, pulls and kills for the character in the top bar, as the Recap counts them. */
function AtAGlance({ character }: { character: string }) {
  const [answer, setAnswer] = useState<{ label: string; recap: Recap } | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchRecapPeriods(controller.signal)
      .then(([period]) =>
        fetchRecap(period, character, controller.signal).then((recap) => setAnswer({ label: period.label, recap })),
      )
      .catch(() => undefined)
    return () => controller.abort()
  }, [character])
  if (!answer) return <Skeleton height={96} lines={2} />
  const { mplus, raid } = answer.recap
  return (
    <section>
      <h2>At a glance · {answer.label}</h2>
      {mplus.keys + raid.pulls === 0 ? (
        <p className="muted">No keys or raid pulls in this period</p>
      ) : (
        <Tiles
          tiles={[
            [String(mplus.keys), 'keys'],
            [mplus.keys ? `${Math.round(mplus.successPct)}%` : '–', 'timed'],
            [String(raid.pulls), 'pulls'],
            [String(raid.kills), 'kills'],
          ]}
        />
      )}
    </section>
  )
}
