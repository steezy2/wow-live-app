import type { SimGem, SimItemInfo, SimProfile, SimReference, SimVariant } from './api'
import { ItemLine, Sockets } from './SimItem'
import { gearGroups } from './simOptions'
import { ItemVariants } from './SimVariants'

/** The gear of a read character laid out like the paper doll: a column down each side and the weapons along the
 * bottom, each slot a card with what is worn and, for Top Gear (pick set), the bag items that fit, the highest item
 * level first, each with a checkbox and a form for variants of it. equipped and bags are the picked indexes into the
 * profile's lists. */
export function SimGear(props: {
  profile: SimProfile
  pick?: {
    equipped: number[]
    bags: number[]
    onToggle: (worn: boolean, index: number) => void
    reference: SimReference
    variants: SimVariant[]
    notes: string[] // what each variant changes, in the app's words (the plan's)
    onAddVariant: (v: SimVariant) => void
    onRemoveVariant: (v: SimVariant) => void
  }
  order: string[]
  info: Map<number, SimItemInfo>
  gems: SimGem[] // to draw the gems in the sockets
}) {
  const { profile, order, pick, info, gems } = props
  const groups = gearGroups(profile.equipped, pick ? profile.bags : [], order)
  return (
    <div className="sim-doll">
      {[0, 1, 2].map((column) => (
        <div key={column} className="sim-doll-col">
          {groups
            .filter((g) => g.column === column)
            .map((g) => (
              <div key={g.key} className="sim-slot">
                <h3>{g.label}</h3>
                {g.items.map(({ item, index, worn }) => (
                  <div key={`${worn}-${index}`} className="sim-entry">
                    <label className={pick ? 'sim-pick' : 'sim-pick fixed'}>
                      {pick && (
                        <input
                          type="checkbox"
                          checked={(worn ? pick.equipped : pick.bags).includes(index)}
                          onChange={() => pick.onToggle(worn, index)}
                        />
                      )}
                      <ItemLine
                        item={item}
                        info={info.get(item.id)}
                        extra={
                          <>
                            {worn && <span className="sim-worn">worn</span>}
                            <Sockets item={item} gems={gems} />
                          </>
                        }
                      />
                    </label>
                    {pick && (
                      <ItemVariants
                        item={item}
                        worn={worn}
                        index={index}
                        reference={pick.reference}
                        variants={pick.variants}
                        notes={pick.notes}
                        onAdd={pick.onAddVariant}
                        onRemove={pick.onRemoveVariant}
                      />
                    )}
                  </div>
                ))}
              </div>
            ))}
        </div>
      ))}
    </div>
  )
}

/** The character's name, spec, realm and average item level above the gear, with the talent string to copy. */
export function SimHeader({ profile }: { profile: SimProfile }) {
  return (
    <header className="sim-header">
      <div>
        <h2>{profile.name}</h2>
        <p className="muted">
          {profile.spec} {profile.class} · {profile.race} · {profile.server}-{profile.region.toUpperCase()}
        </p>
      </div>
      <div className="sim-header-stats">
        {profile.itemLevel > 0 && (
          <span>
            <strong>{profile.itemLevel}</strong>
            <span className="muted small"> item level</span>
          </span>
        )}
        <button onClick={() => void navigator.clipboard.writeText(profile.talents)}>Copy talents</button>
      </div>
    </header>
  )
}
