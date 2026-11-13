import { type ReactNode, useState } from 'react'

/** Lists under tabs, one shown at a time; initial is the one open first. All sit in one grid cell, so the tabs are as
 * tall as the tallest and what is under them never moves when another tab opens. */
export function Tabs({ tabs, initial = 0 }: { tabs: { name: string; body: ReactNode }[]; initial?: number }) {
  const [open, setOpen] = useState(initial)
  return (
    <section className="tabs">
      <div role="tablist">
        {tabs.map((t, i) => (
          <button key={t.name} role="tab" aria-selected={i === open} onClick={() => setOpen(i)}>
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
