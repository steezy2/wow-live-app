import { describe, expect, it } from 'vitest'
import type { RouteEnemy, RouteView } from './api'
import { edit, editorOf, packOf, pullCentres, UNDO_EDITS, type Edit, type Editor } from './routes'

const enemy = (index: number, x: number, y: number, pack = 0): RouteEnemy => ({
  index,
  clone: 1,
  npc: 1,
  count: 1,
  pack,
  x,
  y,
  map: 1,
})

describe('pullCentres', () => {
  it('puts a pull in the middle of its enemies and leaves out pulls with none on the floor', () => {
    const enemies = [enemy(1, 0, 0), enemy(2, 10, 20), enemy(3, 99, 99)]
    const pulls = [
      { color: 'ff0000', enemies: [{ index: 1, clone: 1 }, { index: 2, clone: 1 }] },
      { color: '00ff00', enemies: [{ index: 4, clone: 1 }] },
    ]
    expect(pullCentres(enemies, pulls)).toEqual([{ pull: 0, x: 5, y: 10 }])
  })
})

/** An editor on a route whose pulls hold these enemy indexes (clone 1), with pull `selected` selected. */
function editor(pulls: number[][], selected: number): Editor {
  const route = {
    name: 'r',
    pulls: pulls.map((p) => ({ color: 'ff0000', enemies: p.map((index) => ({ index, clone: 1 })) })),
  } as RouteView
  return { ...editorOf(route), selected }
}
const indexes = (s: Editor) => s.draft.pulls.map((p) => p.enemies.map((e) => e.index))
const toggle = (...index: number[]): Edit => ({ kind: 'toggle', enemies: index.map((i) => enemy(i, 0, 0)) })

describe('edit', () => {
  const pack = [enemy(1, 0, 0, 7), enemy(2, 0, 0, 7), enemy(3, 0, 0)]
  it.each([
    { name: 'toggle adds an enemy to its pull', from: [[1], []], selected: 1, by: toggle(2), want: [[1], [2]] },
    { name: 'toggle takes it out of another pull', from: [[1, 2], []], selected: 1, by: toggle(2), want: [[1], [2]] },
    { name: 'toggle takes it out of its own pull', from: [[1, 2]], selected: 0, by: toggle(1), want: [[2]] },
    { name: 'toggle with no pull makes one', from: [], selected: -1, by: toggle(3), want: [[3]] },
    {
      name: 'pack takes the whole pack',
      from: [[1], []],
      selected: 1,
      by: { kind: 'toggle', enemies: packOf(pack[0], pack) } as Edit,
      want: [[], [1, 2]],
    },
    {
      name: 'pack takes out a pack the pull has whole',
      from: [[1, 2, 3]],
      selected: 0,
      by: { kind: 'toggle', enemies: packOf(pack[1], pack) } as Edit,
      want: [[3]],
    },
    {
      name: 'pack 0 is the enemy alone',
      from: [[]],
      selected: 0,
      by: { kind: 'toggle', enemies: packOf(pack[2], pack) } as Edit,
      want: [[3]],
    },
  ])('$name', ({ from, selected, by, want }) => {
    expect(indexes(edit(editor(from, selected), by))).toEqual(want)
  })

  it('undoes edits in reverse, at most UNDO_EDITS of them, and a name typed is one edit', () => {
    let s = editor([[]], 0)
    for (let i = 1; i <= UNDO_EDITS + 2; i++) s = edit(s, toggle(i))
    s = edit(edit(s, { kind: 'rename', name: 'a' }), { kind: 'rename', name: 'ab' })
    s = edit(s, { kind: 'undo' })
    expect(s.draft.name).toBe('r')
    s = edit(s, { kind: 'undo' })
    expect(indexes(s)).toEqual([[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21]])
    for (let i = 0; i < UNDO_EDITS; i++) s = edit(s, { kind: 'undo' })
    expect(indexes(s)).toEqual([[1, 2, 3]])
    expect(edit(s, { kind: 'undo' })).toBe(s)
  })
})
