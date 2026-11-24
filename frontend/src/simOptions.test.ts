import { expect, test } from 'vitest'
import type { SimChoices, SimItem, SimReference } from './api'
import {
  comparedCount,
  enhancementVariants,
  enchantsFor,
  gainShare,
  gearGroups,
  omniumCompared,
  omniumPicks,
  omniumString,
  simOptionsError,
  togglePick,
  upgradeTrackOf,
} from './simOptions'

const choices = { minLength: 30, maxLength: 900, maxTargets: 20 } as SimChoices
const opts = { fightStyle: 'Patchwerk', length: 300, targets: 1, accuracy: 'normal' }

test('simOptionsError says which option is out of range', () => {
  expect(simOptionsError(opts, choices)).toBeNull()
  expect(simOptionsError({ ...opts, length: 10 }, choices)).toBe('The fight length is 30 to 900 seconds.')
  expect(simOptionsError({ ...opts, targets: 21 }, choices)).toBe('Targets are 1 to 20.')
  expect(simOptionsError({ ...opts, length: 300.5 }, choices)).toContain('fight length')
})

test("gearGroups lists groups in the API's order and leaves empty groups out", () => {
  const equipped: SimItem[] = [
    { slot: 'finger2', group: 'finger', id: 2 },
    { slot: 'head', group: 'head', id: 1 },
  ]
  const bags: SimItem[] = [{ slot: 'finger1', group: 'finger', id: 3 }]
  const groups = gearGroups(equipped, bags, ['head', 'neck', 'finger', 'main_hand'])
  expect(groups.map((g) => g.label)).toEqual(['Head', 'Rings'])
  expect(groups[1].items.map((i) => [i.item.id, i.index, i.worn])).toEqual([
    [2, 0, true],
    [3, 0, false],
  ])
})

test('gainShare scales a gain to the best one and shows a loss as no bar', () => {
  expect(gainShare(50, 100)).toBe(50)
  expect(gainShare(-5, 100)).toBe(0)
  expect(gainShare(10, 0)).toBe(0)
})

const ref = {
  upgrades: [
    {
      group: 618,
      name: 'Myth',
      levels: [
        { bonusID: 100, level: 1, itemLevel: 318 },
        { bonusID: 101, level: 2, itemLevel: 321 },
      ],
    },
  ],
  enchants: [
    { id: 7964, name: 'Amani Mastery', icon: '', slots: ['finger1', 'finger2'] },
    { id: 8038, name: 'Acuity', icon: '', slots: ['main_hand'] },
  ],
  gems: [{ id: 240883, name: 'Quick Lapis', icon: '' }],
  omnium: [
    { id: 1, choice: true, entries: [{ id: 11 }, { id: 12 }] },
    { id: 2, entries: [{ id: 21 }] },
  ],
} as unknown as SimReference

test('gearGroups lists the highest item level first, worn or carried, and items without a level last', () => {
  const equipped: SimItem[] = [{ slot: 'finger1', group: 'finger', id: 1, level: 300 }]
  const bags: SimItem[] = [
    { slot: 'finger1', group: 'finger', id: 2, level: 320 },
    { slot: 'finger1', group: 'finger', id: 3 },
    { slot: 'finger1', group: 'finger', id: 4, level: 310 },
  ]
  expect(gearGroups(equipped, bags, ['finger'])[0].items.map((i) => i.item.id)).toEqual([2, 4, 1, 3])
})

test('upgradeTrackOf finds the track and level from the bonus IDs, and says none for an item on no track', () => {
  const on = upgradeTrackOf(ref, { slot: 'finger1', group: 'finger', id: 1, opts: { bonus_id: [7, 101] } })
  expect([on?.track.name, on?.level.level]).toEqual(['Myth', 2])
  expect(upgradeTrackOf(ref, { slot: 'finger1', group: 'finger', id: 1, opts: { bonus_id: [7] } })).toBeNull()
  expect(upgradeTrackOf(ref, { slot: 'finger1', group: 'finger', id: 1 })).toBeNull()
})

test('enchantsFor lists the enchants of a slot', () => {
  expect(enchantsFor(ref, 'finger2').map((e) => e.id)).toEqual([7964])
  expect(enchantsFor(ref, 'head')).toEqual([])
})

test('comparedCount multiplies the kinds with two or more values picked', () => {
  expect(comparedCount({})).toBe(1)
  expect(comparedCount({ flask: ['a'], food: [] })).toBe(1)
  expect(comparedCount({ flask: ['a', 'b'], food: ['x', 'y', 'z'], potion: ['p'] })).toBe(6)
})

test('the Omnium Folio picks read from, and write to, the export format', () => {
  expect(omniumPicks('136814:1/21:1')).toEqual([136814, 21])
  expect(omniumPicks(undefined)).toEqual([])
  expect(omniumString(ref.omnium, [21, 12])).toBe('12:1/21:1')
  expect(omniumString(ref.omnium, [12, 21, 11])).toBe('11:1/12:1/21:1')
})

test('omniumCompared multiplies the picks of the nodes with two or more', () => {
  expect(omniumCompared(ref.omnium, [11, 21])).toBe(1)
  expect(omniumCompared(ref.omnium, [11, 12, 21])).toBe(2)
})

test('togglePick adds an entry beside the others of its node and removes a picked one', () => {
  expect(togglePick([11, 21], 12)).toEqual([11, 21, 12])
  expect(togglePick([11, 21], 11)).toEqual([21])
  expect(togglePick([], 21)).toEqual([21])
})

test('enhancementVariants tries each enchant and gem on the worn items they fit, leaving out what is worn', () => {
  const worn: SimItem[] = [
    { slot: 'finger1', group: 'finger', id: 1, opts: { enchant_id: [7964], gem_id: [240883] } },
    { slot: 'finger2', group: 'finger', id: 2, opts: { gem_id: [240883] } },
    { slot: 'head', group: 'head', id: 3 },
  ]
  expect(enhancementVariants(worn, ref, [7964], [240883, 5])).toEqual([
    { worn: true, index: 0, gem: 5 },
    { worn: true, index: 1, enchant: 7964 },
    { worn: true, index: 1, gem: 5 },
  ])
})
