import { describe, expect, it } from 'vitest'
import { locateProposal } from './proposal-anchor'
import { sectionRanges } from './paragraph'

const CONTENT = `\\chapter{绪论}
\\section{研究背景}

本研究具有重要的理论意义和现实价值。

\\section{研究方法}

本研究具有重要的理论意义和现实价值。
`

describe('proposal anchor', () => {
  it('locates a unique verbatim snippet', () => {
    const result = locateProposal(CONTENT, { oldText: '研究背景' })
    expect(result.locatable).toBe(true)
    expect(CONTENT.slice(result.start, result.end)).toBe('研究背景')
  })

  it('reports not_found when the text changed', () => {
    expect(locateProposal(CONTENT, { oldText: '已经不存在的句子' })).toMatchObject({
      locatable: false,
      reason: 'not_found',
    })
  })

  it('uses sectionPath to disambiguate repeated text', () => {
    const result = locateProposal(CONTENT, {
      oldText: '本研究具有重要的理论意义和现实价值。',
      sectionPath: '绪论 / 研究方法',
    })
    expect(result.locatable).toBe(true)
    expect(result.start).toBeGreaterThan(CONTENT.indexOf('\\section{研究方法}'))
  })

  it('falls back to cursor offset when section is not enough', () => {
    const cursor = CONTENT.lastIndexOf('现实价值')
    const result = locateProposal(CONTENT, { oldText: '现实价值' }, cursor)
    expect(result.locatable).toBe(true)
    expect(result.start).toBe(cursor)
  })

  it('refuses to guess when still ambiguous', () => {
    expect(locateProposal(CONTENT, { oldText: '现实价值' })).toMatchObject({
      locatable: false,
      reason: 'ambiguous',
    })
  })

  it('builds section ranges with paths and boundaries', () => {
    const ranges = sectionRanges(CONTENT)
    expect(ranges.map((range) => range.path)).toEqual(['绪论', '绪论 / 研究背景', '绪论 / 研究方法'])
    expect(ranges[0].start).toBeLessThan(ranges[1].start)
  })
})
