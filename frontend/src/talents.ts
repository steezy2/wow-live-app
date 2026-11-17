import type { CompareTalents, TalentEntry, TalentNode, TalentTree } from './api'

/** How a node of the tree reads against the top logs. */
export type NodeState = 'both' | 'missed' | 'extra' | 'rank' | 'unused'

/** A node's state and the pick(s) behind it. `missing` is the entry the top logs mostly take that you didn't. */
export interface NodeRead {
  node: TalentNode
  state: NodeState
  /** The entry you took, if any, and its rank. */
  youEntry?: number
  youRank: number
  /** The entry most top logs took here, how many of them did, and its rank. */
  topEntry?: number
  topTaken: number
  topRank: number
}

/** Reads each node of the drawn trees against your picks and the top logs': taken by both, missed (the top logs
 * mostly took it, you didn't), extra (you took it, they mostly didn't), at another rank, or unused by both. */
export function readTree(tree: TalentTree, talents: CompareTalents): NodeRead[] {
  return tree.nodes.map((node) => {
    const entryIDs = new Set(node.entries.map((e) => e.id))
    const you = talents.you.find((p) => p.n === node.id)
    const top = talents.top
      .filter((s) => s.n === node.id && entryIDs.has(s.e) && s.most)
      .sort((a, b) => b.taken - a.taken)[0]
    const read: NodeRead = { node, state: 'unused', youEntry: you?.e, youRank: you?.r ?? 0, topEntry: top?.e,
      topTaken: top?.taken ?? 0, topRank: top?.r ?? 0 }
    if (you && top) read.state = you.e === top.e ? (you.r === top.r ? 'both' : 'rank') : 'missed'
    else if (top) read.state = 'missed'
    else if (you) read.state = 'extra'
    return read
  })
}

/** The entry a node is drawn and explained by: the one you took, else the one the top logs did, else its first. */
export function shownEntry(read: NodeRead): TalentEntry {
  const { node } = read
  return node.entries.find((e) => e.id === (read.youEntry ?? read.topEntry)) ?? node.entries[0]
}

/** The name of the entry an id is in a node. */
export function entryName(node: TalentNode, entry?: number): string {
  return node.entries.find((e) => e.id === entry)?.name ?? node.name
}
