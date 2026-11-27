import { useCallback, useEffect, useRef, useState } from 'react'
import {
  fetchSim,
  fetchSimChoices,
  fetchSimEngine,
  fetchSimReference,
  fetchSims,
  installSimEngine,
  parseSimExport,
  removeSim,
  startSim,
  type Sim,
  type SimChoices,
  type SimConsumables,
  type SimEngineStatus,
  type SimKind,
  type SimOptions,
  type SimPlan,
  type SimProfile,
  type SimEnhancements,
  type SimReference,
  type SimRequest,
  type SimVariant,
} from './api'
import { errorText, sizeText } from './format'
import { OmniumFolio } from './OmniumFolio'
import { ConsumablesCard } from './SimConsumables'
import { SimGear, SimHeader } from './SimGear'
import { useItemInfo } from './simItems'
import { usePlan } from './simPlan'
import { SimResult } from './SimResult'
import { CustomCard } from './SimCustom'
import { EnhancementsCard } from './SimEnhancements'
import { SavedSims } from './SimSaved'
import {
  kindLabel,
  nextLeft,
  omniumPicks,
  omniumString,
  savedSuggestTarget,
  saveSuggestTarget,
  type ShownLeft,
  simOptionsError,
} from './simOptions'
import { Skeleton } from './Skeleton'
import { tip } from './tipAttrs'

/** How often the engine download and a running sim are asked after. */
const POLL_MS = 1000

/** The kinds of sim the page offers, each with its hover text. */
const KINDS: [SimKind, string][] = [
  ['quick', 'One DPS number with each ability and buff'],
  ['topgear', 'The best combination of the gear you carry'],
  ['drops', "Each piece the season's raid and dungeons drop, by its gain"],
  ['stats', 'What a point of each stat is worth for your character'],
]

/** The engine on one line: what is installed (or the plain words about the download), with its buttons. */
function EngineBar(props: {
  engine: SimEngineStatus | null
  busy: boolean
  act: (call: () => Promise<unknown>) => void
  onEngine: (e: SimEngineStatus) => void
}) {
  const { engine, busy, act, onEngine } = props
  const [ownPath, setOwnPath] = useState('')
  const locked = !engine || engine.installing || busy
  return (
    <section className="panel sim-engine">
      <div className="sim-engine-row">
        {engine?.installed ? (
          <span className="muted">
            SimulationCraft {engine.version} for WoW {engine.patch} · {sizeText(engine.size ?? 0)}
            {engine.builtMS ? ` · built ${new Date(engine.builtMS).toLocaleDateString()}` : ''}
            {engine.updatedMS ? ` · updated ${new Date(engine.updatedMS).toLocaleString()}` : ''}
          </span>
        ) : (
          <span className="muted">
            Sims run on this PC with SimulationCraft, which is not ours. The download is the official
            nightly, kept on this app's GitHub release and checked against its checksum. Your character stays on this
            PC: only its gear's item IDs go to Wowhead, for their names and icons.
          </span>
        )}
        <button disabled={locked} onClick={() => act(async () => onEngine(await installSimEngine()))}>
          {engine?.installing ? 'Downloading…' : engine?.installed ? 'Update the engine' : 'Download the engine'}
        </button>
      </div>
      {engine?.stale && <p className="warning">This build is old; a patch has probably made it wrong. Update it.</p>}
      {engine?.error && <p className="warning">{engine.error}</p>}
      <details>
        <summary className="muted small">Use a simc.exe you already have</summary>
        <div className="actions">
          <input value={ownPath} placeholder="C:/Tools/simc/simc.exe" onChange={(e) => setOwnPath(e.target.value)} />
          <button
            disabled={locked || !ownPath.trim()}
            onClick={() => act(async () => onEngine(await installSimEngine(ownPath.trim())))}
          >
            Use this file
          </button>
        </div>
      </details>
    </section>
  )
}

/** A time estimate in words ("45 s", "12 min"). */
const aboutText = (seconds: number) => (seconds < 90 ? `${seconds} s` : `${Math.round(seconds / 60)} min`)

/** About how long a running sim has left in words (nextLeft); empty before anything says. */
function leftText(shown: ShownLeft | undefined): string {
  if (shown?.left == null) return ''
  return shown.left < 1 ? 'almost done' : `about ${aboutText(Math.round(shown.left))} left`
}

/** The sims running now: each with its bar and time left, to watch (its progress card) or cancel. */
function RunningSims(props: {
  runs: Sim[]
  lefts: Record<string, ShownLeft>
  onWatch: (id: string) => void
  onCancel: (id: string) => void
}) {
  return (
    <section className="panel sim-running">
      <h2>Running</h2>
      <ul className="sim-saved">
        {props.runs.map((r) => (
          <li key={r.id}>
            <button className="link" onClick={() => props.onWatch(r.id)}>
              <strong>{r.character}</strong> · {kindLabel(r.kind)}
            </button>
            <progress value={r.progress} max={100} aria-label="Sim progress" />
            <span className="muted small">
              {r.progress}% {leftText(props.lefts[r.id])}
            </span>
            <button className="delete" aria-label="Cancel" onClick={() => props.onCancel(r.id)}>
              ×
            </button>
          </li>
        ))}
      </ul>
    </section>
  )
}

/** Whether the pasted character is the one a finished sim was of, wearing the same items: a suggestion's item index
 * points into that export, and a newer paste can have other items at the same places. */
const sameGear = (profile: SimProfile | null, sim: Sim) =>
  !!profile &&
  profile.name === sim.character &&
  !!sim.result?.equipped?.every((it, i) => profile.equipped[i]?.slot === it.slot && profile.equipped[i]?.id === it.id)

/** What a request makes, in words: the runs besides the sim's own (of the max that run) and about how long it all
 * takes, or why the app would refuse it. */
function PlanText(props: { plan: { plan: SimPlan | null; error: string | null }; kind: SimKind; choices: SimChoices }) {
  const { plan, kind, choices } = props
  const refusal = plan.error ?? plan.plan?.error
  if (refusal) return <p className="warning">{refusal}</p>
  if (!plan.plan) return <p className="muted small">Estimating the time…</p>
  const { combos, seconds } = plan.plan
  const counted: Partial<Record<SimKind, [string, number]>> = {
    topgear: ['combination', choices.maxCombos],
    drops: ['drop', choices.maxDrops],
    quick: ['compared', choices.maxCompare],
  }
  const [noun, max] = counted[kind] ?? ['', 0]
  const what = combos > 0 && noun ? `${combos} ${noun}${combos > 1 && noun !== 'compared' ? 's' : ''} of ${max} · ` : ''
  return (
    <p className="muted small">
      {what}about {aboutText(seconds)}
    </p>
  )
}

/** What Suggest would cost, in words (about how long, and once the Warcraft Logs points of reading the top logs), or
 * why the app refuses it; without Warcraft Logs it offers the Stat Weights sim, which needs none. */
function SuggestText({ plan, onStats }: { plan: { plan: SimPlan | null; error: string | null }; onStats: () => void }) {
  const refusal = plan.error ?? plan.plan?.error
  if (refusal) {
    return (
      <>
        <p className="warning">{refusal}</p>
        {plan.plan?.needsLogs && <button onClick={onStats}>Run Stat Weights instead</button>}
      </>
    )
  }
  if (!plan.plan) return null
  const { seconds, points } = plan.plan
  return (
    <p className="muted small">
      About {aboutText(seconds)}
      {points ? `, and about ${points} Warcraft Logs points once, to read the top players' logs` : ''}
    </p>
  )
}

/** The fight's options and for a Droptimizer where the drops come from and whether they are upgraded, with the Sim
 * button and a running sim's progress, in the panel beside the gear. */
function OptionsPanel(props: {
  kind: SimKind
  choices: SimChoices
  options: SimOptions
  setOptions: (o: SimOptions) => void
  difficulty: string
  setDifficulty: (d: string) => void
  keyLevel: number
  setKeyLevel: (k: number) => void
  optionsError: string | null
  upgrade: string
  setUpgrade: (u: string) => void
  catalystCharges: number | null
  setCatalystCharges: (n: number) => void
  plan: { plan: SimPlan | null; error: string | null } | null
  suggestPlan: { plan: SimPlan | null; error: string | null } | null
  suggestTarget: string
  setSuggestTarget: (name: string) => void
  why: string | null
  canRun: boolean
  onRun: () => void
  onSuggest: (kind: SimKind) => void
}) {
  const { kind, choices, options, setOptions, optionsError } = props
  const scenario = choices.scenarios.find(
    (s) => s.fightStyle === options.fightStyle && s.length === options.length && s.targets === options.targets,
  )
  return (
    <aside className="panel sim-options">
      <h2>Fight</h2>
      <label>
        Scenario
        <select
          value={scenario?.name ?? ''}
          onChange={(e) => {
            const s = choices.scenarios.find((c) => c.name === e.target.value)
            if (s) setOptions({ ...options, fightStyle: s.fightStyle, length: s.length, targets: s.targets })
          }}
        >
          <option value="" disabled>
            Custom: the fight below
          </option>
          {choices.scenarios.map((s) => (
            <option key={s.name}>{s.name}</option>
          ))}
        </select>
      </label>
      <label>
        Style
        <select value={options.fightStyle} onChange={(e) => setOptions({ ...options, fightStyle: e.target.value })}>
          {choices.fightStyles.map((s) => (
            <option key={s}>{s}</option>
          ))}
        </select>
      </label>
      <label>
        Length (s)
        <input
          type="number"
          value={options.length}
          onChange={(e) => setOptions({ ...options, length: Number(e.target.value) })}
        />
      </label>
      <label>
        Targets
        <input
          type="number"
          value={options.targets}
          onChange={(e) => setOptions({ ...options, targets: Number(e.target.value) })}
        />
      </label>
      <label>
        Accuracy
        <select value={options.accuracy} onChange={(e) => setOptions({ ...options, accuracy: e.target.value })}>
          {choices.accuracies.map((a) => (
            <option key={a}>{a}</option>
          ))}
        </select>
      </label>
      <label className="inline">
        <input
          type="checkbox"
          checked={!options.noRaidBuffs}
          onChange={(e) => setOptions({ ...options, noRaidBuffs: !e.target.checked || undefined })}
        />
        Raid buffs (simc's default: Bloodlust, Arcane Intellect, …)
      </label>
      {kind === 'topgear' && (
        <label>
          Catalyst charges
          <select
            value={props.catalystCharges ?? choices.maxCatalystCharges}
            onChange={(e) => props.setCatalystCharges(Number(e.target.value))}
          >
            {Array.from({ length: choices.maxCatalystCharges + 1 }, (_, n) => (
              <option key={n} value={n}>
                {n === 0 ? 'None (no conversions)' : n}
              </option>
            ))}
          </select>
        </label>
      )}
      {kind === 'drops' && (
        <>
          <label>
            Raid difficulty
            <select value={props.difficulty} onChange={(e) => props.setDifficulty(e.target.value)}>
              {choices.difficulties.map((d) => (
                <option key={d}>{d}</option>
              ))}
            </select>
          </label>
          <label>
            Mythic+ key
            <select value={props.keyLevel} onChange={(e) => props.setKeyLevel(Number(e.target.value))}>
              {choices.keyLevels.map((k) => (
                <option key={k} value={k}>
                  {k === 0 ? 'Mythic 0' : `+${k}`}
                </option>
              ))}
            </select>
          </label>
          <label>
            Upgrade drops
            <select value={props.upgrade} onChange={(e) => props.setUpgrade(e.target.value)}>
              <option value="">As dropped</option>
              <option value="max">Fully, with Ascendant Venomstones</option>
              <option value="match">To the rank of the gear you wear</option>
              {Array.from({ length: choices.upgradeRanks }, (_, i) => (
                <option key={i} value={String(i + 1)}>
                  To rank {i + 1}
                </option>
              ))}
            </select>
          </label>
        </>
      )}
      {optionsError && <p className="warning">{optionsError}</p>}
      {props.plan && <PlanText plan={props.plan} kind={kind} choices={choices} />}
      {props.why && <p className="warning">{props.why}</p>}
      <div className="actions">
        <button className="sim-run" disabled={!props.canRun || !!optionsError} onClick={props.onRun}>
          Sim
        </button>
      </div>
      <section className="sim-suggest">
        <h3>Suggest</h3>
        <p className="muted small">
          What the top players of your spec do that you don't, each change confirmed by a sim on one target and on
          AoE.
        </p>
        <label>
          Compare with the top logs of
          <select value={props.suggestTarget} onChange={(e) => props.setSuggestTarget(e.target.value)}>
            {choices.suggestTargets.map((t) => (
              <option key={t.name}>{t.name}</option>
            ))}
          </select>
        </label>
        {props.suggestPlan && <SuggestText plan={props.suggestPlan} onStats={() => props.onSuggest('stats')} />}
        <div className="actions">
          <button
            className="sim-run"
            disabled={!!props.why || !!optionsError || !props.suggestPlan?.plan || !!props.suggestPlan.plan.error}
            onClick={() => props.onSuggest('suggest')}
          >
            Suggest
          </button>
        </div>
      </section>
    </aside>
  )
}

/** The Sim tab (desktop app only): a compact engine bar, the pasted /simc export read into a character with its gear
 * laid out like Raidbots, the kind of sim (Quick Sim, Top Gear or Droptimizer) with its options beside
 * the gear, the Omnium Folio, the running sim's progress, its result and the saved sims. */
export function SimPage() {
  const [choices, setChoices] = useState<SimChoices | null>(null)
  const [engine, setEngine] = useState<SimEngineStatus | null>(null)
  const [text, setText] = useState('')
  const [profile, setProfile] = useState<SimProfile | null>(null)
  const [kind, setKind] = useState<SimKind>('quick')
  const [options, setOptions] = useState<SimOptions | null>(null)
  const [reference, setReference] = useState<SimReference | null>(null)
  const [equipped, setEquipped] = useState<number[]>([])
  const [bags, setBags] = useState<number[]>([])
  const [variants, setVariants] = useState<SimVariant[]>([])
  const [consumables, setConsumables] = useState<SimConsumables>({})
  const [omnium, setOmnium] = useState<number[]>([])
  const [difficulty, setDifficulty] = useState('')
  const [keyLevel, setKeyLevel] = useState(0)
  const [upgrade, setUpgrade] = useState('')
  const [custom, setCustom] = useState('')
  const [chargesPicked, setChargesPicked] = useState<number | null>(null) // null: all the charges (no limit)
  const [enhancements, setEnhancements] = useState<SimEnhancements>({})
  const [targetPick, setTargetPick] = useState<string | null>(null) // null: the character's saved Suggest target
  const [runs, setRuns] = useState<Sim[]>([]) // the sims running now, as last polled
  const [watching, setWatching] = useState<string | null>(null) // the running sim whose progress card shows
  const estimates = useRef<Record<string, number>>({}) // seconds each started sim's plan said
  const [lefts, setLefts] = useState<Record<string, ShownLeft>>({}) // each running sim's time left, as last polled
  const [sim, setSim] = useState<Sim | null>(null) // the finished sim shown
  const [saved, setSaved] = useState<Sim[]>([])
  const [error, setError] = useState<string | null>(null)
  const info = useItemInfo(
    profile && choices ? [...profile.equipped, ...profile.bags] : [],
    choices?.maxItemLookups ?? 0,
  )

  const readSaved = useCallback(
    (watchNewest = false) =>
      fetchSims().then((list) => {
        const running = list.filter((s) => s.state === 'running')
        setSaved(list.filter((s) => s.state !== 'running'))
        setRuns(running)
        const at = Date.now()
        setLefts((prev) =>
          Object.fromEntries(running.map((r) => [r.id, nextLeft(r, estimates.current[r.id] ?? 0, prev[r.id], at)])),
        )
        if (watchNewest && running.length > 0) setWatching(running[0].id)
      }, () => undefined),
    [],
  )
  useEffect(() => {
    const controller = new AbortController()
    fetchSimChoices(controller.signal).then(
      (c) => {
        setChoices(c)
        setOptions(c.defaults)
        setDifficulty(c.defaultDifficulty)
        setKeyLevel(c.defaultKeyLevel)
      },
      (err: unknown) => !controller.signal.aborted && setError(errorText(err)),
    )
    fetchSimReference(controller.signal).then(
      setReference,
      (err: unknown) => !controller.signal.aborted && setError(errorText(err)),
    )
    fetchSimEngine(controller.signal).then(
      setEngine,
      (err: unknown) => !controller.signal.aborted && setError(errorText(err)),
    )
    void readSaved(true)
    return () => controller.abort()
  }, [readSaved])
  useEffect(() => {
    if (!engine?.installing) return
    const timer = setInterval(
      () => void fetchSimEngine().then(setEngine, (err: unknown) => setError(errorText(err))),
      POLL_MS,
    )
    return () => clearInterval(timer)
  }, [engine?.installing])
  const polling = runs.length > 0
  useEffect(() => {
    if (!polling) return
    const timer = setInterval(() => void readSaved(), POLL_MS)
    return () => clearInterval(timer)
  }, [polling, readSaved])
  useEffect(() => {
    // The sim being watched is no longer running: show its report (or why it ended).
    if (!watching || runs.some((r) => r.id === watching)) return
    void fetchSim(watching).then(
      (s) => {
        // started a moment ago, not in the last poll yet: list it, so the poll runs even if it was the only one
        if (s.state === 'running') {
          setRuns((all) => (all.some((r) => r.id === s.id) ? all : [s, ...all]))
          return
        }
        setSim(s)
        setWatching(null)
      },
      (err: unknown) => setError(errorText(err)),
    )
  }, [watching, runs])

  const act = (call: () => Promise<unknown>) => {
    setError(null)
    call().catch((err: unknown) => setError(errorText(err)))
  }
  const read = () =>
    act(async () => {
      const p = await parseSimExport(text)
      setProfile(p)
      setEquipped(p.equipped.map((_, i) => i)) // worn items are candidates until unticked
      setBags([])
      setVariants([])
      setOmnium(omniumPicks(p.omnium))
      setTargetPick(null)
    })
  const character = profile ? `${profile.name}-${profile.server}` : ''
  const suggestTarget =
    choices && profile
      ? (choices.suggestTargets.find((t) => t.name === targetPick) ?? savedSuggestTarget(choices, character))
      : undefined
  const same = (a: SimVariant, b: SimVariant) => JSON.stringify(a) === JSON.stringify(b)
  const optionsError = choices && options ? simOptionsError(options, choices) : null
  const request = (k: SimKind = kind): SimRequest | null => {
    if (!options) return null
    const mine = omniumString(reference?.omnium ?? [], omnium)
    const theirs = omniumString(reference?.omnium ?? [], omniumPicks(profile?.omnium))
    return {
      kind: k,
      export: text,
      options,
      consumables,
      ...(custom.trim() && { custom }),
      ...(mine !== theirs && { omnium: mine }),
      ...(k === 'topgear' && {
        equipped,
        bags,
        variants,
        enhancements,
        catalystCharges: chargesPicked ?? choices?.maxCatalystCharges,
      }),
      ...(k === 'drops' && { difficulty, keyLevel, upgrade }),
      ...(k === 'suggest' && suggestTarget && { target: suggestTarget.target }),
    }
  }
  const planRequest = profile ? request() : null
  const plan = usePlan(planRequest)
  const suggestPlan = usePlan(profile ? request('suggest') : null)
  const start = (k: SimKind = kind) =>
    act(async () => {
      const req = request(k)
      if (!req) return
      const started = await startSim(req)
      const asked = k === 'suggest' ? suggestPlan : plan
      estimates.current[started.id] = asked.plan?.seconds ?? 0
      setRuns((all) => [started, ...all.filter((r) => r.id !== started.id)])
      setWatching(started.id)
    })
  // Opens Top Gear with a suggestion's variant added and its item ticked.
  const tryVariant = (v: SimVariant) => {
    setSim(null)
    setKind('topgear')
    setVariants((all) => (all.some((o) => same(o, v)) ? all : [...all, v]))
    const [list, set] = v.worn ? [equipped, setEquipped] : [bags, setBags]
    if (!list.includes(v.index)) set([...list, v.index])
  }
  const toggle = (worn: boolean, i: number) => {
    const [list, set] = worn ? [equipped, setEquipped] : [bags, setBags]
    set(list.includes(i) ? list.filter((n) => n !== i) : [...list, i])
  }

  if (!choices || !options || !reference) {
    return error ? <p className="warning">{error}</p> : <Skeleton height={320} lines={5} />
  }
  const pick =
    kind === 'topgear'
      ? {
          equipped,
          bags,
          onToggle: toggle,
          reference,
          variants,
          notes: plan.plan?.notes ?? [],
          onAddVariant: (v: SimVariant) => setVariants((all) => (all.some((o) => same(o, v)) ? all : [...all, v])),
          onRemoveVariant: (v: SimVariant) => setVariants((all) => all.filter((o) => !same(o, v))),
        }
      : undefined
  const planBlocks = !!plan.error || !!plan.plan?.error || (kind === 'topgear' && !plan.plan)
  // Why Sim is off, in words (the plan says the request's own refusal beside the button).
  const whyNot = (): string | null => {
    if (!engine?.installed) return 'Download the engine first.'
    if (engine.installing) return 'The engine is still downloading.'
    if (runs.length >= choices.maxParallel) {
      return `${runs.length} sims are running, the most at once: wait for one to end, or cancel it.`
    }
    return null
  }
  const why = whyNot()
  const watched = runs.find((r) => r.id === watching)
  if (watched) {
    return (
      <main className="page settings sim">
        <section className="panel sim-progress">
          <h2>{kindLabel(watched.kind)}</h2>
          <p className="muted">
            {watched.character} · {watched.spec}
          </p>
          <progress value={watched.progress} max={100} aria-label="Sim progress" />
          {watched.stage && <p className="muted small">{watched.stage}</p>}
          <p>
            {watched.progress}%
            <span className="muted"> · {leftText(lefts[watched.id]) || 'starting'}</span>
          </p>
          {runs.length > 1 && <p className="muted small">{runs.length} sims are running; they share the PC.</p>}
          <div className="actions">
            <button onClick={() => setWatching(null)}>Run another sim</button>
            <button onClick={() => act(() => removeSim(watched.id))}>Cancel</button>
          </div>
        </section>
        {error && <p className="warning">{error}</p>}
      </main>
    )
  }
  if (sim) {
    return (
      <main className="page settings sim">
        <div className="actions">
          <button onClick={() => setSim(null)}>← Back to the sim</button>
        </div>
        {error && <p className="warning">{error}</p>}
        {sim.state === 'failed' && <p className="warning">{sim.error}</p>}
        {sim.state === 'cancelled' && <p className="muted">Cancelled.</p>}
        <SimResult
          sim={sim}
          chunk={choices.maxItemLookups}
          onTry={sameGear(profile, sim) ? tryVariant : undefined}
        />
      </main>
    )
  }
  return (
    <main className="page settings sim">
      <EngineBar engine={engine} busy={runs.length > 0} act={act} onEngine={setEngine} />
      {runs.length > 0 && (
        <RunningSims
          runs={runs}
          lefts={lefts}
          onWatch={setWatching}
          onCancel={(id) => act(() => removeSim(id))}
        />
      )}

      {!profile && (
        <section className="panel">
          <h2>Character</h2>
          <label>
            Paste the SimulationCraft addon's export (type /simc in game, copy the window)
            <textarea rows={8} value={text} spellCheck={false} onChange={(e) => setText(e.target.value)} />
          </label>
          <div className="actions">
            <button className="sim-run" disabled={!text.trim()} onClick={read}>
              Read it
            </button>
          </div>
        </section>
      )}

      {profile && (
        <>
          <section className="panel">
            <SimHeader profile={profile} />
            <div className="actions">
              <button
                onClick={() => {
                  setProfile(null)
                  setSim(null)
                }}
              >
                Paste another character
              </button>
            </div>
          </section>
          <div className="sim-tabs" role="group" aria-label="Kind of sim">
            {KINDS.map(([k, hint]) => (
              <button
                key={k}
                aria-pressed={kind === k}
                onClick={() => setKind(k)}
                {...tip(hint)}
              >
                {kindLabel(k)}
              </button>
            ))}
          </div>
          <div className="sim-layout">
            <section className="panel sim-gear">
              <h2>{kind === 'topgear' ? 'Gear to try' : 'Worn gear'}</h2>
              {kind === 'topgear' && (
                <p className="muted small">
                  Tick what it may wear, highest item level first: worn items stay candidates, carried ones are added.
                  Each item can also be tried upgraded, with a socket and gem, or with an enchant. At most{' '}
                  {choices.maxCombos} combinations run.
                </p>
              )}
              <SimGear profile={profile} order={choices.gearGroups} info={info} pick={pick} gems={reference.gems} />
            </section>
            <OptionsPanel
              kind={kind}
              choices={choices}
              options={options}
              setOptions={setOptions}
              difficulty={difficulty}
              setDifficulty={setDifficulty}
              keyLevel={keyLevel}
              setKeyLevel={setKeyLevel}
              optionsError={optionsError}
              upgrade={upgrade}
              setUpgrade={setUpgrade}
              catalystCharges={chargesPicked}
              setCatalystCharges={setChargesPicked}
              plan={planRequest ? plan : null}
              suggestPlan={profile ? suggestPlan : null}
              suggestTarget={suggestTarget?.name ?? ''}
              setSuggestTarget={(name) => {
                setTargetPick(name)
                saveSuggestTarget(character, name)
              }}
              why={why}
              canRun={!why && !planBlocks}
              onRun={() => start()}
              onSuggest={start}
            />
          </div>
          {kind === 'topgear' && (
            <EnhancementsCard reference={reference} value={enhancements} onChange={setEnhancements} />
          )}
          <CustomCard
            value={custom}
            onChange={setCustom}
            error={custom.trim() && plan.error?.includes('custom') ? plan.error : null}
          />
          <ConsumablesCard
            kinds={choices.consumableKinds}
            reference={reference}
            value={consumables}
            onChange={setConsumables}
          />
          {reference.omnium.length > 0 && (
            <OmniumFolio
              nodes={reference.omnium}
              picks={omnium}
              original={omniumPicks(profile.omnium)}
              onChange={setOmnium}
            />
          )}
        </>
      )}

      {error && <p className="warning">{error}</p>}

      {saved.length > 0 && (
        <section className="panel">
          <h2>Saved sims</h2>
          <SavedSims
            sims={saved}
            onOpen={(id) => act(async () => setSim(await fetchSim(id)))}
            onDelete={(id) =>
              act(async () => {
                await removeSim(id)
                await readSaved()
              })
            }
          />
        </section>
      )}
    </main>
  )
}
