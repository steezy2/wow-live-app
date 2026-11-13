import { useDelayed } from './useDelayed'

/** Grey bars standing in for content that is loading, in `height` px of space held from the start; the bars show
 * only if the load outlasts `LOADING_DELAY_MS`, so a quick load never flashes them. */
export function Skeleton({ height = 160, lines = 3 }: { height?: number; lines?: number }) {
  const slow = useDelayed(true)
  return (
    <div className="skeleton" style={{ height }} role="status" aria-label="Loading">
      {slow && Array.from({ length: lines }, (_, i) => <span key={i} className="skeleton-bar" />)}
    </div>
  )
}
