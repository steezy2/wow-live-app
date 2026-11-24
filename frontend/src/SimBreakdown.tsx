import { iconURL, type SimAbility, type SimBuff, type SimLogEntry, type SpellInfo } from './api'
import { simSchoolBar } from './schoolColors'
import { tip } from './tipAttrs'

/** What the lookup knows of each spell by ID. */
type Info = Map<number, SpellInfo>

/** A spell's icon (empty until its lookup answers) and name, with what it does on hover when `hover` is set. */
function SimSpell({ name, id, info, hover = true }: { name: string; id: number; info: Info; hover?: boolean }) {
  const spell = info.get(id)
  const head = [name, spell?.cooldown].filter(Boolean).join(' · ')
  return (
    <span className="sim-spell" {...(hover ? tip(spell?.description, head) : {})}>
      <span className="sim-spell-icon">
        {spell?.icon && <img src={iconURL(spell.icon)} alt="" loading="lazy" />}
      </span>
      <span className="sim-spell-name">{name}</span>
    </span>
  )
}

/** A DPS number in full ("16,732,287"). */
const full = (n: number) => Math.round(n).toLocaleString('en-US')

/** A fight time as the game's log writes it ("0:02:897"). */
function stamp(seconds: number): string {
  const ms = Math.round(seconds * 1000)
  const two = String(Math.floor(ms / 1000) % 60).padStart(2, '0')
  return `${Math.floor(ms / 60000)}:${two}:${String(ms % 1000).padStart(3, '0')}`
}

/** One row of a breakdown: the name in its school's colour, a bar of the share in a fill from that colour, then the
 * numbers. */
function BarRow(props: {
  name: string
  id: number
  info: Info
  school?: string
  share: number
  label: string
  numbers: string[]
}) {
  const { color, fill } = simSchoolBar(props.school, 'var(--accent)')
  return (
    <li className="sim-bar-row" style={{ color }}>
      <SimSpell name={props.name} id={props.id} info={props.info} />
      <span className="sim-bar-pct">{props.label}</span>
      <span className="sim-bar">
        <span style={{ width: `${Math.max(0.5, props.share)}%`, background: fill }} />
      </span>
      {props.numbers.map((n, i) => (
        <span key={i} className="sim-bar-num">
          {n}
        </span>
      ))}
    </li>
  )
}

/** The damage breakdown: each ability's share as a bar in its school's colours, with its DPS and casts. */
export function AbilityBars({ abilities, info }: { abilities: SimAbility[]; info: Info }) {
  const top = Math.max(0.0001, ...abilities.map((a) => a.share))
  return (
    <section className="panel sim-breakdown">
      <h2>Damage breakdown</h2>
      <ul>
        <li className="sim-bar-row sim-bar-head muted small">
          <span>Ability</span>
          <span className="sim-bar-pct">Share</span>
          <span />
          <span className="sim-bar-num">DPS</span>
          <span className="sim-bar-num">Casts</span>
        </li>
        {abilities.map((a) => (
          <BarRow
            key={a.id || a.name}
            name={a.name}
            id={a.id}
            info={info}
            school={a.school}
            share={(a.share / top) * 100}
            label={`${(a.share * 100).toFixed(1)}%`}
            numbers={[full(a.dps), a.casts.toFixed(1)]}
          />
        ))}
      </ul>
    </section>
  )
}

/** The buffs' uptime as bars in their school's colours, with how many times each started. */
export function BuffBars({ buffs, info }: { buffs: SimBuff[]; info: Info }) {
  return (
    <section className="panel sim-breakdown">
      <h2>Buff uptime</h2>
      <ul>
        <li className="sim-bar-row sim-bar-head muted small">
          <span>Buff</span>
          <span className="sim-bar-pct">Uptime</span>
          <span />
          <span className="sim-bar-num">Count</span>
        </li>
        {buffs.map((b) => (
          <BarRow
            key={b.id ? `${b.id}-${b.name}` : b.name}
            name={b.name}
            id={b.id}
            info={info}
            school={b.school}
            share={b.uptime}
            label={`${b.uptime.toFixed(1)}%`}
            numbers={[b.count.toFixed(0)]}
          />
        ))}
      </ul>
    </section>
  )
}

/** The sample ability log: one iteration's actions in order with the buffs that were up, before the pull first. */
export function AbilityLog({ log, info }: { log: SimLogEntry[]; info: Info }) {
  const pulled = log.findIndex((e) => !e.pre)
  return (
    <section className="panel sim-log">
      <h2>Sample ability log</h2>
      <p className="sim-log-warning">
        This is a sample of 1 iteration, it is not a guide on how to play your character. Analyze with care.
      </p>
      <p className="muted small">
        It shows one iteration&rsquo;s actions and will vary between sims with the procs and buffs that happen to fall.
      </p>
      <div className="sim-log-rows">
        {log.map((e, i) => (
          <div key={i}>
            {(i === 0 || i === pulled) && <h3>{e.pre ? 'Pre-pull' : 'Pull'}</h3>}
            <div className="sim-log-row">
              <span className="muted small">{stamp(e.time)}</span>
              <SimSpell name={e.name} id={e.id} info={info} />
              <span className="muted small">{e.target}</span>
              <span className="sim-log-buffs">
                {e.buffs?.map((b) => (
                  <span key={`${b.id}-${b.name}`} className="sim-log-buff">
                    <SimSpell name={b.name} id={b.id} info={info} hover={false} />
                    {b.stacks > 1 && <sup>{b.stacks}</sup>}
                  </span>
                ))}
              </span>
            </div>
          </div>
        ))}
      </div>
    </section>
  )
}
