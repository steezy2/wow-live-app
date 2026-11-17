import type { SimItemInfo, SimProfile } from './api'
import { ItemLine } from './SimItem'
import { DOLL_COLUMNS, gearGroups } from './simOptions'

/** The gear of a read character laid out like the paper doll: a column down each side and the weapons along the
 * bottom, each slot a card with what is worn and, for Top Gear (pick set), the bag items that fit, each with a
 * checkbox. equipped and bags are the picked indexes into the profile's lists. */
export function SimGear(props: {
  profile: SimProfile
  pick?: { equipped: number[]; bags: number[]; onToggle: (worn: boolean, index: number) => void }
  info: Map<number, SimItemInfo>
}) {
  const { profile, pick, info } = props
  const groups = gearGroups(profile.equipped, profile.bags)
  return (
    <div className="sim-doll">
      {DOLL_COLUMNS.map((keys) => (
        <div key={keys[0]} className="sim-doll-col">
          {groups
            .filter((g) => keys.includes(g.key))
            .map((g) => (
              <div key={g.key} className="sim-slot">
                <h3>{g.label}</h3>
                {g.items
                  .filter(({ worn }) => worn || pick)
                  .map(({ item, index, worn }) => (
                    <label key={`${worn}-${index}`} className={pick ? 'sim-pick' : 'sim-pick fixed'}>
                      {pick && (
                        <input
                          type="checkbox"
                          checked={(worn ? pick.equipped : pick.bags).includes(index)}
                          onChange={() => pick.onToggle(worn, index)}
                        />
                      )}
                      <ItemLine item={item} info={info.get(item.id)} extra={worn && <span className="sim-worn">worn</span>} />
                    </label>
                  ))}
              </div>
            ))}
        </div>
      ))}
    </div>
  )
}

/** The character's name, spec, realm and average item level above the gear, with the talent string to copy. */
export function SimHeader({ profile, level }: { profile: SimProfile; level: number }) {
  return (
    <header className="sim-header">
      <div>
        <h2>{profile.name}</h2>
        <p className="muted">
          {profile.spec} {profile.class} · {profile.race} · {profile.server}-{profile.region.toUpperCase()}
        </p>
      </div>
      <div className="sim-header-stats">
        {level > 0 && (
          <span>
            <strong>{level}</strong>
            <span className="muted small"> item level</span>
          </span>
        )}
        <button onClick={() => void navigator.clipboard.writeText(profile.talents)}>Copy talents</button>
      </div>
    </header>
  )
}
