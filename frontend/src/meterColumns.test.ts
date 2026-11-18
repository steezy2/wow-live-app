import { describe, expect, it } from 'vitest'
import type { KeyMeter, OverlayColumn } from './api'
import { OVERLAY_LABELS, overlayColumn, overlayIds, cardGrid } from './meterColumns'

const meter = { rows: [] } as unknown as KeyMeter

describe('overlayColumn', () => {
  // The overlay finds the page's columns by their label, so a renamed page column would leave one without a value.
  it.each(Object.keys(OVERLAY_LABELS) as OverlayColumn[])('%s has a value', (id) => {
    expect(typeof overlayColumn(id, meter, []).value).toBe('function')
  })
})

describe('overlayIds', () => {
  const chosen: OverlayColumn[] = ['dps', 'hps', 'pullDps', 'kicks']
  it.each([
    ['dps', true, ['pullDps', 'dps', 'hps', 'kicks']],
    ['dps', false, ['dps', 'hps', 'kicks']],
    ['hps', true, ['hps', 'dps', 'kicks']],
    ['hps', false, ['hps', 'dps', 'kicks']],
  ] as const)('ranked by %s, Pull DPS %s', (rankBy, pullDps, want) => {
    expect(overlayIds(chosen, rankBy, pullDps)).toEqual(want)
  })
  it('adds the column it ranks by when it was not chosen', () => {
    expect(overlayIds(['kicks'], 'hps', false)).toEqual(['hps', 'kicks'])
  })
})

describe('cardGrid', () => {
  it.each([
    [false, 'minmax(96px, 1fr) repeat(3, 64px)', 96 + 3 * 72],
    [true, 'minmax(72px, 1fr) repeat(3, 44px)', 216],
  ])('dense %s', (dense, template, minWidth) => {
    expect(cardGrid(3, dense)).toEqual({ grid: { gridTemplateColumns: template }, minWidth })
  })
})
