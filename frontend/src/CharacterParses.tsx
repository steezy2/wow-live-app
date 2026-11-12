import { useEffect, useState } from 'react'
import { fetchCharacterParses, type CharacterParses as Parses } from './api'
import { counted } from './format'

/** The Recap's panel of the saved character's Warcraft Logs parses this season in Mythic+ dungeons or raid bosses
 * (kind): best and median, runs and spec, with a link to its page; nothing while there's no character. */
export function CharacterParses({ character, kind }: { character: string; kind: 'mplus' | 'raid' }) {
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
  const shown = parses?.parses.filter((p) => p.kind === kind) ?? []
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
          {shown.length === 0 && (
            <p className="muted">{kind === 'raid' ? 'No ranked kills yet' : 'No ranked runs yet'}</p>
          )}
          <ul className="list">
            {shown.map((p) => (
              <li key={p.kind + p.encounter}>
                <span>{p.encounter}</span>
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
