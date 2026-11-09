import { useCallback, useEffect, useState } from 'react'
import { fetchSavedKey, fetchSettings, type KeyView, type SettingsView } from './api'
import { KeyDetails } from './KeyDetails'
import { MPlusLive } from './MPlusLive'
import { RaidPage } from './RaidPage'
import { SavedKeys } from './SavedKeys'
import { SettingsPage } from './SettingsPage'

/** The site: its sections along the top (M+ Live, Raid, and Settings in the desktop app); M+ Live has the saved keys
 * on the side and the live key or the one picked from them. The app opens on Settings when it can't find the log
 * folder. */
export default function App() {
  const [selected, setSelected] = useState<number | null>(null)
  const [refresh, setRefresh] = useState(0)
  const keyEnded = useCallback(() => setRefresh((n) => n + 1), [])
  const [settings, setSettings] = useState<SettingsView | null>(null)
  const [section, setSection] = useState<'mplus' | 'raid' | 'settings'>('mplus')
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal).then((view) => {
      setSettings(view)
      if (view !== null && !view.logsDirFound) setSection('settings')
    }, () => undefined)
    return () => controller.abort()
  }, [])
  const tab = (name: typeof section) => ({
    className: 'section-tab',
    'aria-current': section === name ? ('page' as const) : undefined,
    onClick: () => setSection(name),
  })
  return (
    <>
      <nav className="topnav" aria-label="Sections">
        <span className="brand">WoW Live</span>
        <button {...tab('mplus')}>M+ Live</button>
        <button {...tab('raid')}>Raid</button>
        {settings && <button {...tab('settings')}>Settings</button>}
      </nav>
      {settings && section === 'settings' ? (
        <main className="page">
          <SettingsPage view={settings} onSaved={setSettings} />
        </main>
      ) : section === 'raid' ? (
        <RaidPage />
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
