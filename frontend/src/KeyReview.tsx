import { useEffect, useState } from 'react'
import { fetchReview, makeReview, type KeyReview as Review, type KeyView } from './api'
import { amount } from './format'
import { PlayerFocus } from './PlayerFocus'

/** How often, and how long, a key that just ended is asked for the review the site makes on its own. */
const WAIT_POLL_MS = 5000
const WAIT_MS = 90_000

/** A finished key's review: each player's numbers against their spec's top runs, their usual here and the party,
 * what to work on, and one thing done well, beside one player's key up close. Made by the site when the key ends, or
 * here on request; onReview hears of it once shown. */
export function KeyReview(props: { keyView: KeyView; justEnded: boolean; onReview: (review: Review) => void }) {
  const { keyView: key, justEnded, onReview } = props
  const [review, setReview] = useState<Review | null>(null)
  useEffect(() => {
    if (review) onReview(review)
  }, [review, onReview])
  const [state, setState] = useState<'loading' | 'waiting' | 'none' | 'making'>('loading')
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const controller = new AbortController()
    const started = Date.now()
    let timer: ReturnType<typeof setTimeout> | undefined
    const read = () =>
      fetchReview(key.start, controller.signal)
        .then((r) => {
          if (r) return setReview(r)
          const wait = justEnded && Date.now() - started < WAIT_MS
          setState(wait ? 'waiting' : 'none')
          if (wait) timer = setTimeout(() => void read(), WAIT_POLL_MS)
        })
        .catch((err: unknown) => {
          if (!controller.signal.aborted) setError(err instanceof Error ? err.message : String(err))
        })
    void read()
    return () => {
      controller.abort()
      clearTimeout(timer)
    }
  }, [key.start, justEnded])

  const make = () => {
    setState('making')
    setError(null)
    makeReview(key.start).then(setReview, (err: unknown) => {
      setError(err instanceof Error ? err.message : String(err))
      setState('none')
    })
  }

  if (!review) {
    return (
      <section className="panel">
        <h2>Key review</h2>
        {state === 'waiting' && <p className="muted">Reviewing against Warcraft Logs…</p>}
        {state === 'making' && <p className="muted">Reviewing against Warcraft Logs (a few points)…</p>}
        {state === 'none' && (
          <button className="action" onClick={make}>
            Review this key (spends Warcraft Logs points)
          </button>
        )}
        {error && <p className="warning">{error}</p>}
        <PlayerFocus keyView={key} />
      </section>
    )
  }
  const columns = review.players[0]?.metrics ?? []
  return (
    <section className="panel review">
      <h2>Key review</h2>
      <p className="muted small">
        {review.baseline}
        {review.party && ` · ${review.party}`}
      </p>
      {review.notes?.map((n) => (
        <p key={n}>{n}</p>
      ))}
      <div className="table-scroll">
        <table className="review-table">
          <thead>
            <tr>
              <th>Player</th>
              {columns.map((n) => (
                <th key={n.name} title={n.help}>
                  {n.name}
                  {n.totalName && ` / ${n.totalName}`}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {review.players.map((p) => (
              <tr key={p.player}>
                <td>
                  <b>{p.player}</b>
                  <div className="muted small">{p.spec}</div>
                </td>
                {p.metrics.map((m) => {
                  const show = (v: number) => (m.amount ? amount(v) : String(Math.round(v)))
                  return (
                    <td key={m.name}>
                      {show(m.value)}
                      {m.total ? ` / ${amount(m.total)}` : ''}
                      <div className="muted small">
                        {m.top > 0 && `${Math.round((100 * m.value) / m.top)}% of top · `}#{m.partyRank}
                      </div>
                    </td>
                  )
                })}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="review-split">
        <div>
          {review.players.map((p) => (
            <div key={p.player} className="review-player">
              <b>{p.player}</b>
              <p className="good">✓ {p.positive}</p>
              {p.advice.map((a) => (
                <p key={a}>• {a}</p>
              ))}
            </div>
          ))}
        </div>
        <PlayerFocus keyView={key} />
      </div>
    </section>
  )
}
