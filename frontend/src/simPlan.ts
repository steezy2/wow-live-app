import { useEffect, useState } from 'react'
import { planSim, type SimPlan, type SimRequest } from './api'
import { errorText } from './format'

/** How long the request must stay as it is before the plan is asked for, so ticking several boxes asks once. */
const DEBOUNCE_MS = 300

/** What a Top Gear request would run (its gear combinations, the most one runs and about how long they take), asked
 * for again a moment after the request changes; null for no request (another kind) or before the answer. A request
 * the app refuses gives its words as error. */
export function usePlan(request: SimRequest | null): { plan: SimPlan | null; error: string | null } {
  const [answer, setAnswer] = useState<{ key: string; plan: SimPlan | null; error: string | null }>({
    key: '',
    plan: null,
    error: null,
  })
  const key = request ? JSON.stringify(request) : ''
  useEffect(() => {
    if (!request) return
    const controller = new AbortController()
    const timer = setTimeout(() => {
      planSim(request, controller.signal).then(
        (plan) => setAnswer({ key, plan, error: null }),
        (err: unknown) => !controller.signal.aborted && setAnswer({ key, plan: null, error: errorText(err) }),
      )
    }, DEBOUNCE_MS)
    return () => {
      clearTimeout(timer)
      controller.abort()
    }
    // the request is a new object each render; its text is what says it changed
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])
  return answer.key === key ? answer : { plan: null, error: null }
}
