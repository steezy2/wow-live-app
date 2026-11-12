import { type ReactNode, useState } from 'react'

/** Lists under tabs, one shown at a time; initial is the one open first. */
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
      <div role="tabpanel">{tabs[open].body}</div>
    </section>
  )
}
