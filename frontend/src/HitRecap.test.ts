import { describe, expect, it } from 'vitest'
import type { HitRecap, KeyTrack } from './api'
import { recapOf } from './HitRecap'

const recap = (player: string, clockMS: number): HitRecap => ({ player, clockMS, maxHP: 1, events: [] })
const track = { recaps: [recap('A', 5000), recap('B', 8000), recap('A', 9000), recap('A', 20000)] } as KeyTrack

describe('recapOf', () => {
  it("finds a death's recap at its exact clock", () => {
    expect(recapOf(track, 'A', 20000)?.clockMS).toBe(20000)
  })
  it("finds a big hit's recap as the player's first one from the hit's start", () => {
    expect(recapOf(track, 'A', 6000)?.clockMS).toBe(9000)
  })
  it('is undefined without a later recap or a track', () => {
    expect(recapOf(track, 'B', 8001)).toBeUndefined()
    expect(recapOf(null, 'A', 0)).toBeUndefined()
  })
})
