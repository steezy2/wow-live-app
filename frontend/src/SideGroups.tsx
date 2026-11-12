import { useState, type ReactNode } from 'react'
import { groupBy, periodLabel } from './format'

/** The side panel's items grouped by day, or by week past the last 7 days, each group a dropdown headed by its label
 * and count; the newest group and the one holding the picked item start open, and a group the user opened or closed
 * stays that way. children gets the group's label too. */
export function SideGroups<T>({ items, time, count, isSelected, children }: {
  items: T[]
  time: (item: T) => number
  count: (group: T[]) => string
  isSelected: (item: T) => boolean
  children: (group: T[], label: string) => ReactNode
}) {
  const [chosen, setChosen] = useState<Map<string, boolean>>(new Map())
  return [...groupBy(items, (item) => periodLabel(time(item)))].map(([label, group], i) => (
    <details
      key={label}
      className="side-group"
      open={chosen.get(label) ?? (i === 0 || group.some(isSelected))}
      onToggle={(e) => {
        const open = e.currentTarget.open
        setChosen((m) => (m.get(label) === open ? m : new Map(m).set(label, open)))
      }}
    >
      <summary>
        {label} <span className="muted small">{count(group)}</span>
      </summary>
      {children(group, label)}
    </details>
  ))
}
