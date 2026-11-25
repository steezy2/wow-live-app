import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { SimSuggestReport, Suggestion } from './api'
import { rankSuggestions } from './simOptions'
import { SimSuggestions } from './SimSuggestions'

const render = (report: SimSuggestReport) =>
  renderToStaticMarkup(<SimSuggestions report={report} equipped={[]} chunk={20} />)

describe('SimSuggestions', () => {
  it('says so when the sim confirms nothing, and counts what it left out', () => {
    const html = render({ suggestions: [], dropped: 3, notes: ['No top logs found'] })
    expect(html).toContain('No change the sim confirms.')
    expect(html).toContain('3 tested changes were left out')
    expect(html).toContain('No top logs found')
    expect(html).toContain('check it fits your playstyle')
  })

  it('shows a row with its words, evidence and gain', () => {
    const html = render({
      suggestions: [
        {
          kind: 'enchant',
          slot: 'finger1',
          what: 'Enchant Amani Mastery on finger1',
          evidence: '14 of 20 top players',
          gain: 812,
          dpsError: 95,
        },
      ],
      dropped: 0,
      notes: [],
    })
    expect(html).toContain('Enchant Amani Mastery on finger1')
    expect(html).toContain('14 of 20 top players')
    expect(html).toContain('+812')
    expect(html).not.toContain('Combined')
    expect(html).not.toContain('Try it in Top Gear')
  })

  it('ranks a talent with DPS in the main list and the others in a closed group under it, a loss shown', () => {
    const html = render({
      suggestions: [
        { kind: 'talent', what: 'Take Ascendance instead of Echo', evidence: '9 of 10 top players', gain: 400 },
        { kind: 'talentOther', what: 'Take Gust of Wind instead of Thunderstorm', evidence: '8 of 10 top players' },
        { kind: 'talentOther', what: 'Take Totemic Surge instead of Earth Shield', evidence: 'x', gain: -120 },
      ],
      dropped: 0,
      notes: [],
    })
    const main = html.indexOf('Take Ascendance')
    const group = html.indexOf('<details><summary>Other talents (2)</summary>')
    expect(main).toBeGreaterThan(-1)
    expect(group).toBeGreaterThan(main)
    expect(html.indexOf('Take Gust of Wind')).toBeGreaterThan(group)
    expect(html).toContain('No DPS gain in the sim: these are what top players take for utility.')
    expect(html).toContain('+400')
    expect(html).toContain('-120')
    expect(html).not.toContain('No change the sim confirms.')
  })

  it('shows a change gaining only on AoE with both columns', () => {
    const html = render({
      suggestions: [{ kind: 'gem', what: 'Gem', evidence: 'x', aoeGain: 700, aoeError: 40 }],
      dropped: 0,
      notes: [],
    })
    expect(html).toContain('1 target')
    expect(html).toContain('+700')
  })

  it("keeps the server's order for the best gain, else ranks by the fight picked within its groups", () => {
    const list: Suggestion[] = [
      { kind: 'gem', what: 'aoe', evidence: 'x', gain: 50, aoeGain: 900 },
      { kind: 'enchant', what: 'single', evidence: 'x', gain: 500, aoeGain: 100 },
      { kind: 'talent', what: 'middle', evidence: 'x', gain: 300, aoeGain: 300 },
      { kind: 'stat', what: 'stat', evidence: 'x', expected: 300 },
      { kind: 'talentOther', what: 'other', evidence: 'x', gain: -50 },
    ]
    const order = (by: 'best' | 'single' | 'aoe') => rankSuggestions(list, by).map((s) => s.what)
    expect(order('best')).toEqual(['aoe', 'single', 'middle', 'stat', 'other'])
    expect(order('single')).toEqual(['single', 'middle', 'aoe', 'stat', 'other'])
    expect(order('aoe')).toEqual(['aoe', 'middle', 'single', 'stat', 'other'])
  })

  it('offers Top Gear only for a change with a variant, when it can open it', () => {
    const report: SimSuggestReport = {
      suggestions: [
        { kind: 'gem', what: 'Gem', evidence: 'x', variant: { worn: true, index: 2, gem: 9 }, gain: 5 },
        { kind: 'talent', what: 'Take Y', evidence: 'x' },
      ],
      dropped: 0,
      notes: [],
    }
    const html = renderToStaticMarkup(
      <SimSuggestions report={report} equipped={[]} chunk={20} onTry={() => undefined} />,
    )
    expect(html.match(/Try it in Top Gear/g)).toHaveLength(1)
  })
})
