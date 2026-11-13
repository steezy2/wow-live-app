import { useCallback, useEffect, useState } from 'react'
import {
  fetchCharacters,
  fetchSavedKey,
  fetchSettings,
  onAppEvent,
  subscribe,
  type Character,
  type KeyView,
  type Settings,
  type SettingsView,
} from './api'
import { CharacterPicker } from './CharacterPicker'
import { KeyDetails } from './KeyDetails'
import { MPlusLive } from './MPlusLive'
import { RaidPage } from './RaidPage'
import { Recap } from './Recap'
import { SavedKeys } from './SavedKeys'
import { SettingsPage } from './SettingsPage'

/** The site: its sections along the top (M+ Live, Raid, Recap, and Settings in the desktop app) and the character
 * picker, whose pick (Settings' character at first, else all) the saved keys, raid nights and recap show; M+ Live has
 * the saved keys on the side and the live key or the one picked from them. The app opens on Settings when it can't find
 * the log folder. */
export default function App() {
  const [selected, setSelected] = useState<number | null>(null)
  const [refresh, setRefresh] = useState(0)
  const keyEnded = useCallback(() => setRefresh((n) => n + 1), [])
  const [settings, setSettings] = useState<SettingsView | null>(null)
  const [section, setSection] = useState<'mplus' | 'raid' | 'recap' | 'settings'>('mplus')
  const [raidPull, setRaidPull] = useState<number | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [picked, setPicked] = useState<string | null>(null)
  const character = picked ?? characters.find((c) => c.preferred)?.id ?? ''
  const preferred = settings?.character ?? ''
  useEffect(() => {
    const controller = new AbortController()
    fetchCharacters(preferred, controller.signal).then(setCharacters, () => undefined)
    return () => controller.abort()
  }, [preferred, refresh, section]) // a section opened again may follow a raid pull or Read old logs
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal).then((view) => {
      setSettings(view)
      if (view !== null && !view.logsDirFound) setSection('settings')
    }, () => undefined)
    const off = onAppEvent('settings', (s) => setSettings((view) => view && { ...view, ...(s as Settings) }))
    return () => {
      controller.abort()
      off()
    }
  }, [])
  const tab = (name: typeof section) => ({
    className: 'section-tab',
    'aria-current': section === name ? ('page' as const) : undefined,
    onClick: () => {
      setSection(name)
      setRaidPull(null)
    },
  })
  const open = (kind: 'mplus' | 'raid', start: number) => {
    if (kind === 'raid') setRaidPull(start)
    else setSelected(start)
    setSection(kind)
  }
  return (
    <>
      <nav className="topnav" aria-label="Sections">
        <span className="brand">WoW Live</span>
        <button {...tab('mplus')}>M+ Live</button>
        <button {...tab('raid')}>Raid</button>
        <button {...tab('recap')}>Recap</button>
        {settings && <button {...tab('settings')}>Settings</button>}
        <CharacterPicker characters={characters} value={character} onChange={setPicked} />
      </nav>
      {settings && section === 'settings' ? (
        <main className="page">
          <SettingsPage view={settings} onSaved={setSettings} />
        </main>
      ) : section === 'raid' ? (
        <RaidPage key={raidPull} initial={raidPull} character={character} />
      ) : section === 'recap' ? (
        <main className="page">
          <Recap character={characters.find((c) => c.id === character)} parsesFor={preferred} onOpen={open} />
        </main>
      ) : (
        <div className="layout">
          <SavedKeys selected={selected} onSelect={setSelected} refresh={refresh} character={character} />
          <main className="page">
            {selected === null ? <MPlusLive onEnded={keyEnded} character={character} onOpen={setSelected} /> : <SavedKey start={selected} />}
          </main>
        </div>
      )}
    </>
  )
}

/** A saved key, read again when the saved keys change (a report linked to it, say). */
function SavedKey({ start }: { start: number }) {
  const [saved, setKey] = useState<KeyView | null>(null)
  const key = saved?.start === start ? saved : null // never the previous key while this one is read
  const [error, setError] = useState<string | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    const read = () =>
      fetchSavedKey(start, controller.signal)
        .then(setKey)
        .catch((err: unknown) => {
          if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
        })
    const off = subscribe(() => void read())
    return () => {
      controller.abort()
      off()
    }
  }, [start])
  if (error) return <p className="warning">{error}</p>
  return key ? <KeyDetails key={key.start} keyView={key} clockMS={key.clockMS} /> : <p className="muted">Loading…</p>
}
