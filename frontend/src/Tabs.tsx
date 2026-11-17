import { type KeyboardEvent, type ReactNode, useId, useState } from 'react'

/** A tab: its id (what the page and the address name it by), its name (it can change, it holds a count) and body. */
export interface TabItem {
  id: string
  name: string
  body: ReactNode
}

/** Lists under tabs, one shown at a time; initial is the id of the one open first, or open, when the page decides
 * which. All sit in one grid cell, so the tabs are as tall as the tallest and what is under them never moves when
 * another tab opens. onOpen hears of the id picked. The arrow keys, Home and End move between the tabs. */
export function Tabs(props: {
  tabs: TabItem[]
  initial?: string
  open?: string
  onOpen?: (id: string) => void
}) {
  const { tabs, initial, onOpen } = props
  const base = useId()
  const [picked, setPicked] = useState(initial)
  const open = props.open ?? picked ?? tabs[0]?.id
  const pick = (id: string) => {
    setPicked(id)
    onOpen?.(id)
    document.getElementById(`${base}-tab-${id}`)?.focus()
  }
  const move = (e: KeyboardEvent) => {
    const at = tabs.findIndex((t) => t.id === open)
    const to = { ArrowRight: at + 1, ArrowLeft: at - 1, Home: 0, End: tabs.length - 1 }[e.key]
    if (to === undefined) return
    e.preventDefault()
    pick(tabs[(to + tabs.length) % tabs.length].id)
  }
  return (
    <section className="tabs">
      <div role="tablist" onKeyDown={move}>
        {tabs.map((t) => (
          <button
            key={t.id}
            id={`${base}-tab-${t.id}`}
            role="tab"
            aria-selected={t.id === open}
            aria-controls={`${base}-panel-${t.id}`}
            tabIndex={t.id === open ? 0 : -1}
            onClick={() => pick(t.id)}
          >
            {t.name}
          </button>
        ))}
      </div>
      <div className="tab-panels">
        {tabs.map((t) => (
          <div
            key={t.id}
            id={`${base}-panel-${t.id}`}
            role="tabpanel"
            aria-labelledby={`${base}-tab-${t.id}`}
            className={t.id === open ? undefined : 'tab-off'}
            inert={t.id !== open}
          >
            {t.body}
          </div>
        ))}
      </div>
    </section>
  )
}
