import { expect, test } from 'vitest'
import type { SimChoices, SimItem } from './api'
import { gearGroups, simOptionsError } from './simOptions'

const choices = { minLength: 30, maxLength: 900, maxTargets: 20 } as SimChoices
const opts = { fightStyle: 'Patchwerk', length: 300, targets: 1, accuracy: 'normal' }

test('simOptionsError says which option is out of range', () => {
  expect(simOptionsError(opts, choices)).toBeNull()
  expect(simOptionsError({ ...opts, length: 10 }, choices)).toBe('The fight length is 30 to 900 seconds.')
  expect(simOptionsError({ ...opts, targets: 21 }, choices)).toBe('Targets are 1 to 20.')
  expect(simOptionsError({ ...opts, length: 300.5 }, choices)).toContain('fight length')
})

test("gearGroups lists groups in the API's order, worn ones first, and leaves empty groups out", () => {
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
