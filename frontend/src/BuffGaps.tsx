import { Fragment } from 'react'
import { type BuffGap } from './api'
import { groupBy } from './format'
import { When, Who, type Seeker } from './Seek'
import { Spell } from './Spell'

/** What a key or raid pull lacked, a line per buff (the most players first), for a tab: who lacked it the whole fight
 * ("everyone" when all did), then each shorter gap as a player and its start (a time that moves the replay with seek) to
 * its end. endMS is the fight's length and players how many were in it. */
export function BuffGaps(props: { gaps: BuffGap[]; endMS: number; players: number; seek?: Seeker }) {
  const { gaps, endMS, players, seek } = props
  const buffs = [...groupBy(gaps, (g) => g.buff)].sort(([, a], [, b]) => b.length - a.length)
  const whole = (g: BuffGap) => g.fromMS === 0 && g.toMS >= endMS
  return (
    <ul className="list scroll buff-gaps small">
      {buffs.map(([buff, list]) => {
        const all = list.filter(whole)
        const parts = list.filter((g) => !whole(g))
        return (
          <li key={buff}>
            <span className="muted">Missing</span> <b>
              <Spell name={buff} />
            </b>{' '}
            {all.length > 0 && (
              <span>
                {all.length === players
                  ? 'everyone'
                  : all.map((g, i) => (
                      <Fragment key={g.player}>
                        {i > 0 && ', '}
                        <Who player={g.player} seek={seek} />
                      </Fragment>
                    ))}{' '}
                <span className="muted">all fight</span>
              </span>
            )}
            {parts.map((g, i) => (
              <span key={`${g.player}-${g.fromMS}`}>
                {(all.length > 0 || i > 0) && ' · '}
                <Who player={g.player} seek={seek} /> <When ms={g.fromMS} seek={seek} />
                <span className="muted">–{g.toMS < endMS ? <When ms={g.toMS} seek={seek} /> : 'end'}</span>
              </span>
            ))}
          </li>
        )
      })}
    </ul>
  )
}
