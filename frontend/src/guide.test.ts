import { describe, expect, it } from 'vitest'
import { guideMet, guideSteps, showGuide } from './guide'
import type { SettingsView } from './api'

describe('showGuide', () => {
  it.each([
    ['new user', false, 0, false, true],
    ['setup done', true, 0, false, false],
    ['existing user with saved keys', false, 12, false, false],
    ['saved keys not known yet', false, null, false, false],
    ['reopened with saved keys', false, 12, true, true],
    ['reopened but done', true, 12, true, false],
  ])('%s', (_, done, keys, reopened, want) => {
    expect(showGuide(done, keys, reopened)).toBe(want)
  })
})

describe('guideMet', () => {
  const view = { logsDirFound: true, lastLogMS: 5, character: 'A-B' } as SettingsView
  it('ignores the optional step', () => {
    expect(guideMet(guideSteps(view, false))).toBe(true)
  })
  it('needs every required step', () => {
    expect(guideMet(guideSteps({ ...view, character: '' }, true))).toBe(false)
  })
})
