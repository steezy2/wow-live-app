import { expect, it } from 'vitest'
import { noteOf, openSection, setNote } from './note'

it('shows a note that finished while its section was away once, on the way back', () => {
  let close = openSection('read logs')
  setNote('read logs', { busy: 'Reading…' })
  close()
  setNote('read logs', { done: '3 logs read' })
  close = openSection('read logs')
  expect(noteOf('read logs')).toEqual({ done: '3 logs read' })
  close()
  openSection('read logs')
  expect(noteOf('read logs')).toBeNull()
})
