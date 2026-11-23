import { useCallback, useEffect, useState } from 'react'
import {
  fetchCharacters,
  fetchSavedKey,
  fetchSavedKeys,
  fetchSettings,
  onAppEvent,
  savedSettings,
  saveSettings,
  type Character,
  type Settings,
  type SettingsView,
  type FightKind,
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
import { SimPage } from './Sim'
import { Skeleton } from './Skeleton'
import { useSaved } from './useSaved'


/** The longest the splash stays up waiting for the settings and the characters. */
const SPLASH_MAX_MS = 2000

type Section = 'home' | 'mplus' | 'routes' | 'raid' | 'recap' | 'sim' | 'settings'

/** Shown in place of the page until the settings and characters are read; the same markup as index.html's splash. */
function Splash() {
  return <div className="splash" role="status">WoW Live</div>
}

/** The site: its sections along the top (Home, M+ Live, Routes, Raid, Recap, and Sim and Settings in the desktop app)
 * and the character picker, whose pick (Settings' character at first, all when M+ Live is first opened, then the last
 * one) the saved keys, raid nights and recap show; M+ Live has the saved keys on the side and the live key or the one
 * picked from them. The app shows a splash until the settings and characters are read (at most SPLASH_MAX_MS), then
 * opens on Home. */
export default function App() {
  const [selected, setSelected] = useState<number | null>(null)
  const [refresh, setRefresh] = useState(0)
  const keyEnded = useCallback(() => setRefresh((n) => n + 1), [])
  const [settings, setSettings] = useState<SettingsView | null>(null)
  const [picking, setSection] = useState<Section | null>(null) // null until a tab is clicked
  const [charactersRead, setCharactersRead] = useState(false)
  // how many keys are saved, for the guide
  const [savedKeys, setSavedKeys] = useState<number | null>(null)
  const [reopened, setReopened] = useState(false) // the setup guide was reopened from Settings
  const [anchor, setAnchor] = useState<string | undefined>() // the Settings panel to scroll to
  const [splashOver, setSplashOver] = useState(false)
  // the settings answered, or there are none (the site without the app)
  const [settingsRead, setSettingsRead] = useState(false)
  // the Routes page stays mounted so unsaved edits survive a tab switch
  const [routesSeen, setRoutesSeen] = useState(false)
  // the Sim page too, so the paste, the profile and a running sim survive it
  const [simSeen, setSimSeen] = useState(false)
  const [raidPull, setRaidPull] = useState<number | null>(null)
  const [characters, setCharacters] = useState<Character[]>([])
  const [picked, setPicked] = useState<string | null>(null)
  const character = picked ?? characters.find((c) => c.preferred)?.id ?? ''
  const preferred = settings?.character ?? ''
  const ready = (settingsRead && charactersRead && savedKeys !== null) || splashOver
  const section = picking ?? (ready ? 'home' : null)
  if (section === 'routes' && !routesSeen) setRoutesSeen(true)
  if (section === 'sim' && !simSeen) setSimSeen(true)
  if (section === 'mplus' && picked === null) setPicked('') // the first M+ open shows all; after that the pick stays
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
    // a section opened again may follow a raid pull or Read old logs
  }, [settingsRead, splashOver, preferred, refresh, section])
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
  const open = (kind: FightKind, start: number) => {
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
        {settings && <button {...tab('sim')}>Sim</button>}
        {(settings || !settingsRead) && (
          <button {...tab('settings')} disabled={!settings}>
            Settings
          </button>
        )}
        <CharacterPicker characters={characters} value={character} onChange={setPicked} />
      </nav>
      {routesSeen && (
        <div hidden={section !== 'routes'}>
          <RoutesPage visible={section === 'routes'} character={character} />
        </div>
      )}
      {simSeen && (
        <div hidden={section !== 'sim'}>
          <SimPage />
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
      ) : section === 'routes' || section === 'sim' ? null : section === 'raid' ? (
        <RaidPage key={raidPull} initial={raidPull} character={character} />
      ) : section === 'recap' ? (
        <main className="page">
          <Recap character={characters.find((c) => c.id === character)} parsesFor={preferred} onOpen={open} />
        </main>
      ) : (
        <div className="layout">
          <SavedKeys selected={selected} onSelect={setSelected} refresh={refresh} character={character} />
          <main className="page">
            {selected === null ? (
              <MPlusLive onEnded={keyEnded} character={character} onOpen={setSelected} />
            ) : (
              <SavedKey start={selected} />
            )}
          </main>
        </div>
      )}
    </>
  )
}

/** A saved key. */
function SavedKey({ start }: { start: number }) {
  // the previous key stays up, dimmed, while this one is read
  const { data: key, error } = useSaved(start, fetchSavedKey)
  if (error) return <p className="warning">{error}</p>
  if (!key) return <Skeleton height={480} />
  return (
    <div className={key.start === start ? undefined : 'stale'}>
      <KeyDetails key={key.start} keyView={key} clockMS={key.clockMS} />
    </div>
  )
}
