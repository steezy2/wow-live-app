import { describe, expect, it } from 'vitest'
import type { KeyMeter, OverlayColumn } from './api'
import { OVERLAY_LABELS, overlayColumn } from './meterColumns'

const meter = { rows: [] } as unknown as KeyMeter

describe('overlayColumn', () => {
  // The overlay finds the page's columns by their label, so a renamed page column would leave one without a value.
  it.each(Object.keys(OVERLAY_LABELS) as OverlayColumn[])('%s has a value', (id) => {
    expect(typeof overlayColumn(id, meter, []).value).toBe('function')
  })
})
