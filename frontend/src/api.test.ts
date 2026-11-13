import { afterEach, expect, test, vi } from 'vitest'
import { FALLBACK_POLL_MS, fetchSpell, subscribe } from './api'

class FakeEventSource {
  static last: FakeEventSource
  onopen: (() => void) | null = null
  onerror: (() => void) | null = null
  listeners: (() => void)[] = []
  closed = false
  constructor() {
    FakeEventSource.last = this
  }
  addEventListener(_type: string, listener: () => void) {
    this.listeners.push(listener)
  }
  close() {
    this.closed = true
  }
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test('subscribe reads now and on each change, polls while the stream is down and stops when unsubscribed', () => {
  vi.useFakeTimers()
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('location', { host: 'localhost:8080' })
  const onChange = vi.fn()
  const unsubscribe = subscribe(onChange)
  const source = FakeEventSource.last
  source.onopen?.()
  source.listeners.forEach((listener) => listener())
  expect(onChange).toHaveBeenCalledTimes(3)
  source.onerror?.()
  source.onerror?.()
  vi.advanceTimersByTime(2 * FALLBACK_POLL_MS)
  expect(onChange).toHaveBeenCalledTimes(5)
  source.onopen?.()
  vi.advanceTimersByTime(2 * FALLBACK_POLL_MS)
  expect(onChange).toHaveBeenCalledTimes(6)
  unsubscribe()
  expect(source.closed).toBe(true)
})

test('fetchSpell asks once for an answer, keeps an unknown ability, and asks again after a failure', async () => {
  const answers = [
    new Response(JSON.stringify({ id: 1, name: 'Alter Time', description: 'x' })),
    new Response('{}', { status: 404 }),
    new Response('{}', { status: 502 }),
    new Response(JSON.stringify({ id: 3, name: 'Barkskin', description: 'y' })),
  ]
  const fetchMock = vi.fn(async () => answers.shift() as Response)
  vi.stubGlobal('fetch', fetchMock)
  expect((await fetchSpell('Alter Time'))?.id).toBe(1)
  expect((await fetchSpell('Alter Time'))?.id).toBe(1)
  expect(await fetchSpell('Septic Spatter')).toBeNull()
  expect(await fetchSpell('Septic Spatter')).toBeNull()
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(await fetchSpell('Barkskin')).toBeNull()
  expect((await fetchSpell('Barkskin'))?.id).toBe(3)
  expect(fetchMock).toHaveBeenCalledTimes(4)
})
