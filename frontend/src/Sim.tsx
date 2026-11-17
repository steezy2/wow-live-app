import { useCallback, useEffect, useState } from 'react'
import {
  fetchSim,
  fetchSimChoices,
  fetchSimEngine,
  fetchSims,
  installSimEngine,
  parseSimExport,
  removeSim,
  startSim,
  type Sim as SimRun,
  type SimChoices,
  type SimEngineStatus,
  type SimKind,
  type SimOptions,
  type SimProfile,
} from './api'
import { sizeText } from './format'
import { SimResult } from './SimResult'
import { gearGroups, simOptionsError } from './simOptions'
import { Skeleton } from './Skeleton'
import { tip } from './tipAttrs'

/** How often the engine download and a running sim are asked after. */
const POLL_MS = 1000

const KINDS: [SimKind, string, string][] = [
  ['quick', 'Quick Sim', 'One DPS number with each ability and buff'],
  ['topgear', 'Top Gear', 'The best combination of the gear you carry'],
  ['drops', 'Droptimizer', "Each piece the season's raid and dungeons drop, by its gain"],
]

/** A message for an error thrown by a call. */
const errorText = (err: unknown) => (err instanceof Error ? err.message : String(err))

/** The Sim tab (desktop app only): the engine's status and download, a pasted /simc export, the kind of sim and its
 * options, the running sim's progress, its result, and the saved sims. */
export function Sim() {
  const [choices, setChoices] = useState<SimChoices | null>(null)
  const [engine, setEngine] = useState<SimEngineStatus | null>(null)
  const [ownPath, setOwnPath] = useState('')
  const [text, setText] = useState('')
  const [profile, setProfile] = useState<SimProfile | null>(null)
  const [kind, setKind] = useState<SimKind>('quick')
  const [options, setOptions] = useState<SimOptions | null>(null)
  const [equipped, setEquipped] = useState<number[]>([])
  const [bags, setBags] = useState<number[]>([])
  const [difficulty, setDifficulty] = useState('')
  const [keyLevel, setKeyLevel] = useState(0)
  const [sim, setSim] = useState<SimRun | null>(null)
  const [saved, setSaved] = useState<SimRun[]>([])
  const [error, setError] = useState<string | null>(null)

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
      (err: unknown) => setError(errorText(err)),
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
  const running = sim?.state === 'running' ? sim.id : null
  useEffect(() => {
    if (!running) return
    const timer = setInterval(
      () =>
        void fetchSim(running).then(
          (s) => {
            setSim(s)
            if (s.state !== 'running') void readSaved()
          },
          (err: unknown) => setError(errorText(err)),
        ),
      POLL_MS,
    )
    return () => clearInterval(timer)
  }, [running, readSaved])

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
    })
  const optionsError = choices && options ? simOptionsError(options, choices) : null
  const start = () =>
    act(async () => {
      if (!options) return
      setSim(
        await startSim({
          kind,
          export: text,
          options,
          ...(kind === 'topgear' && { equipped, bags }),
          ...(kind === 'drops' && { difficulty, keyLevel }),
        }),
      )
    })
  const toggle = (list: number[], set: (l: number[]) => void, i: number) =>
    set(list.includes(i) ? list.filter((n) => n !== i) : [...list, i])

  if (!choices || !options) return error ? <p className="warning">{error}</p> : <Skeleton height={320} lines={5} />
  return (
    <main className="page settings">
      <section className="panel">
        <h2>Engine</h2>
        {engine?.installed ? (
          <p className="muted">
            SimulationCraft {engine.version} for WoW {engine.patch}
            {engine.size ? ` · ${sizeText(engine.size)}` : ''}
            {engine.file ? ` · ${engine.file}` : ''}
            {engine.builtMS ? ` · built ${new Date(engine.builtMS).toLocaleDateString()}` : ''}
          </p>
        ) : (
          <p className="muted">
            Sims run on this PC with SimulationCraft, which is not ours. The download (about 116 MB) is the official
            nightly, kept on this app's GitHub release and checked against its checksum, and nothing about your
            character is sent anywhere.
          </p>
        )}
        {engine?.stale && <p className="warning">This build is old; a patch has probably made it wrong. Update it.</p>}
        {engine?.error && <p className="warning">{engine.error}</p>}
        <div className="actions">
          <button
            disabled={!engine || engine.installing || !!running}
            onClick={() => act(async () => setEngine(await installSimEngine()))}
          >
            {engine?.installing ? 'Downloading…' : engine?.installed ? 'Update the engine' : 'Download the engine'}
          </button>
        </div>
        <label>
          Or use a simc.exe you already have (its full path)
          <input value={ownPath} placeholder="C:/Tools/simc/simc.exe" onChange={(e) => setOwnPath(e.target.value)} />
        </label>
        <div className="actions">
          <button
            disabled={!engine || engine.installing || !!running || !ownPath.trim()}
            onClick={() => act(async () => setEngine(await installSimEngine(ownPath.trim())))}
          >
            Use this file
          </button>
        </div>
      </section>

      <section className="panel">
        <h2>Character</h2>
        <label>
          Paste the SimulationCraft addon's export (type /simc in game, copy the window)
          <textarea rows={6} value={text} onChange={(e) => {
              setText(e.target.value)
              setProfile(null) // the picked gear indexes belong to the text that was read
            }}
            spellCheck={false}
          />
        </label>
        <div className="actions">
          <button disabled={!text.trim()} onClick={read}>
            Read it
          </button>
          {profile && (
            <span>
              {profile.name} · {profile.spec} {profile.class} · {profile.equipped.length} worn,{' '}
              {profile.bags.length} carried
            </span>
          )}
        </div>
      </section>

      {profile && (
        <section className="panel">
          <h2>Sim</h2>
          <div className="actions">
            {KINDS.map(([k, label, hint]) => (
              <label key={k} className="inline" {...tip(hint)}>
                <input type="radio" name="sim-kind" checked={kind === k} onChange={() => setKind(k)} /> {label}
              </label>
            ))}
          </div>
          <div className="actions">
            <label>
              Fight
              <select
                value={options.fightStyle}
                onChange={(e) => setOptions({ ...options, fightStyle: e.target.value })}
              >
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
          </div>
          {optionsError && <p className="warning">{optionsError}</p>}
          {kind === 'topgear' && (
            <>
              <p className="muted">
                Tick what it may wear: worn items stay candidates, carried ones are added. At most{' '}
                {choices.maxCombos}{' '}
                combinations run.
              </p>
              {gearGroups(profile.equipped, profile.bags).map((g) => (
                <div key={g.label}>
                  <strong>{g.label}</strong>
                  {g.items.map(({ item, index, worn }) => (
                    <label key={`${worn}-${index}`} className="inline">
                      <input
                        type="checkbox"
                        checked={(worn ? equipped : bags).includes(index)}
                        onChange={() => (worn ? toggle(equipped, setEquipped, index) : toggle(bags, setBags, index))}
                      />
                      {item.name ?? `item ${item.id}`} {item.level ? `(${item.level})` : ''}
                      {worn && <span className="muted"> worn</span>}
                    </label>
                  ))}
                </div>
              ))}
            </>
          )}
          {kind === 'drops' && (
            <div className="actions">
              <label>
                Raid difficulty
                <select value={difficulty} onChange={(e) => setDifficulty(e.target.value)}>
                  {choices.difficulties.map((d) => (
                    <option key={d}>{d}</option>
                  ))}
                </select>
              </label>
              <label>
                Mythic+ key
                <select value={keyLevel} onChange={(e) => setKeyLevel(Number(e.target.value))}>
                  {choices.keyLevels.map((k) => (
                    <option key={k} value={k}>
                      {k === 0 ? 'Mythic 0' : `+${k}`}
                    </option>
                  ))}
                </select>
              </label>
            </div>
          )}
          <div className="actions">
            <button disabled={!engine?.installed || !!running || !!optionsError} onClick={start}>
              Run
            </button>
            {running && sim && (
              <>
                <progress value={sim.progress} max={100} aria-label="Sim progress" /> {sim.progress}%
                <button onClick={() => act(() => removeSim(sim.id))}>Cancel</button>
              </>
            )}
          </div>
        </section>
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
          <ul className="list">
            {saved.map((s) => (
              <li key={s.id}>
                <button onClick={() => act(async () => setSim(await fetchSim(s.id)))}>
                  {s.character} · {KINDS.find(([k]) => k === s.kind)?.[1]} · {new Date(s.startedMS).toLocaleString()}
                </button>
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
