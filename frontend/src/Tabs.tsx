import { type ReactNode, useState } from 'react'

/** Lists under tabs, one shown at a time; initial is the one open first, or open, when the page decides which. All sit
 * in one grid cell, so the tabs are as tall as the tallest and what is under them never moves when another tab opens.
 * onOpen hears of the one picked. A tab's name can change (it holds a count); its place is its identity. */
export function Tabs(props: {
  tabs: { name: string; body: ReactNode }[]
  initial?: number
  open?: number
  onOpen?: (index: number) => void
}) {
  const { tabs, initial = 0, onOpen } = props
  const [picked, setPicked] = useState(initial)
  const open = props.open ?? picked
  return (
    <section className="tabs">
      <div role="tablist">
        {tabs.map((t, i) => (
          <button
            key={i}
            role="tab"
            aria-selected={i === open}
            onClick={() => {
              setPicked(i)
              onOpen?.(i)
            }}
          >
            {t.name}
          </button>
        ))}
      </div>
      <div className="tab-panels">
        {tabs.map((t, i) => (
          <div key={i} role="tabpanel" className={i === open ? undefined : 'tab-off'} inert={i !== open}>
            {t.body}
          </div>
        ))}
      </div>
    </section>
  )
}
