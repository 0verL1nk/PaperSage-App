/** 科研组织产物（与 server/internal/research 的 payload 契约对应）。 */

export type ResearchArtifactKind =
  | 'literature_note'
  | 'literature_comparison'
  | 'chapter_plan'
  | 'chapter_digest'
  | 'chapter_review'

export type ResearchFieldStatus = 'reported' | 'not_read' | 'not_reported' | 'insufficient'

export interface ResearchEvidenceRef {
  sourceVersionId?: string
  documentId?: string
  startByte?: number
  endByte?: number
  quote?: string
}

export interface ResearchField {
  text: string
  status: ResearchFieldStatus
  scope?: string
  evidence?: ResearchEvidenceRef[]
}

export interface LiteratureNotePayload {
  source: ResearchEvidenceRef
  title?: string
  question: ResearchField
  method: ResearchField
  objects: ResearchField
  conditions: ResearchField
  results: ResearchField
  limitations: ResearchField
}

export interface ComparisonGroup {
  sourceId: string
  label?: string
  values: ResearchField[]
}

export interface LiteratureComparisonPayload {
  question: string
  axes: string[]
  groups: ComparisonGroup[]
  commonalities?: ResearchField[]
  differences?: ResearchField[]
  conflicts?: ResearchField[]
  missing?: ResearchField[]
  conditions?: string
}

export interface SectionClaim {
  text: string
  status: ResearchFieldStatus
  evidence?: ResearchEvidenceRef[]
}

export interface ChapterSection {
  id: string
  title: string
  responsibility: string
  questions?: string[]
  claims?: SectionClaim[]
  dependencies?: string[]
  order: number
  filePath?: string
  startByte?: number
  endByte?: number
}

export interface ChapterPlanPayload {
  sections: ChapterSection[]
}

export interface ChapterDigestPayload {
  input: { latexFileId: string; revision?: string }
  sectionId: string
  content: string
  terms?: string[]
  coverage: string[]
  status: string
}

/** 跨章节检查：语义结论只来自真实模型，页面只负责如实呈现覆盖与依据。 */
export type ReviewDimensionKey = 'goal_conclusion' | 'evidence_support' | 'cross_section' | 'norm_compliance'
export type ReviewDimensionStatus = 'checked' | 'findings' | 'insufficient' | 'not_checked' | 'failed'
export type ReviewSeverity = 'high' | 'medium' | 'low'
export type ReviewBasisKind = 'norm' | 'plan' | 'text' | 'source'
export type ReviewSourceKind = 'latex' | 'plan' | 'document' | 'source' | 'norm'

export interface ReviewDimension {
  key: ReviewDimensionKey
  status: ReviewDimensionStatus
  note?: string
}

export interface ReviewBasis {
  kind: ReviewBasisKind
  ref: string
  filePath?: string
  quote?: string
  lineStart?: number
  lineEnd?: number
}

export interface ReviewFinding {
  id: string
  dimension: ReviewDimensionKey
  severity: ReviewSeverity
  sectionId?: string
  sectionTitle?: string
  filePath?: string
  quote?: string
  lineStart?: number
  lineEnd?: number
  problem: string
  basis: ReviewBasis[]
  nextStep: string
  confidence?: 'high' | 'low'
  anchorVerified?: boolean
}

export interface ReviewSource {
  kind: ReviewSourceKind
  name: string
}

export interface ReviewNormEntry {
  id: string
  /** 后端 norms.Entry.Code：与 version 一起构成稳定引用号。 */
  code?: string
  version?: number
  title: string
  rule: string
  category?: string
  source?: string
  scope?: string
}

export interface ChapterReviewPayload {
  input: {
    planArtifactId?: string
    planRevision?: number
    planStatus?: 'adopted' | 'candidate' | 'missing'
    planSections?: Array<{ id: string; title: string }>
    normsAvailable: boolean
    normsNote?: string
    norms?: ReviewNormEntry[]
  }
  dimensions: ReviewDimension[]
  findings: ReviewFinding[]
  sourcesRead: ReviewSource[]
}

export type ResearchPayload =
  | LiteratureNotePayload
  | LiteratureComparisonPayload
  | ChapterPlanPayload
  | ChapterDigestPayload
  | ChapterReviewPayload

export type ResearchRevisionStatus = 'candidate' | 'adopted' | 'rejected' | 'superseded'

export interface ResearchRevision {
  id: string
  revision: number
  status: ResearchRevisionStatus
  origin: 'user' | 'agent'
  payload: ResearchPayload
  runId?: string | null
  createdAt: string
}

export interface ResearchArtifact {
  id: string
  projectId: string
  kind: ResearchArtifactKind
  state: string
  currentRevision: number
  adoptedRevision: number | null
  createdAt: string
  updatedAt: string
  current?: ResearchRevision
  adopted?: ResearchRevision
  revisions?: ResearchRevision[]
}

export interface ResearchDependencyInput {
  kind: 'source_version' | 'source_segment' | 'artifact_revision' | 'run'
  id: string
}

export interface CreateResearchArtifactInput {
  kind: ResearchArtifactKind
  payload: ResearchPayload
  origin?: 'user' | 'agent'
  runId?: string
  clientRequestId?: string
  dependencies?: ResearchDependencyInput[]
}

export interface AddResearchRevisionInput {
  expectedRevision: number
  payload: ResearchPayload
  origin?: 'user' | 'agent'
  runId?: string
  dependencies?: ResearchDependencyInput[]
}

export const RESEARCH_KIND_LABEL: Record<ResearchArtifactKind, string> = {
  literature_note: '阅读记录',
  literature_comparison: '文献比较',
  chapter_plan: '章节计划',
  chapter_digest: '章节摘要',
  chapter_review: '跨章节检查',
}

export const RESEARCH_STATUS_LABEL: Record<ResearchRevisionStatus, string> = {
  candidate: '候选',
  adopted: '已采用',
  rejected: '已拒绝',
  superseded: '已被替换',
}
