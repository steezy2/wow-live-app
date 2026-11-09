import { useCallback, useEffect, useState } from 'react'
import { fetchSavedKey, fetchSettings, type KeyView, type SettingsView } from './api'
import { KeyDetails } from './KeyDetails'
import { MPlusLive } from './MPlusLive'
import { SavedKeys } from './SavedKeys'
import { SettingsPage } from './SettingsPage'

/** The site: its sections along the top (M+ Live, and Settings in the desktop app), the saved keys on the side, and
 * the live key or the one picked from them. The app opens on Settings when it can't find the log folder. */
export default function App() {
  const [selected, setSelected] = useState<number | null>(null)
  const [refresh, setRefresh] = useState(0)
  const keyEnded = useCallback(() => setRefresh((n) => n + 1), [])
  const [settings, setSettings] = useState<SettingsView | null>(null)
  const [showSettings, setShowSettings] = useState(false)
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal).then((view) => {
      setSettings(view)
      setShowSettings(view !== null && !view.logsDirFound)
    }, () => undefined)
    return () => controller.abort()
  }, [])
  return (
    <>
      <nav className="topnav" aria-label="Sections">
        <span className="brand">WoW Live</span>
        <button
          className="section-tab"
          aria-current={showSettings ? undefined : 'page'}
          onClick={() => setShowSettings(false)}
        >
          M+ Live
        </button>
        {settings && (
          <button
            className="section-tab"
            aria-current={showSettings ? 'page' : undefined}
            onClick={() => setShowSettings(true)}
          >
            Settings
          </button>
        )}
      </nav>
      {settings && showSettings ? (
        <main className="page">
          <SettingsPage view={settings} onSaved={setSettings} />
        </main>
      ) : (
        <div className="layout">
          <SavedKeys selected={selected} onSelect={setSelected} refresh={refresh} />
          <main className="page">
            {selected === null ? <MPlusLive onEnded={keyEnded} /> : <SavedKey start={selected} />}
          </main>
        </div>
      )}
    </>
  )
}

/** A saved key, read once. */
function SavedKey({ start }: { start: number }) {
  const [key, setKey] = useState<KeyView | null>(null)
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchSavedKey(start, controller.signal)
      .then(setKey)
      .catch((err: unknown) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
      })
    return () => controller.abort()
  }, [start])
  if (error) return <p className="warning">{error}</p>
  return key ? <KeyDetails keyView={key} clockMS={key.clockMS} /> : <p className="muted">Loading…</p>
}
