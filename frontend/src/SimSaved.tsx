import { simClassColor, simClassName } from './classColors'
import { amount } from './format'
import { type Sim } from './api'
import { kindLabel } from './simOptions'

/** The saved sims as full-width bars, tinted with the character's class colour: who it was ("Frost Mage"), what kind of
 * sim, its DPS and when it ran. A click opens the report; the cross deletes it. */
export function SavedSims(props: { sims: Sim[]; onOpen: (id: string) => void; onDelete: (id: string) => void }) {
  return (
    <ul className="sim-saved-list">
      {props.sims.map((s) => {
        const who = [s.spec && s.spec[0].toUpperCase() + s.spec.slice(1), simClassName(s.class)].filter(Boolean)
        return (
          <li key={s.id} style={{ '--class': simClassColor(s.class) ?? 'var(--muted)' } as React.CSSProperties}>
            <button className="sim-saved-open" onClick={() => props.onOpen(s.id)}>
              <span className="sim-saved-who">
                <strong>{s.character}</strong>
                <span className="muted">{who.join(' ')}</span>
              </span>
              <span className="sim-saved-kind">{kindLabel(s.kind)}</span>
              <span className="sim-saved-dps">{s.dps ? `${amount(s.dps)} DPS` : ''}</span>
              <span className="muted small">{new Date(s.startedMS).toLocaleString()}</span>
            </button>
            <button className="delete" aria-label="Delete" onClick={() => props.onDelete(s.id)}>
              ×
            </button>
          </li>
        )
      })}
    </ul>
  )
}
