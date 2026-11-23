/**
 * 跨章节检查的呈现逻辑：覆盖完整性、问题分组、历史对比。
 * 这里只做确定性派生，不产生语义结论——语义结论必须来自模型报告。
 */
import type {
  ChapterReviewPayload,
  ReviewBasis,
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

/**
 * 旧报告一致性检查：修复前可能存下「维度标 findings 却没有问题条目」的记录。
 * 返回非空表示这份历史报告不可作为审查结果展示（前端隔离，不改写既有数据）。
 */
export function reviewReportProblems(report: ChapterReviewPayload): string[] {
  const problems: string[] = []
  const dimensions = report.dimensions ?? []
  const keys = new Set<string>()
  for (const dimension of dimensions) {
    if (keys.has(dimension.key)) {
      problems.push(`${REVIEW_DIMENSION_LABEL[dimension.key] ?? dimension.key}：维度重复`)
    }
    keys.add(dimension.key)
  }
  for (const key of REVIEW_DIMENSION_ORDER) {
    if (!keys.has(key)) problems.push(`${REVIEW_DIMENSION_LABEL[key]}：缺少维度结果`)
  }
  const counts = new Map<string, number>()
  for (const finding of report.findings ?? []) {
    counts.set(finding.dimension, (counts.get(finding.dimension) ?? 0) + 1)
    if (!keys.has(finding.dimension)) {
      problems.push(`问题 ${finding.id} 的维度不在四维结果中`)
    }
  }
  for (const dimension of dimensions) {
    const count = counts.get(dimension.key) ?? 0
    const label = REVIEW_DIMENSION_LABEL[dimension.key] ?? dimension.key
    if (dimension.status === 'findings' && count === 0) {
      problems.push(`${label}：标记为发现问题，但没有问题条目`)
    }
    if (dimension.status === 'checked' && count > 0) {
      problems.push(`${label}：标记为已检查，但包含问题条目`)
    }
  }
  return problems
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

/**
 * 历史排序必须按解析后的时间比较。ISO 字符串带不同时区偏移时，
 * 直接 localeCompare 会把 +08:00 的旧记录误判为“最新”。
 */
export function sortReportsByUpdatedAt<T extends { updatedAt: string }>(reports: T[]): T[] {
  return [...reports].sort((a, b) => timeValue(b.updatedAt) - timeValue(a.updatedAt))
}

function timeValue(value: string | undefined): number {
  if (!value) return 0
  const time = Date.parse(value)
  return Number.isNaN(time) ? 0 : time
}

/** 章节 id → 标题映射，来源是工具写入的 planSections 与问题自身的章节标题。 */
export function planSectionTitles(report: ChapterReviewPayload): Map<string, string> {
  const titles = new Map<string, string>()
  for (const section of report.input?.planSections ?? []) {
    if (section.id && section.title) titles.set(section.id, section.title)
  }
  for (const finding of report.findings ?? []) {
    if (finding.sectionId && finding.sectionTitle) titles.set(finding.sectionId, finding.sectionTitle)
  }
  return titles
}

const REVIEW_TEXT_REPLACEMENTS: Array<[RegExp, string]> = [
  [/norms\.available\s*=\s*false/gi, '学术规范库尚未接入'],
  [/norms\.available\s*=\s*true/gi, '学术规范库已接入'],
  [/norms\.entries/gi, '已采用的规范'],
  [/get_chapter_review_context/gi, '检查上下文'],
  [/\bwarnings\b/gi, '材料提示'],
  [/status\s*=\s*adopted/gi, '章节计划已采用'],
  [/\brevision\s*[=:]?\s*(\d+)/gi, '第 $1 版'],
  [/\bnot_read\b/gi, '尚未核对'],
  [/\bnot_reported\b/gi, '未报告'],
  [/\bnot_checked\b/gi, '未检查'],
  [/\binsufficient\b/gi, '材料不足'],
  [/\breported\b/gi, '已报告'],
  [/\bchecked\b/gi, '已检查'],
  [/\bfailed\b/gi, '检查失败'],
  [/\bfindings\b/gi, '问题'],
  [/\bpayload\b/gi, '报告内容'],
  [/\bfinding\b/gi, '问题'],
  [/\bbasis\b/gi, '依据'],
  [/\bplan\b/gi, '章节计划'],
  [/\br(\d+)\b/gi, '课题要求'],
]

/**
 * 把模型 notes 里残留的内部字段/枚举/章节 id 换成学生能懂的中文。
 * 只用于展示；逐字引文（quote）不经过这里。
 */
export function humanizeReviewText(text: string, titles: Map<string, string>): string {
  if (!text) return text
  let out = text
  for (const [pattern, replacement] of REVIEW_TEXT_REPLACEMENTS) {
    out = out.replace(pattern, replacement)
  }
  out = out.replace(/\bs(\d+)\b/gi, (_match, digits: string) => {
    const title = titles.get(`s${digits}`)
    return title ? `「${title}」` : '对应章节'
  })
  return out
}

export function displayBasisRef(basis: ReviewBasis, titles: Map<string, string>): string {
  if (basis.kind === 'plan') return titles.get(basis.ref) ?? '章节计划'
  if (basis.kind === 'text') return basis.filePath || basis.ref
  return basis.ref || '未命名依据'
}

/** 面向学生的错误文案；英文库信息统一折叠到“技术细节”。 */
export function friendlyErrorMessage(error: unknown): string {
  const raw = error instanceof Error ? error.message : String(error ?? '')
  if (/[一-鿿]/.test(raw)) return raw
  if (/network|fetch|timeout|econn|socket/i.test(raw)) return '网络连接失败，请检查网络后重试。'
  return '暂时无法读取检查数据，请稍后重试。'
}

export function reviewLocation(finding: ReviewFinding, titles?: Map<string, string>): string {
  const section = finding.sectionTitle
    || (finding.sectionId ? titles?.get(finding.sectionId) : undefined)
    || '未标注章节'
  const file = finding.filePath ? ` · ${finding.filePath}` : ''
  const lines = finding.lineStart
    ? ` · 第 ${finding.lineStart}${finding.lineEnd && finding.lineEnd !== finding.lineStart ? `–${finding.lineEnd}` : ''} 行`
    : ''
  return `${section}${file}${lines}`
}
