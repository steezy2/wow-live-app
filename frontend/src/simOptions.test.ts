import { expect, test } from 'vitest'
import type { SimChoices, SimItem, SimReference } from './api'
import {
  averageLevel,
  enchantsFor,
  gainShare,
  gearGroups,
  itemIDs,
  omniumPicks,
  omniumString,
  pickedConsumables,
  simOptionsError,
  togglePick,
  upgradeTrackOf,
  variantLabel,
} from './simOptions'

const choices = { minLength: 30, maxLength: 900, maxTargets: 20 } as SimChoices
const opts = { fightStyle: 'Patchwerk', length: 300, targets: 1, accuracy: 'normal' }

test('simOptionsError says which option is out of range', () => {
  expect(simOptionsError(opts, choices)).toBeNull()
  expect(simOptionsError({ ...opts, length: 10 }, choices)).toBe('The fight length is 30 to 900 seconds.')
  expect(simOptionsError({ ...opts, targets: 21 }, choices)).toBe('Targets are 1 to 20.')
  expect(simOptionsError({ ...opts, length: 300.5 }, choices)).toContain('fight length')
})

test('gearGroups puts both rings under one heading, worn ones first, and leaves empty slots out', () => {
  const equipped: SimItem[] = [
    { slot: 'finger2', id: 2 },
    { slot: 'head', id: 1 },
  ]
  const bags: SimItem[] = [{ slot: 'finger1', id: 3 }]
  const groups = gearGroups(equipped, bags)
  expect(groups.map((g) => g.label)).toEqual(['Head', 'Rings'])
  expect(groups[1].items.map((i) => [i.item.id, i.index, i.worn])).toEqual([
    [2, 0, true],
    [3, 0, false],
  ])
})

test('itemIDs lists each item once, and averageLevel ignores items without a level', () => {
  const items: SimItem[] = [
    { slot: 'head', id: 1, level: 600 },
    { slot: 'neck', id: 2 },
    { slot: 'back', id: 1, level: 610 },
  ]
  expect(itemIDs(items)).toEqual([1, 2])
  expect(averageLevel(items)).toBe(605)
  expect(averageLevel([{ slot: 'head', id: 1 }])).toBe(0)
})

test('gainShare scales a gain to the best one and shows a loss as no bar', () => {
  expect(gainShare(50, 100)).toBe(50)
  expect(gainShare(-5, 100)).toBe(0)
  expect(gainShare(10, 0)).toBe(0)
  expect(gainShare(120, 100)).toBe(100)
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
  const equipped: SimItem[] = [{ slot: 'finger1', id: 1, level: 300 }]
  const bags: SimItem[] = [
    { slot: 'finger1', id: 2, level: 320 },
    { slot: 'finger1', id: 3 },
    { slot: 'finger1', id: 4, level: 310 },
  ]
  expect(gearGroups(equipped, bags)[0].items.map((i) => i.item.id)).toEqual([2, 4, 1, 3])
})

test('upgradeTrackOf finds the track and level from the bonus IDs, and says none for an item on no track', () => {
  const on = upgradeTrackOf(ref, { slot: 'finger1', id: 1, opts: { bonus_id: [7, 101] } })
  expect([on?.track.name, on?.level.level]).toEqual(['Myth', 2])
  expect(upgradeTrackOf(ref, { slot: 'finger1', id: 1, opts: { bonus_id: [7] } })).toBeNull()
  expect(upgradeTrackOf(ref, { slot: 'finger1', id: 1 })).toBeNull()
})

test('enchantsFor lists the enchants of a slot', () => {
  expect(enchantsFor(ref, 'finger2').map((e) => e.id)).toEqual([7964])
  expect(enchantsFor(ref, 'head')).toEqual([])
})

test('variantLabel names what changed', () => {
  const item: SimItem = { slot: 'finger1', id: 1, opts: { bonus_id: [100] } }
  expect(variantLabel(ref, item, { index: 0, upgrade: 2, socket: true, gem: 240883, enchant: 7964 })).toBe(
    'Myth 2/2 · socket + Quick Lapis · Amani Mastery',
  )
  expect(variantLabel(ref, item, { index: 0, gem: 240883 })).toBe('Quick Lapis')
})

test('pickedConsumables leaves out the kinds on simc default', () => {
  expect(pickedConsumables({ flask: 'a', food: '', potion: undefined })).toEqual({ flask: 'a' })
})

test('the Omnium Folio picks read from, and write to, the export format', () => {
  expect(omniumPicks('136814:1/21:1')).toEqual([136814, 21])
  expect(omniumPicks(undefined)).toEqual([])
  expect(omniumString(ref.omnium, [21, 12])).toBe('12:1/21:1')
})

test('togglePick replaces another entry of a choice node and unpicks a picked one', () => {
  const [choice, single] = ref.omnium
  expect(togglePick(choice, [11, 21], 12)).toEqual([21, 12])
  expect(togglePick(choice, [11, 21], 11)).toEqual([21])
  expect(togglePick(single, [], 21)).toEqual([21])
})
