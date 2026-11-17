import { expect, test } from 'vitest'
import type { SimChoices, SimItem } from './api'
import { averageLevel, gainShare, gearGroups, itemIDs, simOptionsError } from './simOptions'

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
