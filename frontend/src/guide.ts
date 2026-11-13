import type { SettingsView } from './api'

/** One step of Home's setup guide: what to check, whether it holds, and the Settings panel (an element id) that fixes it. */
export interface GuideStep {
  label: string
  met: boolean
  anchor: string
  optional?: boolean
}

/** Whether Home shows the setup guide: until it is done, and only for a new user (no saved keys; null while not yet
 * known) unless it was reopened from Settings. */
export function showGuide(setupDone: boolean, savedKeys: number | null, reopened: boolean): boolean {
  return !setupDone && (reopened || savedKeys === 0)
}

/** The guide's steps: the log folder found, a combat log written, a character picked, and the optional Warcraft Logs keys. */
export function guideSteps(view: SettingsView, keysSet: boolean): GuideStep[] {
  return [
    { label: 'The combat log folder is found', met: view.logsDirFound, anchor: 'settings-game' },
    { label: 'The game has written a combat log (turn on Advanced Combat Logging, type /combatlog)', met: view.lastLogMS > 0, anchor: 'settings-game' },
    { label: 'A character is picked for Warcraft Logs imports and parses', met: view.character !== '', anchor: 'settings-import' },
    { label: 'Warcraft Logs keys, for key reviews and estimates', met: keysSet, anchor: 'settings-wcl-keys', optional: true },
  ]
}

/** Whether every required step is met. */
export const guideMet = (steps: GuideStep[]): boolean => steps.every((s) => s.optional || s.met)
