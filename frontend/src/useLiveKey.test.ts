import { afterEach, expect, test, vi } from 'vitest'
import { joinClock } from './useLiveKey'

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test('a clock is shared, runs while any has joined and skips its ticks while the page is hidden', () => {
  vi.useFakeTimers()
  const page = { hidden: false, addEventListener: vi.fn(), removeEventListener: vi.fn() }
  vi.stubGlobal('document', page)
  const [a, b] = [vi.fn(), vi.fn()]
  const leaveA = joinClock(1000, a)
  const leaveB = joinClock(1000, b)
  vi.advanceTimersByTime(2000)
  expect([a.mock.calls.length, b.mock.calls.length]).toEqual([2, 2])
  expect(vi.getTimerCount()).toBe(1)
  page.hidden = true
  vi.advanceTimersByTime(2000)
  expect(a).toHaveBeenCalledTimes(2)
  leaveA()
  expect(vi.getTimerCount()).toBe(1)
  leaveB()
  expect(vi.getTimerCount()).toBe(0)
  expect(page.removeEventListener).toHaveBeenCalledTimes(1)
})
