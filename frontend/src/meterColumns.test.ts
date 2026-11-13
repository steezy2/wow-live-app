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
  const chosen: OverlayColumn[] = ['dps', 'pullDps', 'kicks']
  it('puts Pull DPS first for one card', () => {
    expect(overlayIds(chosen, true)).toEqual(['pullDps', 'dps', 'kicks'])
  })
  it('drops Pull DPS from the two cards', () => {
    expect(overlayIds(chosen, false)).toEqual(['dps', 'kicks'])
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
