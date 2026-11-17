import { useEffect, useState } from 'react'
import { fetchTalentTree, type CompareTalents, type PlayerCompare, type TalentTree as Tree } from './api'
import { entryName, readTree, shownEntry, type NodeRead, type NodeState } from './talents'
import { spellTip } from './tipAttrs'

/** Grid units per pixel: the tree's own grid puts nodes 300-600 apart. */
const unit = 20
const radius = 11
/** How long a talent picked from the lists below the tree pulses. */
const pulseMS = 5000

/** The mark on a node's corner for each state, so a state reads without its colour: what the legend shows too. */
const marks: Record<NodeState, string> = { both: '✓', missed: '!', extra: '+', rank: '≠', unused: '' }

/** A player's talents against the top logs': the spec's tree with each node ringed and marked by who took it, and a
 * list each for what the top logs take that the player didn't and the other way round, whose talents a click finds
 * in the tree. Needs the spec's tree and, to say anything, the player's saved talents. */
export function TalentCompare({ compare }: { compare: PlayerCompare }) {
  const talents = compare.talents
  const [tree, setTree] = useState<Tree | null>(null)
  useEffect(() => {
    if (!talents) return
    const controller = new AbortController()
    fetchTalentTree(talents.specID, controller.signal).then(setTree, () => setTree(null))
    return () => controller.abort()
  }, [talents])
  if (!talents) return null
  if (talents.you.length === 0) {
    return (
      <section className="panel">
        <h2>Talents</h2>
        <p className="muted">This run's talents weren't saved: use Read old logs on Home to fill them in.</p>
      </section>
    )
  }
  if (!tree || tree.specID !== talents.specID) return null // none yet, or the last spec's
  return <TalentPanel tree={tree} talents={talents} player={compare.player} />
}

function TalentPanel({ tree, talents, player }: { tree: Tree; talents: CompareTalents; player: string }) {
  const reads = readTree(tree, talents)
  const missed = reads.filter((r) => r.state === 'missed' || r.state === 'rank')
  const extra = reads.filter((r) => r.state === 'extra')
  const xs = reads.map((r) => r.node.x / unit)
  const ys = reads.map((r) => r.node.y / unit)
  const left = Math.min(...xs) - 2 * radius
  const top = Math.min(...ys) - 2 * radius
  const byID = new Map(reads.map((r) => [r.node.id, r]))
  const [found, setFound] = useState<number | null>(null)
  useEffect(() => {
    if (found === null) return
    const timer = setTimeout(() => setFound(null), pulseMS)
    return () => clearTimeout(timer)
  }, [found])
  const find = (id: number) => {
    setFound(null) // a talent picked again starts its pulse over
    requestAnimationFrame(() => {
      setFound(id)
      document.getElementById(nodeID(id))?.scrollIntoView({ behavior: 'smooth', block: 'center', inline: 'center' })
    })
  }
  return (
    <section className="panel">
      <h2>Talents</h2>
      <p className="small talent-legend">
        <Legend state="missed" text={`top logs take it, ${player} doesn't`} />
        <Legend state="extra" text={`only ${player}`} />
        <Legend state="rank" text="another rank" />
        <Legend state="both" text="both" />
      </p>
      <svg
        className="talent-tree"
        viewBox={`${left} ${top} ${Math.max(...xs) - left + 2 * radius} ${Math.max(...ys) - top + 2 * radius}`}
        role="img"
        aria-label={`${player}'s talents against the top ${talents.of} logs`}
      >
        <defs>
          <clipPath id="talent-icon" clipPathUnits="objectBoundingBox">
            <circle cx={0.5} cy={0.5} r={0.5} />
          </clipPath>
        </defs>
        {reads.flatMap((r) =>
          r.node.next.map((id) => {
            const to = byID.get(id)
            return to ? (
              <line
                key={`${r.node.id}-${id}`}
                className="talent-link"
                x1={r.node.x / unit}
                y1={r.node.y / unit}
                x2={to.node.x / unit}
                y2={to.node.y / unit}
              />
            ) : null
          }),
        )}
        {reads.map((r) => (
          <Node key={r.node.id} read={r} of={talents.of} found={found === r.node.id} />
        ))}
      </svg>
      <Differences title="The top logs take, you don't" reads={missed} of={talents.of} onFind={find} />
      <Differences title={`Only ${player}`} reads={extra} of={talents.of} onFind={find} />
    </section>
  )
}

function nodeID(id: number): string {
  return `talent-node-${id}`
}

/** One legend entry: the ring and mark a state has in the tree, and what it means. */
function Legend({ state, text }: { state: NodeState; text: string }) {
  return (
    <span className="talent-legend-item">
      <span className={`legend-dot talent-${state}`}>{marks[state]}</span> {text}
    </span>
  )
}

/** One node: its icon in a ring coloured by its state, a mark on its corner (and under it, how many top logs took a
 * talent you didn't), its rank, and a hover saying what the talent does and who took it. A found node pulses. */
function Node({ read, of, found }: { read: NodeRead; of: number; found: boolean }) {
  const { node, state } = read
  const cx = node.x / unit
  const cy = node.y / unit
  const entry = shownEntry(read)
  return (
    <g id={nodeID(node.id)} className={`talent-node talent-${state}`} {...spellTip(entry.spellID, nodeText(read, of))}>
      {found && <circle className="talent-pulse" cx={cx} cy={cy} r={radius} />}
      <circle className="talent-ring" cx={cx} cy={cy} r={radius} />
      {entry.icon && (
        <image href={`/api/icons/${entry.icon}`} x={cx - radius} y={cy - radius} width={2 * radius} height={2 * radius}
          clipPath="url(#talent-icon)" />
      )}
      {marks[state] && (
        <g className="talent-badge">
          <circle cx={cx + radius * 0.8} cy={cy - radius * 0.8} r={radius * 0.5} />
          <text x={cx + radius * 0.8} y={cy - radius * 0.8} dy="0.35em" textAnchor="middle">
            {marks[state]}
          </text>
        </g>
      )}
      {read.youRank > 0 && node.maxRanks > 1 && (
        <text className="talent-rank" x={cx + radius} y={cy + radius} textAnchor="end">
          {read.youRank}
        </text>
      )}
      {state === 'missed' && (
        <text className="talent-count" x={cx} y={cy + radius + 8} textAnchor="middle">
          {read.topTaken}/{of}
        </text>
      )}
    </g>
  )
}

/** What a node's hover says: your entry and rank, and the top logs' with how many took it. */
function nodeText(r: NodeRead, of: number): string {
  const ranks = (n: number) => (r.node.maxRanks > 1 ? ` (${n}/${r.node.maxRanks})` : '')
  const you = r.youEntry ? `You: ${entryName(r.node, r.youEntry)}${ranks(r.youRank)}` : 'You: not taken'
  const top = r.topEntry
    ? `Top logs: ${entryName(r.node, r.topEntry)}${ranks(r.topRank)}, ${r.topTaken} of ${of}`
    : 'Top logs: mostly not taken'
  return `${you}\n${top}`
}

/** A list of talents, each a button that finds it in the tree. */
function Differences({
  title,
  reads,
  of,
  onFind,
}: {
  title: string
  reads: NodeRead[]
  of: number
  onFind: (id: number) => void
}) {
  if (reads.length === 0) return null
  return (
    <>
      <h3>{title}</h3>
      <ul className="list talent-list">
        {reads.map((r) => (
          <li key={r.node.id}>
            <button type="button" className="talent-jump" onClick={() => onFind(r.node.id)}>
              {entryName(r.node, r.state === 'extra' ? r.youEntry : r.topEntry)}
            </button>
            {r.state !== 'extra' && (
              <span className="muted">
                {r.topTaken} of {of} top logs
              </span>
            )}
          </li>
        ))}
      </ul>
    </>
  )
}
