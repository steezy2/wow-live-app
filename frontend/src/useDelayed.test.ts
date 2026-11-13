import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { delayFlag } from './useDelayed'

describe('delayFlag', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('turns true only after the delay', () => {
    const set = vi.fn()
    delayFlag(set, true, 250)
    vi.advanceTimersByTime(249)
    expect(set).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1)
    expect(set).toHaveBeenCalledWith(true)
  })

  it('never turns true when the wait is stopped first', () => {
    const set = vi.fn()
    delayFlag(set, true, 250)()
    vi.advanceTimersByTime(1000)
    expect(set).not.toHaveBeenCalled()
  })

  it('turns false at once', () => {
    const set = vi.fn()
    delayFlag(set, false, 250)
    expect(set).toHaveBeenCalledWith(false)
  })
})
