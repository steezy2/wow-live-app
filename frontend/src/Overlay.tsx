import { type CSSProperties, type ReactNode, useCallback, useEffect, useRef, useState } from 'react'
import { abilityViews, raidTargets } from './abilities'
import {
  fetchSettings,
  onAppEvent,
  overlayShape,
  type KeyDeath,
  type KeyTrack,
  type KeyView,
  type OverlayColumn,
  type OverlaySettings,
  type RaidPull,
  type Settings,
} from './api'
import { classColors } from './classColors'
import { AbilityBars } from './FightMeters'
import { Skeleton } from './Skeleton'
import { chestLabel, clock, pullTitle, resultText, routeLine } from './format'
import { HitRecapView } from './HitRecap'
import { ForcesBar, TimerBar } from './KeyDetails'
import { healersOnly, overlayColumn, overlayIds, rankOrder } from './meterColumns'
import { MeterCard } from './MeterCards'
import { RaidOverlayCard } from './RaidPage'
import { abilitiesBetween, recapOf, useRaidTrack, useTrack } from './replay'
import { DEMO_KEY, DEMO_RAID, DEMO_RAID_TRACK, DEMO_START, DEMO_TRACK } from './overlayDemo'
import { useLiveKey, useOverlayMoving, useOverlayPreview, useRaidLive } from './useLiveKey'

/** How many of a player's abilities (and enemies) the overlay's breakdown lists. */
const OVERLAY_SPELLS = 8

/** How many rows of a death's recap the overlay shows (the newest). */
const OVERLAY_RECAP_ROWS = 12


/** The window's scale limits: the overlay scales to fill its window, but never below half or above double size. */
const FIT_MIN = 0.5
const FIT_MAX = 2

/** How much the content's size must change (as a share) before the app is told, so a number growing a digit doesn't
 * resize the window on every poll. */
const SHAPE_CHANGE = 0.01

/** Scales the overlay to its window with CSS zoom on root: the content's size at zoom 1 (its measured size divided
 * by the current zoom) against the window's, so nothing scrolls. The app holds the window to that size's shape (told
 * with overlayShape), so no side is left spare. Returns the refs for the root and the content. */
function useFitZoom() {
  const root = useRef<HTMLDivElement>(null)
  const content = useRef<HTMLDivElement>(null)
  useEffect(() => {
    let told = [0, 0] // the content size the app was last told
    const fit = () => {
      if (!root.current || !content.current) return
      const zoom = Number(root.current.style.zoom) || 1
      const { width, height } = content.current.getBoundingClientRect()
      if (!width || !height) return
      const shape = [width / zoom, height / zoom]
      if (shape.some((n, i) => Math.abs(n / told[i] - 1) > SHAPE_CHANGE)) {
        overlayShape(Math.round(shape[0]), Math.round(shape[1]), FIT_MIN, FIT_MAX)
        told = shape
      }
      const next = Math.min(window.innerWidth / (width / zoom), window.innerHeight / (height / zoom))
      const clamped = Math.min(FIT_MAX, Math.max(FIT_MIN, next))
      if (Math.abs(clamped - zoom) > 0.005) root.current.style.zoom = String(clamped)
    }
    const observer = new ResizeObserver(fit)
    observer.observe(content.current!)
    window.addEventListener('resize', fit)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', fit)
    }
  }, [])
  return { root, content }
}

/** The overlay's settings (opacity, one card or two, players and columns), from the app's settings and each save;
 * null until read. */
function useOverlaySettings(): OverlaySettings | null {
  const [settings, setSettings] = useState<OverlaySettings | null>(null)
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal).then((view) => view && setSettings(view.overlay), () => undefined)
    const off = onAppEvent('settings', (s) => setSettings((s as Settings).overlay))
    return () => {
      controller.abort()
      off()
    }
  }, [])
  return settings
}

/** The overlay over the game (/overlay, the desktop app's second window): the clock against the timer, forces,
 * the estimate, deaths, the route's next pull and the pull's DPS and HPS during a key, and the last raid pull between
 * raid pulls. Everything else stays in the full window. While
 * the app says it's being moved, it shows a border and how to move and resize it. */
export function Overlay() {
  const { key, clockMS } = useLiveKey()
  const moving = useOverlayMoving()
  const settings = useOverlaySettings()
  const raid = useRaidLive(key?.status !== 'running')
  const preview = useOverlayPreview()
  const { root, content } = useFitZoom()
  return (
    <div
      ref={root}
      className={moving ? 'overlay moving' : 'overlay'}
      style={{ '--overlay-bg': `${settings?.opacity ?? 0}%` } as CSSProperties}
    >
      <div ref={content} className="overlay-fit">
        {moving && (
          <p className="small overlay-wrap">Drag to move, drag a corner to resize; Ctrl+Alt+O or Settings when done.</p>
        )}
        {!settings ? null : preview === 'raid' ? (
          <OverlayRaid pull={DEMO_RAID} settings={settings} />
        ) : preview === 'mplus' ? (
          <OverlayKey keyView={DEMO_KEY} clockMS={DEMO_KEY.clockMS} settings={settings} />
        ) : raid?.betweenPulls && raid.last ? (
          <OverlayRaid key={raid.last.start} pull={raid.last} settings={settings} />
        ) : key ? (
          <OverlayKey keyView={key} clockMS={clockMS} settings={settings} />
        ) : (
          <p className="muted">Waiting for the next key</p>
        )}
      </div>
    </div>
  )
}

/** The overlay's view of the live key; a click on a player's row (when the overlay takes clicks) shows their
 * abilities in that card's span: the last pull's until a new pull starts, or the whole key's so far (the one-card
 * meter's rows are the whole key's). */
function OverlayKey(props: { keyView: KeyView; clockMS: number; settings: OverlaySettings }) {
  const { keyView: key, clockMS, settings } = props
  const [picked, setPicked] = useState({ player: '', pull: 0, whole: false })
  const pull = key.lastPull.startMS
  const pickPull = useCallback((player: string) => setPicked({ player, pull, whole: false }), [pull])
  const pickWhole = useCallback((player: string) => setPicked({ player, pull, whole: true }), [pull])
  const back = useCallback(() => setPicked({ player: '', pull: 0, whole: false }), [])
  const [deathsOpen, setDeathsOpen] = useState(false)
  const closeDeaths = useCallback(() => setDeathsOpen(false), [])
  const focus = picked.whole || picked.pull === pull ? picked.player : ''
  const timer = key.timerMS ?? 0
  const projection = key.status === 'running' ? key.projection : undefined
  const latest = key.deaths.length > 0 ? key.deaths[key.deaths.length - 1] : undefined
  const last = key.lastPull
  const pullDeaths = key.deaths.filter((d) => d.clockMS >= last.startMS && d.clockMS <= last.endMS)
  const columns = (ids: OverlayColumn[], deaths: KeyDeath[]) => ids.map((id) => overlayColumn(id, last, deaths))
  const ids = overlayIds(settings.columns, settings.rankBy, settings.oneCard)
  const [fresh, dismissFresh] = useFreshDeath(key.deaths, settings.deathRecapSeconds, false)
  return (
    <>
      <div className="overlay-clock">
        <span className="big">{clock(clockMS)}</span>
        {timer > 0 && <span className="muted"> / {clock(timer)}</span>}{' '}
        {key.status !== 'running' ? (
          <span>{resultText(key.status, key.chests, clockMS, timer)}</span>
        ) : (
          projection && (
            <span className={`chests c${projection.chests}`}>
              {chestLabel(projection.chests)} · {clock(projection.finalMS)}
              {projection.runs > 1 && (
                <span className="muted small">
                  {' '}
                  ({clock(projection.lowMS)}–{clock(projection.highMS)})
                </span>
              )}
            </span>
          )
        )}
      </div>
      {timer > 0 && <TimerBar clockMS={clockMS} timerMS={timer} keyView={key} best={projection?.best ?? []} />}
      {key.forces && <ForcesBar forces={key.forces} level={key.level} />}
      <button
        className="row-button small overlay-wrap"
        disabled={key.deaths.length === 0}
        onClick={() => setDeathsOpen(true)}
      >
        {key.deaths.length} deaths +{clock(key.penaltyMS)}
        {latest && ` · last ${deathText(latest)}`}
      </button>
      <div className="overlay-extra">
        {key.route && <p className="small overlay-wrap">{routeLine(key.route)}</p>}
        {deathsOpen ? (
          <DeathsDrill keyView={key} onBack={closeDeaths} />
        ) : fresh ? (
          <KeyRecap keyView={key} death={fresh} onBack={dismissFresh} />
        ) : focus ? (
          <Breakdown
            keyView={key}
            player={focus}
            title={picked.whole ? 'Key' : pullTitle(key, clockMS)}
            from={picked.whole ? 0 : key.lastPull.startMS}
            to={picked.whole ? clockMS : key.lastPull.endMS}
            onBack={back}
          />
        ) : settings.splitCards ? (
          rankOrder(settings.rankBy).map((by) => (
            <MeterCard
              key={by}
              title={by.toUpperCase()}
              meter={by === 'hps' ? healersOnly(key.overall) : key.overall}
              columns={columns(overlayIds(settings.columns, by, false), key.deaths)}
              rows={settings.rows}
              dense
              onPick={pickWhole}
            />
          ))
        ) : settings.oneCard ? (
          <MeterCard
            title={`${pullTitle(key, clockMS)} · key`}
            meter={key.overall}
            columns={columns(ids, key.deaths)}
            rows={settings.rows}
            dense
            onPick={pickWhole}
          />
        ) : (
          <>
            <MeterCard
              title={pullTitle(key, clockMS)}
              meter={last}
              columns={columns(ids, pullDeaths)}
              rows={settings.rows}
              dense
              onPick={pickPull}
            />
            <MeterCard
              title="Key"
              meter={key.overall}
              columns={columns(ids, key.deaths)}
              rows={settings.rows}
              dense
              onPick={pickWhole}
            />
          </>
        )}
      </div>
    </>
  )
}

/** The key's live track, or the preview's dummy one. */
function useKeyTrack(key: KeyView) {
  const demo = key.start === DEMO_START
  const track = useTrack(key.start, !demo && key.status === 'running')
  return demo ? DEMO_TRACK : track
}

/** The raid overlay between pulls: the pull's card, in place of which a death's recap shows when one of its lines
 * was clicked, or for the settings' seconds after the pull ends (the first death's; a raid log is written in batches,
 * so there is no newest one to show live). */
function OverlayRaid({ pull, settings }: { pull: RaidPull; settings: OverlaySettings }) {
  const [fresh, dismiss] = useFreshDeath(pull.deaths, settings.deathRecapSeconds, pull.start !== DEMO_START)
  const [picked, setPicked] = useState<RaidPull['deaths'][number]>()
  const death = picked ?? fresh
  if (!death) return <RaidOverlayCard pull={pull} settings={settings} onPickDeath={setPicked} />
  return <RaidRecap pull={pull} death={death} onBack={picked ? () => setPicked(undefined) : dismiss} />
}

/** The death to show a recap of without a click: once deaths has grown (at first, with fromStart, from none), the
 * newest, or with fromStart the first (a raid log is written in batches), stays up for seconds (0 is off) or until
 * dismissed. */
function useFreshDeath<D>(deaths: D[], seconds: number, fromStart: boolean) {
  const seen = useRef(fromStart ? 0 : deaths.length)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const [fresh, setFresh] = useState<D>()
  const count = deaths.length
  useEffect(() => {
    const grew = count > seen.current
    seen.current = count
    if (!grew || !seconds) return
    setFresh(fromStart ? deaths[0] : deaths[deaths.length - 1])
    clearTimeout(timer.current)
    timer.current = setTimeout(() => setFresh(undefined), seconds * 1000)
    // deaths is read when it grows only: a new array each poll mustn't restart the timer
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [count, seconds])
  useEffect(() => () => clearTimeout(timer.current), [])
  const dismiss = useCallback(() => setFresh(undefined), [])
  return [fresh, dismiss] as const
}

/** A death's recap card: back goes to the meters. */
function RecapDrill(props: { title: string; death: KeyDeath; track: KeyTrack | null | undefined; onBack: () => void }) {
  const { title, death, track, onBack } = props
  return (
    <Drill title={title} heading={deathText(death)} onBack={onBack}>
      <RecapBody track={track} death={death} />
    </Drill>
  )
}

/** The death's recap rows from the track, or why there are none. */
function RecapBody({ track, death }: { track: KeyTrack | null | undefined; death: KeyDeath }) {
  const recap = recapOf(track, death.player, death.clockMS)
  if (recap) return <HitRecapView recap={recap} ready={death.ready} limit={OVERLAY_RECAP_ROWS} />
  return track ? <p className="muted">No recap for this death</p> : <Skeleton height={64} />
}

function KeyRecap(props: { keyView: KeyView; death: KeyDeath; onBack: () => void }) {
  const track = useKeyTrack(props.keyView)
  return <RecapDrill title="Key" death={props.death} track={track} onBack={props.onBack} />
}

function RaidRecap(props: { pull: RaidPull; death: KeyDeath; onBack: () => void }) {
  const { track: saved } = useRaidTrack(props.pull.start)
  const track = props.pull.start === DEMO_START ? DEMO_RAID_TRACK : saved
  return <RecapDrill title={`Pull ${props.pull.number}`} death={props.death} track={track} onBack={props.onBack} />
}

/** A player's top abilities from the key's clock from to to (the last pull, or the whole key so far), from the live
 * track's ability stretches: their damage over their damage into each enemy, or their healing when they healed more.
 * A click on the title, a right click or Esc goes back. */
function Breakdown(props: {
  keyView: KeyView
  player: string
  title: string
  from: number
  to: number
  onBack: () => void
}) {
  const { keyView: key, player, title, from, to, onBack } = props
  const track = useKeyTrack(key)
  const between = track ? abilitiesBetween(track, key, player, from, to) : undefined
  const views = between && abilityViews(player, between.abilities, between.taken)
  const sum = (rows: { value: number }[]) => rows.reduce((n, r) => n + r.value, 0)
  const healer = views !== undefined && sum(views.healing) > sum(views.damage)
  const color = classColors[key.overall.rows.find((r) => r.player === player)?.class ?? ''] ?? 'var(--accent)'
  const targets = healer ? [] : raidTargets(between?.targets, player)
  return (
    <Drill title={title} heading={`${player} · ${healer ? 'Healing' : 'Damage'}`} onBack={onBack}>
      {views ? (
        <>
          <AbilityBars rows={healer ? views.healing : views.damage} color={color} limit={OVERLAY_SPELLS} />
          {targets.length > 0 && <AbilityBars rows={targets} color={color} label="Enemy" limit={OVERLAY_SPELLS} />}
        </>
      ) : (
        track ? <p className="muted">No ability timeline for this key</p> : <Skeleton height={64} />
      )}
    </Drill>
  )
}

/** A card the overlay shows in place of the meters: a title button, a right click or Esc goes back. */
function Drill(props: { title: string; heading: string; onBack: () => void; children: ReactNode }) {
  const { title, heading, onBack, children } = props
  useEffect(() => {
    const esc = (e: KeyboardEvent) => e.key === 'Escape' && onBack()
    window.addEventListener('keydown', esc)
    return () => window.removeEventListener('keydown', esc)
  }, [onBack])
  return (
    <div
      className="card focus drill"
      onContextMenu={(e) => {
        e.preventDefault()
        onBack()
      }}
    >
      <h2>
        <button className="back" onClick={onBack}>
          ‹ {title}
        </button>{' '}
        · {heading}
      </h2>
      {children}
    </div>
  )
}

/** "Name: Ability 12:03", the ability the death names or else its killer. */
const deathText = (d: KeyDeath) => `${d.player}: ${d.ability || d.killer} ${clock(d.clockMS)}`

/** The key's deaths, newest first; a click on one shows its recap (the last 12 rows) from the live track, and back
 * returns to the list, then to the meters. */
function DeathsDrill({ keyView: key, onBack }: { keyView: KeyView; onBack: () => void }) {
  const track = useKeyTrack(key)
  const [picked, setPicked] = useState<KeyDeath>()
  const back = useCallback(() => (picked ? setPicked(undefined) : onBack()), [picked, onBack])
  return (
    <Drill title={picked ? 'Deaths' : 'Key'} heading={picked ? deathText(picked) : 'Deaths'} onBack={back}>
      {picked ? (
        <RecapBody track={track} death={picked} />
      ) : (
        <ul className="list">
          {key.deaths.toReversed().map((d, i) => (
            <li key={i}>
              <button className="row-button" onClick={() => setPicked(d)}>
                {deathText(d)}
              </button>
            </li>
          ))}
        </ul>
      )}
    </Drill>
  )
}
