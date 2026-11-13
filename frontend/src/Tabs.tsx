import { type ReactNode, useState } from 'react'

/** Lists under tabs, one shown at a time; initial is the one open first. All sit in one grid cell, so the tabs are as
 * tall as the tallest and what is under them never moves when another tab opens. onOpen hears of the one picked. */
export function Tabs(props: {
  tabs: { name: string; body: ReactNode }[]
  initial?: number
  onOpen?: (index: number) => void
}) {
  const { tabs, initial = 0, onOpen } = props
  const [open, setOpen] = useState(initial)
  return (
    <section className="tabs">
      <div role="tablist">
        {tabs.map((t, i) => (
          <button
            key={t.name}
            role="tab"
            aria-selected={i === open}
            onClick={() => {
              setOpen(i)
              onOpen?.(i)
            }}
          >
            {t.name}
          </button>
        ))}
      </div>
      <div className="tab-panels">
        {tabs.map((t, i) => (
          <div key={t.name} role="tabpanel" className={i === open ? undefined : 'tab-off'} inert={i !== open}>
            {t.body}
          </div>
        ))}
      </div>
    </section>
  )
}
