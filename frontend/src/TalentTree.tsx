import { useEffect, useState } from 'react'
import { fetchTalentTree, type CompareTalents, type PlayerCompare, type TalentTree as Tree } from './api'
import { entryName, readTree, type NodeRead } from './talents'
import { tip } from './tipAttrs'

/** Grid units per pixel: the tree's own grid puts nodes 300-600 apart. */
const unit = 20
const radius = 11

/** A player's talents against the top logs': the spec's tree with each node coloured by who took it, and a list each
 * for what the top logs take that the player didn't and the other way round. Needs the spec's tree and, to say
 * anything, the player's saved talents. */
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
  if (!tree) return null
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
  return (
    <section className="panel">
      <h2>Talents</h2>
      <p className="small">
        <span className="legend-dot talent-missed" /> top logs take it, {player} doesn't · <span className="legend-dot talent-extra" />{' '}
        only {player} · <span className="legend-dot talent-both" /> both
      </p>
      <svg
        className="talent-tree"
        viewBox={`${left} ${top} ${Math.max(...xs) - left + 2 * radius} ${Math.max(...ys) - top + 2 * radius}`}
        role="img"
        aria-label={`${player}'s talents against the top ${talents.of} logs`}
      >
        {reads.flatMap((r) =>
          r.node.next.map((id) => {
            const to = byID.get(id)
            return to ? (
              <line key={`${r.node.id}-${id}`} className="talent-link" x1={r.node.x / unit} y1={r.node.y / unit} x2={to.node.x / unit}
                y2={to.node.y / unit} />
            ) : null
          }),
        )}
        {reads.map((r) => (
          <Node key={r.node.id} read={r} of={talents.of} />
        ))}
      </svg>
      <Differences title="The top logs take, you don't" reads={missed} of={talents.of} />
      <Differences title={`Only ${player}`} reads={extra} of={talents.of} />
    </section>
  )
}

/** One node: a circle coloured by its state, its rank in it, and a hover saying who took what. */
function Node({ read, of }: { read: NodeRead; of: number }) {
  const { node, state } = read
  return (
    <g className={`talent-node talent-${state}`} {...tip(nodeText(read, of), node.name)}>
      <circle cx={node.x / unit} cy={node.y / unit} r={radius} />
      {read.youRank > 0 && (
        <text x={node.x / unit} y={node.y / unit} dy="0.35em" textAnchor="middle">
          {node.maxRanks > 1 ? read.youRank : ''}
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

function Differences({ title, reads, of }: { title: string; reads: NodeRead[]; of: number }) {
  if (reads.length === 0) return null
  return (
    <>
      <h3>{title}</h3>
      <ul className="list">
        {reads.map((r) => (
          <li key={r.node.id}>
            <span>{entryName(r.node, r.state === 'extra' ? r.youEntry : r.topEntry)}</span>
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
