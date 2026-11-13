import { useEffect, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { fetchSpell } from './api'

/** An element this close to the window's top gets its box below it. */
const NEAR_TOP = 90

/** Half the widest a hover box is (index.css's .tip), so a box centered this far in is inside the window. */
const TIP_HALF = 140

/** The hover box: above (x, y) in the window (below it with below), its head in small text over its content. It
 * never takes the mouse. */
export function TipBox(props: { x: number; y: number; head?: string; below?: boolean; children: ReactNode }) {
  const { x, y, head, below, children } = props
  const left = Math.min(window.innerWidth - TIP_HALF, Math.max(TIP_HALF, x)) // inside the window
  return createPortal(
    <div className={below ? 'tip below' : 'tip'} style={{ left, top: y }} role="tooltip">
      {head && <span className="muted">{head}</span>}
      {children}
    </div>,
    document.body, // not in the page's tree, where a transformed ancestor would move what is fixed to the window
  )
}

/** Shows a TipBox over whatever element under the mouse has `tip` or `spellTip` attributes, as soon as the mouse is
 * on it, so the whole app has one hover style. A spell's box says what the ability does once the backend has it, and
 * falls back to the element's own tip text. Mounted once. */
export function TipHost() {
  const [shown, setShown] = useState<{
    x: number
    y: number
    text: string
    head?: string
    below?: boolean
  } | null>(null)
  useEffect(() => {
    let current: Element | null = null // the element the box is for, so a late spell answer can't land on another
    const over = (e: MouseEvent) => {
      const el = (e.target as Element | null)?.closest?.('[data-tip],[data-spell]') ?? null
      if (el === current) return
      current = el
      if (!el) return setShown(null)
      const box = el.getBoundingClientRect()
      const x = box.left + box.width / 2
      const text = el.getAttribute('data-tip') ?? ''
      const below = box.top < NEAR_TOP // no room above
      const y = below ? box.bottom : box.top
      const own = { x, y, below, text, head: el.getAttribute('data-tip-head') ?? undefined }
      const spell = el.getAttribute('data-spell')
      setShown(spell ? { ...own, text: own.text || spell } : own)
      if (!spell) return
      void fetchSpell(spell).then((info) => {
        if (info && current === el) {
          const head = [info.name, info.cooldown].filter(Boolean).join(' · ')
          setShown({ x, y, below, head, text: info.description })
        }
      })
    }
    const hide = () => {
      current = null
      setShown(null)
    }
    // An element that goes while the mouse is on it (a list drawn again) leaves no mouse event to end its box.
    const gone = new MutationObserver(() => current && !current.isConnected && hide())
    gone.observe(document.body, { childList: true, subtree: true })
    document.addEventListener('mouseover', over)
    document.addEventListener('mousedown', hide)
    document.addEventListener('scroll', hide, true)
    return () => {
      gone.disconnect()
      document.removeEventListener('mouseover', over)
      document.removeEventListener('mousedown', hide)
      document.removeEventListener('scroll', hide, true)
    }
  }, [])
  return (
    shown && (
      <TipBox x={shown.x} y={shown.y} head={shown.head} below={shown.below}>
        {shown.text}
      </TipBox>
    )
  )
}
