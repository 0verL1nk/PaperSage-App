import { describe, expect, it } from 'vitest'

import type { ChapterReviewPayload, ReviewFinding } from '@/types/research'
import { friendlyErrorMessage } from '@/services/errors'
import {
  compareReviewReports,
  displayBasisRef,
  findingsByDimension,
  humanizeReviewText,
  planSectionTitles,
  reviewCoverage,
  reviewLocation,
  reviewReportProblems,
  sortReportsByUpdatedAt,
} from './cross-chapter-review'

function finding(overrides: Partial<ReviewFinding> = {}): ReviewFinding {
  return {
    id: 'f1',
    dimension: 'goal_conclusion',
    severity: 'high',
    sectionId: 's1',
    sectionTitle: '方法',
    filePath: 'chapters/method.tex',
    quote: '样本 120 份',
    problem: '方法与结论样本量不一致',
    basis: [{ kind: 'text', ref: 'chapters/method.tex', quote: '样本 120 份' }],
    nextStep: '统一样本量',
    ...overrides,
  }
}

function report(overrides: Partial<ChapterReviewPayload> = {}): ChapterReviewPayload {
  return {
    input: { normsAvailable: false, normsNote: '规范库尚未接入' },
    dimensions: [
      { key: 'goal_conclusion', status: 'findings' },
      { key: 'evidence_support', status: 'insufficient', note: '缺少资料原文' },
      { key: 'cross_section', status: 'checked' },
      { key: 'norm_compliance', status: 'not_checked', note: '规范库尚未接入' },
    ],
    findings: [finding()],
    sourcesRead: [{ kind: 'latex', name: 'chapters/method.tex' }],
    ...overrides,
  }
}

describe('reviewCoverage', () => {
  it('treats insufficient/not_checked as incomplete and keeps the reason', () => {
    const coverage = reviewCoverage(report())
    expect(coverage.complete).toBe(false)
    expect(coverage.covered).toBe(2)
    expect(coverage.reasons.join('')).toContain('缺少资料原文')
    expect(coverage.reasons.join('')).toContain('规范库尚未接入')
  })

  it('is complete only when all four dimensions are checked or findings', () => {
    const coverage = reviewCoverage(report({
      dimensions: [
        { key: 'goal_conclusion', status: 'checked' },
        { key: 'evidence_support', status: 'checked' },
        { key: 'cross_section', status: 'checked' },
        { key: 'norm_compliance', status: 'checked' },
      ],
      findings: [],
    }))
    expect(coverage).toEqual({ complete: true, covered: 4, reasons: [] })
  })

  it('reports a missing dimension instead of silently treating it as passed', () => {
    const coverage = reviewCoverage(report({ dimensions: [{ key: 'goal_conclusion', status: 'checked' }] }))
    expect(coverage.complete).toBe(false)
    expect(coverage.reasons).toHaveLength(3)
  })
})

describe('compareReviewReports', () => {
  it('separates added, resolved and persisting findings', () => {
    const previous = report({ findings: [finding({ id: 'old', problem: '旧问题' }), finding({ id: 'same', quote: '仍存在' })] })
    const latest = report({ findings: [finding({ id: 'same', quote: '仍存在' }), finding({ id: 'new', quote: '新问题' })] })
    const comparison = compareReviewReports(previous, latest)
    expect(comparison.added.map((item) => item.id)).toEqual(['new'])
    expect(comparison.resolved.map((item) => item.id)).toEqual(['old'])
    expect(comparison.persisting.map((item) => item.id)).toEqual(['same'])
  })
})

describe('findingsByDimension', () => {
  it('groups findings in the fixed dimension order and drops empty groups', () => {
    const groups = findingsByDimension(report({
      findings: [
        finding({ id: 'a', dimension: 'cross_section' }),
        finding({ id: 'b', dimension: 'goal_conclusion' }),
      ],
    }))
    expect(groups.map((group) => group.key)).toEqual(['goal_conclusion', 'cross_section'])
  })
})

describe('reviewLocation', () => {
  it('renders section, file and line range without bare ids', () => {
    expect(reviewLocation(finding({ lineStart: 2, lineEnd: 3 }))).toBe('方法 · chapters/method.tex · 第 2–3 行')
  })

  it('falls back to the plan title when sectionTitle is absent', () => {
    expect(reviewLocation(finding({ sectionTitle: undefined }), new Map([['s1', '方法']]))).toBe('方法 · chapters/method.tex')
  })
})

describe('sortReportsByUpdatedAt', () => {
  it('orders by parsed time when ISO offsets differ', () => {
    const reports = [
      { id: 'old', updatedAt: '2026-11-23T05:00:00+08:00' },
      { id: 'new', updatedAt: '2026-11-23T03:30:00+00:00' },
    ]
    expect(sortReportsByUpdatedAt(reports).map((item) => item.id)).toEqual(['new', 'old'])
  })
})

describe('humanizeReviewText', () => {
  it('replaces internal enums, ids and plan section ids with student-facing Chinese', () => {
    const text = humanizeReviewText(
      'norms.available=false；status=adopted，revision=1；计划 s3 的 not_read 论断；项目要求 r1',
      new Map([['s3', '实验结果']]),
    )
    expect(text).toContain('学术规范库尚未接入')
    expect(text).toContain('章节计划已采用')
    expect(text).toContain('第 1 版')
    expect(text).toContain('「实验结果」')
    expect(text).toContain('尚未核对')
    expect(text).toContain('课题要求')
    expect(text).not.toMatch(/norms\.available|not_read|status=adopted|\bs3\b|\br1\b/)
  })

  it('falls back to a generic chapter label when the id is unknown', () => {
    expect(humanizeReviewText('计划 s9', new Map())).toContain('对应章节')
  })

  it('does not duplicate a title already written after the id, and keeps requirement numbers', () => {
    const text = humanizeReviewText(
      '章节计划设有 s5「讨论」；项目要求 r1「参考文献按 GB/T 7714」；检查上下文返回 norms.available=false，材料提示亦提示规范库未接入',
      new Map([['s5', '讨论']]),
    )
    expect(text).toContain('「讨论」')
    expect(text).not.toContain('「讨论」「讨论」')
    expect(text).toContain('课题要求 1')
    expect(text).not.toContain('项目要求课题要求')
    expect(text).toContain('材料提示也说明')
    expect(text).not.toContain('检查上下文返回')
  })
})

describe('planSectionTitles and displayBasisRef', () => {
  const payload = report({
    input: { normsAvailable: false, planSections: [{ id: 's3', title: '实验结果' }] },
  })

  it('maps plan section ids to titles', () => {
    expect(planSectionTitles(payload).get('s3')).toBe('实验结果')
  })

  it('shows the title instead of the raw plan id in basis refs', () => {
    expect(displayBasisRef({ kind: 'plan', ref: 's3' }, planSectionTitles(payload))).toBe('实验结果')
    expect(displayBasisRef({ kind: 'text', ref: 'chapters/method.tex', filePath: 'chapters/method.tex' }, new Map())).toBe('chapters/method.tex')
  })
})

describe('friendlyErrorMessage', () => {
  it('translates network library errors and preserves Chinese backend messages', () => {
    expect(friendlyErrorMessage(new Error('Network request failed.'))).toBe('网络连接失败，请检查网络后重试。')
    expect(friendlyErrorMessage(new Error('还没有跨章节检查报告'))).toBe('还没有跨章节检查报告')
  })
})

describe('reviewReportProblems', () => {
  it('accepts a consistent report', () => {
    expect(reviewReportProblems(report())).toEqual([])
  })

  it('flags a dimension marked findings with no finding (the pre-fix legacy record)', () => {
    const problems = reviewReportProblems(report({ findings: [] }))
    expect(problems.join('')).toContain('目标与结论')
    expect(problems.join('')).toContain('标记为发现问题，但没有问题条目')
  })

  it('flags a checked dimension that contains findings', () => {
    const problems = reviewReportProblems(report({
      dimensions: [
        { key: 'goal_conclusion', status: 'checked' },
        { key: 'evidence_support', status: 'insufficient', note: '缺少资料原文' },
        { key: 'cross_section', status: 'checked' },
        { key: 'norm_compliance', status: 'not_checked', note: '规范库尚未接入' },
      ],
    }))
    expect(problems.join('')).toContain('标记为已检查，但包含问题条目')
  })

  it('flags missing dimensions instead of treating them as passed', () => {
    const problems = reviewReportProblems(report({ dimensions: [{ key: 'goal_conclusion', status: 'findings' }] }))
    expect(problems.filter((problem) => problem.includes('缺少维度结果'))).toHaveLength(3)
  })
})
