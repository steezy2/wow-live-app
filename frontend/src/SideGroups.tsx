import type { ReactNode } from 'react'
import { groupBy, periodLabel } from './format'

/** The side panel's items grouped by day, or by week past the last 7 days, each group a dropdown headed by its label
 * and count; the newest group and the one holding the picked item start open. */
export function SideGroups<T>({ items, time, count, isSelected, children }: {
  items: T[]
  time: (item: T) => number
  count: (group: T[]) => string
  isSelected: (item: T) => boolean
  children: (group: T[]) => ReactNode
}) {
  return [...groupBy(items, (item) => periodLabel(time(item)))].map(([label, group], i) => (
    <details key={label} className="side-group" open={i === 0 || group.some(isSelected)}>
      <summary>
        {label} <span className="muted small">{count(group)}</span>
      </summary>
      {children(group)}
    </details>
  ))
}
