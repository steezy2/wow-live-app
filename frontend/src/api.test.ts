import { afterEach, expect, test, vi } from 'vitest'
import { FALLBACK_POLL_MS, fetchMobs, fetchSpell, subscribe } from './api'

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

/** A page whose hidden flag the test sets; show() fires its visibilitychange listeners. */
function stubPage() {
  const page = {
    hidden: false,
    listeners: [] as (() => void)[],
    addEventListener: (_type: string, listener: () => void) => page.listeners.push(listener),
    removeEventListener: (_type: string, listener: () => void) => {
      page.listeners = page.listeners.filter((l) => l !== listener)
    },
    show() {
      page.hidden = false
      page.listeners.forEach((listener) => listener())
    },
  }
  vi.stubGlobal('document', page)
  return page
}

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

test('subscribe reads now and on each change, polls while the stream is down and stops when unsubscribed', () => {
  vi.useFakeTimers()
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('location', { host: 'localhost:8080' })
  stubPage()
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

test('subscribe holds changes while the page is hidden and reads once when it shows', () => {
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('location', { host: 'localhost:8080' })
  const page = stubPage()
  page.hidden = true
  const onChange = vi.fn()
  const unsubscribe = subscribe(onChange)
  FakeEventSource.last.listeners.forEach((listener) => listener())
  FakeEventSource.last.listeners.forEach((listener) => listener())
  expect(onChange).not.toHaveBeenCalled()
  page.show()
  page.show()
  expect(onChange).toHaveBeenCalledTimes(1)
  unsubscribe()
  expect(page.listeners).toHaveLength(0)
})

test('fetchSpell asks once for an answer, keeps an unknown spell ID, and asks again after a failure or an unknown name', async () => {
  const answers = [
    new Response(JSON.stringify({ id: 1, name: 'Alter Time', description: 'x' })),
    new Response('{}', { status: 404 }),
    new Response('{}', { status: 404 }),
    new Response('{}', { status: 404 }),
    new Response('{}', { status: 502 }),
    new Response(JSON.stringify({ id: 3, name: 'Barkskin', description: 'y' })),
  ]
  const fetchMock = vi.fn(async () => answers.shift() as Response)
  vi.stubGlobal('fetch', fetchMock)
  expect((await fetchSpell('Alter Time'))?.id).toBe(1)
  expect((await fetchSpell('Alter Time'))?.id).toBe(1)
  expect(await fetchSpell(999)).toBeNull()
  expect(await fetchSpell(999)).toBeNull()
  expect(fetchMock).toHaveBeenCalledTimes(2)
  expect(await fetchSpell('Septic Spatter')).toBeNull() // the log may show it later
  expect(await fetchSpell('Septic Spatter')).toBeNull()
  expect(fetchMock).toHaveBeenCalledTimes(4)
  expect(await fetchSpell('Barkskin')).toBeNull()
  expect((await fetchSpell('Barkskin'))?.id).toBe(3)
  expect(fetchMock).toHaveBeenCalledTimes(6)
})

test('fetchMobs asks for the level and character picked, or the default with none', async () => {
  const fetchMock = vi.fn(async () => new Response('{}'))
  vi.stubGlobal('fetch', fetchMock)
  await fetchMobs(1015, null, '')
  await fetchMobs(1015, 17, '')
  await fetchMobs(1015, null, 'Mage-Realm')
  expect(fetchMock.mock.calls.map((c) => (c as unknown[])[0])).toEqual([
    '/api/mplus/dungeons/1015/mobs',
    '/api/mplus/dungeons/1015/mobs?level=17',
    '/api/mplus/dungeons/1015/mobs?character=Mage-Realm',
  ])
})

test('fetchSpell asks by spell ID apart from names', async () => {
  const info = { id: 270003, name: 'Bone Shield', description: 'x' }
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(info)))
  vi.stubGlobal('fetch', fetchMock)
  expect((await fetchSpell(270003))?.name).toBe('Bone Shield')
  expect(fetchMock.mock.calls[0]).toEqual(['/api/spells?id=270003'])
})
