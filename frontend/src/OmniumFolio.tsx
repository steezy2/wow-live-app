import { iconURL, type TalentNode } from './api'
import { omniumString, togglePick } from './simOptions'
import { tip } from './tipAttrs'

/** The Omnium Folio's runes, a row of choices per node from the end of its path down to where it starts, each rune an
 * icon to click: the picked ones are lit. Two or more picked in a node are compared (every combination of the nodes
 * with several picks is simmed). picks are the entry IDs; original is the export's own picks, which the page says it
 * differs from and can go back to. */
export function OmniumFolio(props: {
  nodes: TalentNode[]
  picks: number[]
  original: number[]
  onChange: (picks: number[]) => void
}) {
  const { nodes, picks, original, onChange } = props
  const changed = omniumString(nodes, picks) !== omniumString(nodes, original)
  return (
    <section className="panel sim-omnium">
      <h2>
        Omnium Folio <span className="muted small">{picks.length} runes</span>
      </h2>
      <p className="muted small">
        Pick the runes to sim with{changed ? ', not the ones in your export' : ' (they start as your export has them)'}.
        Pick two or more of a node to compare them.
      </p>
      <div className="sim-omnium-tree">
        {[...nodes]
          .sort((a, b) => b.y - a.y)
          .map((n) => (
            <div key={n.id} className="sim-omnium-node">
              {n.entries.map((e) => (
                <button
                  key={e.id}
                  className={picks.includes(e.id) ? 'sim-rune picked' : 'sim-rune'}
                  aria-pressed={picks.includes(e.id)}
                  onClick={() => onChange(togglePick(picks, e.id))}
                  {...tip(e.name)}
                >
                  <span className="sim-icon">
                    {e.icon && <img src={iconURL(e.icon)} alt="" loading="lazy" />}
                  </span>
                  <span className="small">{e.name}</span>
                </button>
              ))}
            </div>
          ))}
      </div>
      {changed && (
        <div className="actions">
          <button onClick={() => onChange(original)}>Back to my export's runes</button>
        </div>
      )}
    </section>
  )
}
