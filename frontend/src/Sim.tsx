import { useCallback, useEffect, useState } from 'react'
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
  type Sim as SimRun,
  type SimChoices,
  type SimCompare,
  type SimConsumables,
  type SimEngineStatus,
  type SimKind,
  type SimOptions,
  type SimPlan,
  type SimProfile,
  type SimReference,
  type SimRequest,
  type SimVariant,
} from './api'
import { errorText, sizeText } from './format'
import { OmniumFolio } from './OmniumFolio'
import { ConsumableCompare, ConsumableSelects } from './SimConsumables'
import { SimGear, SimHeader } from './SimGear'
import { useItemInfo } from './simItems'
import { usePlan } from './simPlan'
import { SimResult } from './SimResult'
import { averageLevel, omniumPicks, omniumString, pickedConsumables, simOptionsError } from './simOptions'
import { Skeleton } from './Skeleton'
import { tip } from './tipAttrs'

/** How often the engine download and a running sim are asked after. */
const POLL_MS = 1000

const KINDS: [SimKind, string, string][] = [
  ['quick', 'Quick Sim', 'One DPS number with each ability and buff'],
  ['topgear', 'Top Gear', 'The best combination of the gear you carry'],
  ['drops', 'Droptimizer', "Each piece the season's raid and dungeons drop, by its gain"],
  ['consumables', 'Consumables', 'Flasks, food, potions, augment runes or weapon oils against each other'],
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
          </span>
        ) : (
          <span className="muted">
            Sims run on this PC with SimulationCraft, which is not ours. The download (about 116 MB) is the official
            nightly, kept on this app's GitHub release and checked against its checksum; nothing about your character
            is sent anywhere.
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

/** What a Top Gear request makes, in words: how many combinations (of the most that run) and about how long, or why the
 * app would refuse it. */
function PlanText({ plan }: { plan: { plan: SimPlan | null; error: string | null } }) {
  const refusal = plan.error ?? plan.plan?.error
  if (refusal) return <p className="warning">{refusal}</p>
  if (!plan.plan) return <p className="muted small">Counting the combinations…</p>
  const { combos, max, seconds } = plan.plan
  const about = seconds < 90 ? `${seconds} s` : `${Math.round(seconds / 60)} min`
  return (
    <p className="muted small">
      {combos} combination{combos > 1 ? 's' : ''} of {max} · about {about}
    </p>
  )
}

/** The fight's options, the consumables and for a Droptimizer where the drops come from, with the Sim button and a
 * running sim's progress, in the panel beside the gear. */
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
  reference: SimReference
  consumables: SimConsumables
  setConsumables: (c: SimConsumables) => void
  plan: { plan: SimPlan | null; error: string | null } | null
  canRun: boolean
  running: SimRun | null
  onRun: () => void
  onCancel: () => void
}) {
  const { kind, choices, options, setOptions, optionsError, running } = props
  return (
    <aside className="panel sim-options">
      <h2>Fight</h2>
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
        </>
      )}
      <h2>Consumables</h2>
      <ConsumableSelects
        kinds={choices.consumableKinds}
        reference={props.reference}
        value={props.consumables}
        onChange={props.setConsumables}
      />
      {optionsError && <p className="warning">{optionsError}</p>}
      {props.plan && <PlanText plan={props.plan} />}
      <button className="sim-run" disabled={!props.canRun || !!running || !!optionsError} onClick={props.onRun}>
        {running ? 'Simming…' : 'Sim'}
      </button>
      {running && (
        <div className="sim-progress">
          <progress value={running.progress} max={100} aria-label="Sim progress" /> {running.progress}%
          <button onClick={props.onCancel}>Cancel</button>
        </div>
      )}
    </aside>
  )
}

/** The Sim tab (desktop app only): a compact engine bar, the pasted /simc export read into a character with its gear
 * laid out like Raidbots, the kind of sim (Quick Sim, Top Gear or Droptimizer) with its options beside the gear, the
 * running sim's progress, its result and the saved sims. */
export function Sim() {
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
  const [compare, setCompare] = useState<SimCompare>({ kind: 'flask', values: [] })
  const [omnium, setOmnium] = useState<number[]>([])
  const [difficulty, setDifficulty] = useState('')
  const [keyLevel, setKeyLevel] = useState(0)
  const [sim, setSim] = useState<SimRun | null>(null)
  const [saved, setSaved] = useState<SimRun[]>([])
  const [error, setError] = useState<string | null>(null)
  const info = useItemInfo(profile ? [...profile.equipped, ...profile.bags] : [])

  const readSaved = useCallback(() => fetchSims().then(setSaved, () => undefined), [])
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
    fetchSimEngine(controller.signal).then(setEngine, () => undefined)
    void readSaved()
    return () => controller.abort()
  }, [readSaved])
  useEffect(() => {
    if (!engine?.installing) return
    const timer = setInterval(() => void fetchSimEngine().then(setEngine, () => undefined), POLL_MS)
    return () => clearInterval(timer)
  }, [engine?.installing])
  const running = sim?.state === 'running' ? sim : null
  const runningID = running?.id
  useEffect(() => {
    if (!runningID) return
    const timer = setInterval(
      () =>
        void fetchSim(runningID).then(
          (s) => {
            setSim(s)
            if (s.state !== 'running') void readSaved()
          },
          (err: unknown) => setError(errorText(err)),
        ),
      POLL_MS,
    )
    return () => clearInterval(timer)
  }, [runningID, readSaved])

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
    })
  const optionsError = choices && options ? simOptionsError(options, choices) : null
  const request = (): SimRequest | null => {
    if (!options) return null
    const mine = omniumString(reference?.omnium ?? [], omnium)
    const theirs = omniumString(reference?.omnium ?? [], omniumPicks(profile?.omnium))
    return {
      kind,
      export: text,
      options,
      consumables: pickedConsumables(consumables),
      ...(mine !== theirs && { omnium: mine }),
      ...(kind === 'topgear' && { equipped, bags, variants }),
      ...(kind === 'drops' && { difficulty, keyLevel }),
      ...(kind === 'consumables' && { compare }),
    }
  }
  const planRequest = kind === 'topgear' && profile ? request() : null
  const plan = usePlan(planRequest)
  const start = () =>
    act(async () => {
      const req = request()
      if (req) setSim(await startSim(req))
    })
  const toggle = (worn: boolean, i: number) => {
    const [list, set] = worn ? [equipped, setEquipped] : [bags, setBags]
    set(list.includes(i) ? list.filter((n) => n !== i) : [...list, i])
  }

  if (!choices || !options || !reference) {
    return error ? <p className="warning">{error}</p> : <Skeleton height={320} lines={5} />
  }
  const same = (a: SimVariant, b: SimVariant) => JSON.stringify(a) === JSON.stringify(b)
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
  const planBlocks = kind === 'topgear' && (!!plan.error || !plan.plan || !!plan.plan.error)
  const compareBlocks = kind === 'consumables' && compare.values.length === 0
  return (
    <main className="page settings sim">
      <EngineBar engine={engine} busy={!!running} act={act} onEngine={setEngine} />

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
            <SimHeader profile={profile} level={averageLevel(profile.equipped)} />
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
          <div className="sim-tabs" role="tablist">
            {KINDS.map(([k, label, hint]) => (
              <button
                key={k}
                role="tab"
                aria-selected={kind === k}
                className={kind === k ? 'active' : ''}
                onClick={() => setKind(k)}
                {...tip(hint)}
              >
                {label}
              </button>
            ))}
          </div>
          <div className="sim-layout">
            <section className="panel sim-gear">
              {kind === 'consumables' ? (
                <>
                  <h2>Consumables to compare</h2>
                  <p className="muted small">
                    Each ticked one is simmed in place of the one you pick for it (or simc's default); at most{' '}
                    {choices.maxCompare}.
                  </p>
                  <ConsumableCompare
                    kinds={choices.consumableKinds}
                    reference={reference}
                    compare={compare}
                    max={choices.maxCompare}
                    onChange={setCompare}
                  />
                </>
              ) : (
                <>
                  <h2>{kind === 'topgear' ? 'Gear to try' : 'Worn gear'}</h2>
                  {kind === 'topgear' && (
                    <p className="muted small">
                      Tick what it may wear, highest item level first: worn items stay candidates, carried ones are
                      added. Each item can also be tried upgraded, with a socket and gem, or with an enchant. At
                      most {choices.maxCombos} combinations run.
                    </p>
                  )}
                  <SimGear profile={profile} info={info} pick={pick} />
                </>
              )}
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
              reference={reference}
              consumables={consumables}
              setConsumables={setConsumables}
              plan={planRequest ? plan : null}
              canRun={!!engine?.installed && !planBlocks && !compareBlocks}
              running={running}
              onRun={start}
              onCancel={() => running && act(() => removeSim(running.id))}
            />
          </div>
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
      {sim && sim.state !== 'running' && (
        <>
          {sim.state === 'failed' && <p className="warning">{sim.error}</p>}
          {sim.state === 'cancelled' && <p className="muted">Cancelled.</p>}
          <SimResult sim={sim} />
        </>
      )}

      {saved.length > 0 && (
        <section className="panel">
          <h2>Saved sims</h2>
          <ul className="sim-saved">
            {saved.map((s) => (
              <li key={s.id}>
                <button className="link" onClick={() => act(async () => setSim(await fetchSim(s.id)))}>
                  <strong>{s.character}</strong> · {s.spec} · {KINDS.find(([k]) => k === s.kind)?.[1]}
                  {s.result ? ` · ${Math.round(s.result.dps).toLocaleString()} DPS` : ''}
                </button>
                <span className="muted small">{new Date(s.startedMS).toLocaleString()}</span>
                <button
                  className="delete"
                  aria-label="Delete"
                  onClick={() =>
                    act(async () => {
                      await removeSim(s.id)
                      await readSaved()
                    })
                  }
                >
                  ×
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </main>
  )
}
