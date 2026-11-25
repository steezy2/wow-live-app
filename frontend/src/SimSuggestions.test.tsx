import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { SimSuggestReport } from './api'
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
  })
})
