import { describe, expect, it } from 'vitest'
import type { CompareTalents, TalentNode, TalentTree } from './api'
import { readTree } from './talents'

const node = (id: number, entries: number[], tree: TalentNode['tree'] = 'spec', subTree?: number): TalentNode => ({
  id,
  name: `Node ${id}`,
  tree,
  subTree,
  x: id * 300,
  y: 1500,
  maxRanks: 1,
  next: [],
  entries: entries.map((e) => ({ id: e, spellID: e, name: `Entry ${e}`, icon: '' })),
})
const tree: TalentTree = {
  specID: 62,
  class: 'Mage',
  spec: 'Arcane',
  nodes: [
    node(1, [10]),
    node(2, [20]),
    node(3, [30]),
    node(4, [40, 41]),
    node(5, [50]),
    node(6, [60], 'hero', 7),
    node(7, [70], 'hero', 8),
  ],
}

describe('readTree', () => {
  it('says who took each talent', () => {
    const talents: CompareTalents = {
      specID: 62,
      of: 10,
      you: [{ n: 1, e: 10, r: 1 }, { n: 3, e: 30, r: 1 }, { n: 4, e: 41, r: 1 }, { n: 6, e: 60, r: 1 }],
      top: [
        { n: 1, e: 10, r: 1, taken: 9, most: true },
        { n: 2, e: 20, r: 1, taken: 8, most: true },
        { n: 3, e: 30, r: 1, taken: 2, most: false },
        { n: 4, e: 40, r: 1, taken: 7, most: true },
        { n: 5, e: 50, r: 1, taken: 4, most: false },
      ],
    }
    const state = Object.fromEntries(readTree(tree, talents).map((r) => [r.node.id, r.state]))
    expect(state).toEqual({ 1: 'both', 2: 'missed', 3: 'extra', 4: 'missed', 5: 'unused', 6: 'extra', 7: 'unused' })
  })

  it('reads both hero trees', () => {
    const talents: CompareTalents = {
      specID: 62,
      of: 4,
      you: [{ n: 1, e: 10, r: 1 }],
      top: [{ n: 7, e: 70, r: 1, taken: 4, most: true }],
    }
    const state = Object.fromEntries(readTree(tree, talents).map((r) => [r.node.id, r.state]))
    expect(state).toMatchObject({ 6: 'unused', 7: 'missed' })
  })
})
