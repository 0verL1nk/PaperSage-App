/**
 * 跨章节检查的呈现逻辑：覆盖完整性、问题分组、历史对比。
 * 这里只做确定性派生，不产生语义结论——语义结论必须来自模型报告。
 */
import type {
  ChapterReviewPayload,
  ReviewBasisKind,
  ReviewDimension,
  ReviewDimensionKey,
  ReviewDimensionStatus,
  ReviewFinding,
  ReviewSeverity,
  ReviewSourceKind,
} from '@/types/research'

export const REVIEW_DIMENSION_ORDER: ReviewDimensionKey[] = [
  'goal_conclusion', 'evidence_support', 'cross_section', 'norm_compliance',
]

export const REVIEW_DIMENSION_LABEL: Record<ReviewDimensionKey, string> = {
  goal_conclusion: '章节目标与结论',
  evidence_support: '论断证据支撑',
  cross_section: '章节间重复/矛盾',
  norm_compliance: '学术规范符合度',
}

export const REVIEW_DIMENSION_HINT: Record<ReviewDimensionKey, string> = {
  goal_conclusion: '章节计划写的目标与正文实际结论是否一致。',
  evidence_support: '正文与计划中的论断是否有已读资料或原文证据支撑。',
  cross_section: '不同章节之间是否出现重复表述或相互矛盾。',
  norm_compliance: '是否符合项目已采用的学术规范条目。',
}

export const REVIEW_STATUS_LABEL: Record<ReviewDimensionStatus, string> = {
  checked: '已检查',
  findings: '发现问题',
  insufficient: '证据不足',
  not_checked: '未检查',
  failed: '检查失败',
}

export const REVIEW_SEVERITY_LABEL: Record<ReviewSeverity, string> = {
  high: '严重',
  medium: '中等',
  low: '轻微',
}

export const REVIEW_BASIS_LABEL: Record<ReviewBasisKind, string> = {
  norm: '规范条目',
  plan: '章节计划',
  text: '章节原文',
  source: '资料证据',
}

export const REVIEW_SOURCE_LABEL: Record<ReviewSourceKind, string> = {
  latex: '章节文件',
  plan: '章节计划',
  document: '参考资料',
  source: '来源版本',
  norm: '学术规范',
}

export interface ReviewCoverage {
  complete: boolean
  covered: number
  reasons: string[]
}

export function reviewCoverage(report: ChapterReviewPayload): ReviewCoverage {
  const reasons: string[] = []
  let covered = 0
  for (const key of REVIEW_DIMENSION_ORDER) {
    const dimension = report.dimensions?.find((item) => item.key === key)
    if (!dimension) {
      reasons.push(`${REVIEW_DIMENSION_LABEL[key]}：本次没有返回结果`)
      continue
    }
    if (dimension.status === 'checked' || dimension.status === 'findings') {
      covered += 1
      continue
    }
    reasons.push(`${REVIEW_DIMENSION_LABEL[key]}：${dimension.note?.trim() || REVIEW_STATUS_LABEL[dimension.status]}`)
  }
  return { complete: covered === REVIEW_DIMENSION_ORDER.length, covered, reasons }
}

export function dimensionOf(report: ChapterReviewPayload, key: ReviewDimensionKey): ReviewDimension | undefined {
  return report.dimensions?.find((item) => item.key === key)
}

/** 问题按固定维度顺序分组；未知维度也保留，避免报告字段漂移时静默丢问题。 */
export function findingsByDimension(report: ChapterReviewPayload): Array<{ key: ReviewDimensionKey; label: string; findings: ReviewFinding[] }> {
  const groups = new Map<ReviewDimensionKey, ReviewFinding[]>()
  for (const key of REVIEW_DIMENSION_ORDER) groups.set(key, [])
  for (const finding of report.findings ?? []) {
    const list = groups.get(finding.dimension)
    if (list) list.push(finding)
    else groups.set(finding.dimension, [finding])
  }
  return [...groups.entries()]
    .filter(([, findings]) => findings.length > 0)
    .map(([key, findings]) => ({ key, label: REVIEW_DIMENSION_LABEL[key] ?? key, findings }))
}

export function findingKey(finding: ReviewFinding): string {
  const where = finding.sectionId ?? finding.filePath ?? ''
  const text = (finding.quote ?? finding.problem).replace(/\s+/g, ' ').trim()
  return `${finding.dimension}|${where}|${text}`
}

export interface ReviewComparison {
  added: ReviewFinding[]
  resolved: ReviewFinding[]
  persisting: ReviewFinding[]
}

/** 与上一次报告对比：按「维度 + 定位 + 引文/问题」判断新增、已消失、仍存在。 */
export function compareReviewReports(previous: ChapterReviewPayload, latest: ChapterReviewPayload): ReviewComparison {
  const previousKeys = new Set((previous.findings ?? []).map(findingKey))
  const latestKeys = new Set((latest.findings ?? []).map(findingKey))
  const added = (latest.findings ?? []).filter((finding) => !previousKeys.has(findingKey(finding)))
  const resolved = (previous.findings ?? []).filter((finding) => !latestKeys.has(findingKey(finding)))
  const persisting = (latest.findings ?? []).filter((finding) => previousKeys.has(findingKey(finding)))
  return { added, resolved, persisting }
}

export function formatReviewTime(value: string | undefined): string {
  if (!value) return '时间未知'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '时间未知'
  return date.toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })
}

export function reviewLocation(finding: ReviewFinding): string {
  const section = finding.sectionTitle || finding.sectionId || '未标注章节'
  const file = finding.filePath ? ` · ${finding.filePath}` : ''
  const lines = finding.lineStart
    ? ` · 第 ${finding.lineStart}${finding.lineEnd && finding.lineEnd !== finding.lineStart ? `–${finding.lineEnd}` : ''} 行`
    : ''
  return `${section}${file}${lines}`
}
