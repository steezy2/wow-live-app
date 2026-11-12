import { useEffect, useState } from 'react'
import { fetchCharacterParses, type CharacterParses as Parses } from './api'
import { counted } from './format'

/** The Recap's panel of the saved character's Warcraft Logs parses this season, Mythic+ dungeons then raid bosses:
 * best and median, runs and spec, with a link to its page; nothing while there's no character. */
export function CharacterParses({ character }: { character: string }) {
  // The last answer, for the character it was asked for: the parses or why they couldn't be read.
  const [answer, setAnswer] = useState<{ character: string; parses?: Parses; problem?: string }>({ character: '' })
  useEffect(() => {
    if (!character) return
    const controller = new AbortController()
    fetchCharacterParses(controller.signal).then(
      (parses) => setAnswer({ character, parses }),
      (err: unknown) => {
        if (!controller.signal.aborted)
          setAnswer({ character, problem: err instanceof Error ? err.message : String(err) })
      },
    )
    return () => controller.abort()
  }, [character])
  const { parses, problem } = answer.character === character ? answer : { parses: undefined, problem: undefined }
  if (!character || (!parses && !problem)) return null
  const page = parses && (
    <a href={parses.url} target="_blank" rel="noreferrer">
      {character} on Warcraft Logs
    </a>
  )
  return (
    <section className="panel">
      <h2>Parses this season</h2>
      {!parses ? (
        <p className="muted">{problem}</p>
      ) : !parses.found ? (
        <p className="muted">Not on Warcraft Logs yet ({page})</p>
      ) : (
        <>
          <p>{page}</p>
          {parses.parses.length === 0 && <p className="muted">No ranked runs or kills yet</p>}
          <ul className="list">
            {parses.parses.map((p) => (
              <li key={p.kind + p.encounter}>
                <span>
                  {p.encounter} <span className="muted small">{p.kind === 'raid' ? 'raid' : 'M+'}</span>
                </span>
                <span>
                  best <b className={parseClass(p.best)}>{Math.floor(p.best)}</b> · median {Math.floor(p.median)}
                  <span className="muted small">
                    {' '}
                    · {counted(p.runs, p.kind === 'raid' ? 'kill' : 'run')} · {p.spec}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}

/** Warcraft Logs' colour band of a parse: grey, green, blue, purple, orange, pink, gold. */
function parseClass(parse: number): string {
  if (parse >= 100) return 'parse gold'
  if (parse >= 99) return 'parse pink'
  if (parse >= 95) return 'parse orange'
  if (parse >= 75) return 'parse purple'
  if (parse >= 50) return 'parse blue'
  if (parse >= 25) return 'parse green'
  return 'parse grey'
}
