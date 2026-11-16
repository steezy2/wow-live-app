import { describe, expect, it, vi } from 'vitest'
import type { Drawing, DungeonMob, Patrol, RouteEnemy, RouteView } from './api'
import {
  dismissOn,
  drawingAt,
  edit,
  editorOf,
  enemyKey,
  mobActions,
  packOf,
  patrolOf,
  pullCentres,
  pullHull,
  type Point,
  pullMobs,
  simplifyLine,
  UNDO_EDITS,
  type Edit,
  type Editor,
} from './routes'

const enemy = (index: number, x: number, y: number, pack = 0, npc = 1): RouteEnemy => ({
  index,
  clone: 1,
  npc,
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

describe('patrolOf', () => {
  it('finds the patrol by enemy and clone index, not by enemy index alone', () => {
    const walk: Patrol = { map: 1, loop: true, color: '003280', points: [], enemies: [{ index: 6, clone: 2 }] }
    expect(patrolOf({ index: 6, clone: 2 }, [walk])).toBe(walk)
    expect(patrolOf({ index: 6, clone: 1 }, [walk])).toBeUndefined()
  })
})

describe('pullHull', () => {
  const box = (h: Point[]) => ({
    x0: Math.min(...h.map((p) => p[0])),
    x1: Math.max(...h.map((p) => p[0])),
    y0: Math.min(...h.map((p) => p[1])),
    y1: Math.max(...h.map((p) => p[1])),
  })
  const square: Point[] = [[0, 0], [10, 0], [0, 10], [10, 10]]
  const inner: Point[] = Array.from({ length: 8 }, (_, i) => [3 + (i % 4), 3 + Math.floor(i / 4) * 3])
  it.each<[string, Point[], ReturnType<typeof box> | undefined]>([
    ['no points', [], undefined],
    ['one point is a circle', [[10, 10]], { x0: 8, x1: 12, y0: 8, y1: 12 }],
    ['two points are a capsule', [[0, 0], [10, 0]], { x0: -2, x1: 12, y0: -2, y1: 2 }],
    ['three points', [[0, 0], [10, 0], [0, 10]], { x0: -2, x1: 12, y0: -2, y1: 12 }],
    ['collinear points', [[0, 0], [5, 0], [10, 0]], { x0: -2, x1: 12, y0: -2, y1: 2 }],
    ['twelve points, the inner ones inside the outline', [...square, ...inner], { x0: -2, x1: 12, y0: -2, y1: 12 }],
  ])('%s', (_, points, want) => {
    const hull = pullHull(points, 2)
    if (!want) return expect(hull).toEqual([])
    const got = box(hull)
    expect(got.x0).toBeCloseTo(want.x0)
    expect(got.x1).toBeCloseTo(want.x1)
    expect(got.y0).toBeCloseTo(want.y0)
    expect(got.y1).toBeCloseTo(want.y1)
  })
  it('keeps every corner at the padding from its point', () => {
    const hull = pullHull([[0, 0], [10, 0], [5, 8]], 2)
    const far = (p: Point) => Math.min(...[[0, 0], [10, 0], [5, 8]].map(([x, y]) => Math.hypot(p[0] - x, p[1] - y)))
    expect(Math.max(...hull.map(far))).toBeCloseTo(2)
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

describe('recolor', () => {
  it('changes the selected pull only, and Undo takes it back', () => {
    const s = edit(editor([[1], [2]], 1), { kind: 'recolor', color: '3eff3e' })
    expect(s.draft.pulls.map((p) => p.color)).toEqual(['ff0000', '3eff3e'])
    expect(edit(s, { kind: 'undo' }).draft.pulls[1].color).toBe('ff0000')
  })
  it('does nothing with no pull selected', () => {
    const s = editor([[1]], -1)
    expect(edit(s, { kind: 'recolor', color: '3eff3e' })).toBe(s)
  })
})

describe('mobActions', () => {
  const pack = [enemy(1, 0, 0, 7), enemy(2, 0, 0, 7), enemy(3, 0, 0)]
  const labels = (a: ReturnType<typeof mobActions>) => a.map((x) => x.label)
  it('adds an enemy in no pull to the selected one, with its pack', () => {
    const actions = mobActions(pack[0], editor([[3], []], 1), pack)
    expect(labels(actions)).toEqual(['Add it to pull 2', 'Add its pack to pull 2'])
    expect(actions[0].edit).toEqual({ kind: 'toggle', enemies: [pack[0]] })
    expect(actions[1].edit).toEqual({ kind: 'toggle', enemies: [pack[0], pack[1]] })
  })
  it('takes it out of the selected pull that has it', () => {
    expect(labels(mobActions(pack[2], editor([[3]], 0), pack))).toEqual(['Take it out of pull 1'])
  })
  it('selects the other pull it is in', () => {
    const actions = mobActions(pack[1], editor([[1, 2], []], 1), pack)
    expect(labels(actions)).toEqual(['Add it to pull 2', 'Add its pack to pull 2', 'Select its pull (1)'])
    expect(actions[2].edit).toEqual({ kind: 'select', pull: 0 })
  })
})

describe('pullMobs', () => {
  const mob = (npc: number, name: string, health: number): DungeonMob => ({ npc, name, health })
  const enemies = [enemy(1, 0, 0, 0, 10), enemy(2, 0, 0, 0, 20), enemy(3, 0, 0, 0, 20), enemy(4, 0, 0, 0, 30)]
  const byKey = new Map(enemies.map((e) => [enemyKey(e), e]))
  const pull = [1, 2, 3, 4].map((index) => ({ index, clone: 1 }))
  it('groups a pull by name, most first, and sums its health', () => {
    const mobs = new Map([mob(10, 'Guard', 100), mob(20, 'Hound', 50)].map((m) => [m.npc, m]))
    const { groups, health } = pullMobs(pull, byKey, mobs)
    expect(groups.map((g) => [g.enemies.length, g.name])).toEqual([[2, 'Hound'], [1, 'Guard'], [1, 'NPC 30']])
    expect(health).toBe(200)
  })
  it('names them by NPC without mob data', () => {
    const want = { groups: [{ name: 'NPC 10', enemies: [enemies[0]] }], health: 0 }
    expect(pullMobs(pull.slice(0, 1), byKey, new Map())).toEqual(want)
  })
})

describe('dismissOn', () => {
  const key = (k: string) => Object.assign(new Event('keydown'), { key: k })
  it('closes on Escape and a press outside, not on another key or a press inside, until stopped', () => {
    const doc = new EventTarget()
    let within = true
    const close = vi.fn()
    const stop = dismissOn(doc, (t) => t === doc && within, close)
    doc.dispatchEvent(key('Enter'))
    doc.dispatchEvent(new Event('pointerdown'))
    expect(close).not.toHaveBeenCalled()
    doc.dispatchEvent(key('Escape'))
    within = false
    doc.dispatchEvent(new Event('pointerdown'))
    expect(close).toHaveBeenCalledTimes(2)
    stop()
    doc.dispatchEvent(key('Escape'))
    expect(close).toHaveBeenCalledTimes(2)
  })
})

describe('drawings', () => {
  const line: Drawing = { id: 'a', map: 1, kind: 'line', color: 'ff3e3e', points: [[0, 0], [10, 0], [10, 10]] }
  const marker: Drawing = { id: 'b', map: 1, kind: 'marker', color: 'ff3e3e', points: [[20, 20]] }
  const elsewhere: Drawing = { ...marker, id: 'c', map: 2 }

  it('adds and erases drawings as edits Undo takes back', () => {
    let s = editor([[1]], 0)
    s = edit(edit(s, { kind: 'draw', drawing: line }), { kind: 'draw', drawing: marker })
    s = edit(s, { kind: 'erase', id: 'a' })
    expect(s.draft.drawings).toEqual([marker])
    s = edit(s, { kind: 'undo' })
    expect(s.draft.drawings).toEqual([line, marker])
    expect(edit(edit(s, { kind: 'undo' }), { kind: 'undo' }).draft.drawings).toEqual([])
  })

  it.each<[string, [number, number], string | undefined]>([
    ['a line by its nearest segment', [11, 5], 'a'],
    ['a marker by its point', [21, 21], 'b'],
    ['nothing out of reach', [5, 5], undefined],
    ["this floor's drawing, not another floor's at the same place", [20, 20.5], 'b'],
  ])('drawingAt finds %s', (_, p, want) => {
    expect(drawingAt([line, marker, elsewhere], 1, p, 2)?.id).toBe(want)
  })
})

describe('simplifyLine', () => {
  it.each<[string, Point[], number, Point[]]>([
    ['a short line stays', [[0, 0], [1, 1]], 1, [[0, 0], [1, 1]]],
    ['a nearly straight line keeps its ends', [[0, 0], [1, 0.1], [2, -0.1], [3, 0]], 0.5, [[0, 0], [3, 0]]],
    ['a corner stays', [[0, 0], [5, 0.1], [10, 0], [10, 5], [10, 10]], 0.5, [[0, 0], [10, 0], [10, 10]]],
  ])('%s', (_, points, tolerance, want) => {
    expect(simplifyLine(points, tolerance, Infinity)).toEqual(want)
  })
  it('loosens the tolerance until at most max points are left', () => {
    const zigzag = Array.from({ length: 100 }, (_, i): Point => [i, (i % 2) * (i / 10)])
    const got = simplifyLine(zigzag, 0.01, 10)
    expect(got.length).toBeLessThanOrEqual(10)
    expect([got[0], got[got.length - 1]]).toEqual([zigzag[0], zigzag[99]])
  })
})
