import { describe, expect, it } from 'vitest'

import type { ChapterReviewPayload, ReviewFinding } from '@/types/research'
import { compareReviewReports, findingsByDimension, reviewCoverage, reviewLocation, reviewReportProblems } from './cross-chapter-review'

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
