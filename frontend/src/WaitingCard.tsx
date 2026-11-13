import { useEffect, useState, type ReactNode } from 'react'
import { fetchSettings, type SettingsView } from './api'
import { useNow } from './useLiveKey'
import { stateLine, type TonightRow } from './waiting'

/** The three things a waiting page needs, in order. */
const STEPS = [
  'Combat logging is on',
  'A raid or key is started in game',
  'The first pull ends, then it shows here',
]

/** An empty Raid or M+ page: what it waits for, where the app watches and when the game last wrote, a checklist
 * with the first unmet step highlighted, and tonight's rows when there are any (children: lines above, such as the
 * last key). */
export function WaitingCard({ title, hint, rows, children }: {
  title: string
  hint: string
  rows: TonightRow[]
  children?: ReactNode
}) {
  const [view, setView] = useState<SettingsView | null>(null)
  const now = useNow(60_000)
  useEffect(() => {
    const controller = new AbortController()
    fetchSettings(controller.signal).then(setView, () => undefined)
    return () => controller.abort()
  }, [])
  const line = stateLine(view, now)
  const loggingOn = !!line && !line.warn
  const current = loggingOn ? 1 : 0
  return (
    <section className="waiting-card">
      <h1>{title}</h1>
      <p className="muted">{hint}</p>
      {line && <p className={line.warn ? 'warning' : 'muted small'}>{line.text}</p>}
      {children}
      <ol className="waiting-steps">
        {STEPS.map((step, i) => (
          <li key={step} className={i < current ? 'done' : i === current ? 'current' : undefined}>
            {step}
          </li>
        ))}
      </ol>
      {rows.length > 0 && (
        <>
          <h2>Tonight so far</h2>
          <ul className="waiting-tonight">
            {rows.map((r) => (
              <li key={r.id}>
                <button className="item" onClick={r.onOpen}>
                  <span>{r.label}</span>
                  <span className={r.good ? 'good' : 'muted'}>{r.result}</span>
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}
