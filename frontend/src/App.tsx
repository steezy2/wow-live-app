import { useCallback, useEffect, useState } from 'react'
import {
  fetchCharacters,
  fetchSavedKey,
  fetchSavedKeys,
  fetchSettings,
  onAppEvent,
  savedSettings,
  saveSettings,
  subscribe,
  type Character,
  type KeyView,
  type Settings,
  type SettingsView,
} from './api'
import { CharacterPicker } from './CharacterPicker'
import { Home } from './Home'
import { KeyDetails } from './KeyDetails'
import { MPlusLive } from './MPlusLive'
import { RaidPage } from './RaidPage'
import { Recap } from './Recap'
import { RoutesPage } from './RoutesPage'
import { SavedKeys } from './SavedKeys'
import { SettingsPage } from './SettingsPage'
import { Skeleton } from './Skeleton'

/** The longest the splash stays up waiting for the settings and the characters. */
const SPLASH_MAX_MS = 2000

type Section = 'home' | 'mplus' | 'routes' | 'raid' | 'recap' | 'settings'

/** The section the app opens on, chosen once the settings are known (null when they never came): Home on the first
 * run (setup not done and no saved keys, null while not known), else Settings when the log folder is missing, else M+
 * Live. */
function firstSection(view: SettingsView | null, savedKeys: number | null): Section {
  if (view && !view.setupDone && savedKeys === 0) return 'home'
  return view && !view.logsDirFound ? 'settings' : 'mplus'
}

/** Shown in place of the page until the settings and characters are read; the same markup as index.html's splash. */
function Splash() {
  return <div className="splash" role="status">WoW Live</div>
}

/** The site: its sections along the top (Home, M+ Live, Routes, Raid, Recap, and Settings in the desktop app) and the character
 * picker, whose pick (Settings' character at first, else all) the saved keys, raid nights and recap show; M+ Live has
 * the saved keys on the side and the live key or the one picked from them. The app shows a splash until the settings and
 * characters are read (at most SPLASH_MAX_MS), then opens on Home on the first run, or on Settings when it can't find the log folder. */
export default function App() {
  const [selected, setSelected] = useState<number | null>(null)
  const [refresh, setRefresh] = useState(0)
  const keyEnded = useCallback(() => setRefresh((n) => n + 1), [])
  const [settings, setSettings] = useState<SettingsView | null>(null)
  const [picking, setSection] = useState<Section | null>(null) // null until a tab is clicked or the first one is chosen
  const [charactersRead, setCharactersRead] = useState(false)
  const [savedKeys, setSavedKeys] = useState<number | null>(null) // how many keys are saved, for the first section and the guide
  const [reopened, setReopened] = useState(false) // the setup guide was reopened from Settings
  const [anchor, setAnchor] = useState<string | undefined>() // the Settings panel to scroll to
  const [splashOver, setSplashOver] = useState(false)
  const [settingsRead, setSettingsRead] = useState(false) // the settings answered, or there are none (the site without the app)
  const [first, setFirst] = useState<Section | null>(null) // the section opened on, chosen once
  // the Routes page stays mounted so unsaved edits survive a tab switch
  const [routesSeen, setRoutesSeen] = useState(false)
  const [raidPull, setRaidPull] = useState<number | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [picked, setPicked] = useState<string | null>(null)
  const character = picked ?? characters.find((c) => c.preferred)?.id ?? ''
  const preferred = settings?.character ?? ''
  const ready = (settingsRead && charactersRead && savedKeys !== null) || splashOver
  const section = picking ?? first
  if (section === 'routes' && !routesSeen) setRoutesSeen(true)
  if (ready && first === null) setFirst(firstSection(settings, savedKeys)) // chosen once, so later changes never move the page
  useEffect(() => {
    const timer = setTimeout(() => setSplashOver(true), SPLASH_MAX_MS)
    return () => clearTimeout(timer)
  }, [])
  useEffect(() => {
    if (!settingsRead && !splashOver) return // the preferred character comes with the settings: one read, no pop-in
    const controller = new AbortController()
    fetchCharacters(preferred, controller.signal)
      .then(setCharacters, () => undefined)
      .then(() => setCharactersRead(true))
    return () => controller.abort()
  }, [settingsRead, splashOver, preferred, refresh, section]) // a section opened again may follow a raid pull or Read old logs
  useEffect(() => {
    const controller = new AbortController()
    fetchSavedKeys('', controller.signal).then((keys) => setSavedKeys(keys.length), () => undefined)
    return () => controller.abort()
  }, [])
  useEffect(() => {
    if (section === 'settings' && anchor) document.getElementById(anchor)?.scrollIntoView()
  }, [section, anchor, settings])
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal)
      .then(setSettings, () => undefined)
      .then(() => setSettingsRead(true))
    const off = onAppEvent('settings', (s) => setSettings((view) => view && { ...view, ...(s as Settings) }))
    return () => {
      controller.abort()
      off()
    }
  }, [])
  const openSettings = (panel?: string) => {
    setAnchor(panel)
    setSection('settings')
  }
  const openHome = (guide: boolean) => {
    if (guide && settings) {
      setReopened(true)
      saveSettings({ ...savedSettings(settings), setupDone: false }).then(setSettings, () => undefined)
    }
    setSection('home')
  }
  const tab = (name: Section) => ({
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
        {(settings || !settingsRead) && (
          <button {...tab('home')} disabled={!settings}>
            Home
          </button>
        )}
        <button {...tab('mplus')}>M+ Live</button>
        <button {...tab('routes')}>Routes</button>
        <button {...tab('raid')}>Raid</button>
        <button {...tab('recap')}>Recap</button>
        {(settings || !settingsRead) && (
          <button {...tab('settings')} disabled={!settings}>
            Settings
          </button>
        )}
        <CharacterPicker characters={characters} value={character} onChange={setPicked} />
      </nav>
      {routesSeen && (
        <div hidden={section !== 'routes'}>
          <RoutesPage visible={section === 'routes'} />
        </div>
      )}
      {section === null ? (
        <Splash />
      ) : settings && section === 'settings' ? (
        <main className="page">
          <SettingsPage view={settings} onSaved={setSettings} onHome={openHome} />
        </main>
      ) : section === 'home' ? (
        <main className="page">
          {settings ? (
            <Home
              view={settings}
              onSaved={setSettings}
              savedKeys={savedKeys}
              reopened={reopened}
              character={character}
              onOpen={open}
              onSettings={openSettings}
              onRecap={() => setSection('recap')}
            />
          ) : (
            <Skeleton height={320} lines={5} />
          )}
        </main>
      ) : section === 'routes' ? null : section === 'raid' ? (
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
  const [key, setKey] = useState<KeyView | null>(null) // the previous key stays up, dimmed, while this one is read
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
  if (!key) return <Skeleton height={480} />
  return (
    <div className={key.start === start ? undefined : 'stale'}>
      <KeyDetails key={key.start} keyView={key} clockMS={key.clockMS} />
    </div>
  )
}
