import { useEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { fetchSpell, type DungeonMob, type MobSpell, type Patrol, type RouteEnemy } from './api'
import { amount } from './format'
import { dismissOn, mobName, PULL_COLORS, type Edit } from './routes'
import { Spell } from './Spell'

/** How wide a menu is at most (index.css's .context-menu) and the least room it keeps under it, so it stays inside
 * the window. */
const MENU_WIDTH = 300
const MENU_ROOM = 240

/** What Keystone.guru's flags mean in a fight. */
const FLAG_TEXT: Record<string, string> = {
  bolstering: 'Bolsters',
  bursting: 'Bursts',
  sanguine: 'Sanguine',
  dangerous: 'Dangerous',
  truesight: 'Sees stealth',
  runsAwayInFear: 'Runs away in fear',
}

const NO_SPELLS: MobSpell[] = []

/** A box at (x, y) in the window over the page, focused when it opens; Escape or a press outside it closes it. */
function ContextMenu(props: { x: number; y: number; label: string; onClose: () => void; children: ReactNode }) {
  const { x, y, label, onClose, children } = props
  const ref = useRef<HTMLDivElement>(null)
  useEffect(() => ref.current?.focus(), [])
  useEffect(() => dismissOn(document, (t) => !!ref.current?.contains(t as Node), onClose), [onClose])
  const { clientWidth: innerWidth, clientHeight: innerHeight } = document.documentElement // without the scrollbar
  const top = Math.max(0, Math.min(y, innerHeight - MENU_ROOM))
  return createPortal(
    <div
      ref={ref}
      className="context-menu"
      role="dialog"
      aria-label={label}
      tabIndex={-1}
      style={{ left: Math.max(0, Math.min(x, innerWidth - MENU_WIDTH)), top, maxHeight: innerHeight - top }}
    >
      {children}
    </div>,
    document.body, // not in the page's tree, where a transformed ancestor would move what is fixed to the window
  )
}

/** A mob's card at a key level: what it is, its health there and the level's damage multiplier (no source gives a
 * mob's own damage), its forces, the patrol it walks, Keystone.guru's flags and its spells (each hoverable, marked
 * Interruptible and Magic), with what the route editor can do with it. Without the mob's data (an old reference
 * file), its name and forces only. */
export function MobMenu(props: {
  enemy: RouteEnemy
  mob?: DungeonMob
  level?: number
  damageMult?: number
  patrol?: Patrol
  actions: { label: string; edit: Edit }[]
  x: number
  y: number
  onEdit: (e: Edit) => void
  onClose: () => void
}) {
  const { enemy, mob, level, damageMult, patrol, actions, x, y, onEdit, onClose } = props
  const spells = mob?.spells ?? NO_SPELLS
  const [names, setNames] = useState(new Map<number, string>())
  useEffect(() => {
    let on = true
    void Promise.all(spells.map((s) => fetchSpell(s.id))).then(
      (infos) => on && setNames(new Map(spells.map((s, i) => [s.id, infos[i]?.name ?? '']))),
    )
    return () => void (on = false)
  }, [spells])
  const name = mobName(enemy.npc, mob)
  const what = [mob?.type, mob?.class, mob?.level && `level ${mob.level}`].filter(Boolean).join(' · ')
  return (
    <ContextMenu x={x} y={y} label={name} onClose={onClose}>
      <h3>{name}</h3>
      {what && <p className="muted small">{what}</p>}
      <dl className="mob-facts">
        {mob && mob.health > 0 && (
          <>
            <dt>Health at +{level}</dt>
            <dd>{amount(mob.health)}</dd>
          </>
        )}
        {mob && damageMult !== undefined && (
          <>
            <dt>Damage at +{level}</dt>
            <dd>×{damageMult.toFixed(2)}</dd>
          </>
        )}
        <dt>Forces</dt>
        <dd>{enemy.count}</dd>
        {patrol && (
          <>
            <dt>Patrols</dt>
            <dd>{patrol.loop ? 'Round a loop' : 'Back and forth'}</dd>
          </>
        )}
      </dl>
      {mob && damageMult !== undefined && (
        <p className="muted small">No source gives a mob's own damage: this is the key level's multiplier over +1.</p>
      )}
      {(mob?.flags?.length ?? 0) > 0 && (
        <p className="mob-badges">
          {mob?.flags?.map((f) => (
            <span key={f} className="tag">
              {FLAG_TEXT[f] ?? f}
            </span>
          ))}
        </p>
      )}
      {spells.length > 0 && (
        <ul className="mob-spells">
          {spells.map((s) => (
            <li key={s.id}>
              <Spell name={names.get(s.id) || `Spell ${s.id}`} id={s.id} />
              {s.interruptible && <span className="tag good">Interruptible</span>}
              {s.magic && <span className="tag">Magic</span>}
            </li>
          ))}
        </ul>
      )}
      <MenuActions actions={actions} onEdit={onEdit} onClose={onClose} />
    </ContextMenu>
  )
}

/** A pull row's menu: move the pull up or down, delete it, or give it another of Mythic Dungeon Tools' colors. The
 * pull is the editor's selected one, so the edits are the toolbar's. */
export function PullMenu(props: {
  pull: number
  pulls: number
  color: string
  x: number
  y: number
  onEdit: (e: Edit) => void
  onClose: () => void
}) {
  const { pull, pulls, color, x, y, onEdit, onClose } = props
  const actions: { label: string; edit: Edit }[] = [
    ...(pull > 0 ? [{ label: 'Move up', edit: { kind: 'move', by: -1 } as const }] : []),
    ...(pull < pulls - 1 ? [{ label: 'Move down', edit: { kind: 'move', by: 1 } as const }] : []),
    { label: 'Delete pull', edit: { kind: 'delete' } },
  ]
  return (
    <ContextMenu x={x} y={y} label={`Pull ${pull + 1}`} onClose={onClose}>
      <h3>Pull {pull + 1}</h3>
      <div className="pull-colors">
        {PULL_COLORS.map((c) => (
          <button
            key={c}
            className="route-swatch"
            style={{ background: `#${c}` }}
            aria-pressed={c === color}
            aria-label={`Color ${c}`}
            onClick={() => (onEdit({ kind: 'recolor', color: c }), onClose())}
          />
        ))}
      </div>
      <MenuActions actions={actions} onEdit={onEdit} onClose={onClose} />
    </ContextMenu>
  )
}

/** A menu's buttons, each making its edit and closing the menu. */
function MenuActions(props: {
  actions: { label: string; edit: Edit }[]
  onEdit: (e: Edit) => void
  onClose: () => void
}) {
  const { actions, onEdit, onClose } = props
  return (
    <div className="menu-actions">
      {actions.map((a) => (
        <button key={a.label} onClick={() => (onEdit(a.edit), onClose())}>
          {a.label}
        </button>
      ))}
    </div>
  )
}
