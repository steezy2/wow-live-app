import { type ReactNode, useState } from 'react'

/** Lists under tabs, one shown at a time. */
export function Tabs({ tabs }: { tabs: { name: string; body: ReactNode }[] }) {
  const [open, setOpen] = useState(0)
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
