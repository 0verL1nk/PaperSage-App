/** 学术规范资源库的前端契约，与 server/internal/norms 的响应字段一一对应。 */

export type NormScenario = 'all' | 'undergraduate_thesis' | 'intro_paper'
export type NormAuthority = 'school' | 'journal' | 'standard' | 'general' | 'user'
export type NormBinding = 'must' | 'should' | 'recommended'
export type NormRuleKind = 'official' | 'general' | 'user' | 'model'
export type NormState = 'active' | 'archived'
export type NormAdoptionState = 'enabled' | 'disabled'

export interface NormPack {
  id: string
  key: string
  name: string
  authority: NormAuthority
  edition: string
  sourceUrl?: string
  sourceNote?: string
  scenario: NormScenario
  isBuiltin: boolean
  state: NormState
  entryCount: number
}

export interface NormExample {
  kind: 'good' | 'bad'
  text: string
  note?: string
}

export interface NormEntry {
  id: string
  packId: string
  packName: string
  code: string
  category: string
  ruleKind: NormRuleKind
  binding: NormBinding
  scenario: NormScenario
  version: number
  sortOrder: number
  state: NormState
  title: string
  ruleText: string
  sourceRef: string
  sourceQuote?: string
  sourceUrl?: string
  applicableSections: string[]
  tags: string[]
  isBuiltin: boolean
}

export interface NormEntryPage {
  items: NormEntry[]
  total: number
  page: number
  pageSize: number
}

export interface NormAdoption {
  entryId: string
  version: number
  state: NormAdoptionState
  overrideText?: string
  hasNewerVersion: boolean
  latestVersion: number
  entry: NormEntry
}

export interface NormVersion {
  version: number
  title: string
  ruleText: string
  rationale?: string
  examples: NormExample[]
  applicableSections: string[]
  sourceRef: string
  sourceQuote?: string
  sourceUrl?: string
  origin: NormRuleKind | 'builtin' | 'import'
  contentHash: string
  createdAt: string
}

export interface NormEntryDetail extends NormEntry {
  rationale?: string
  examples: NormExample[]
  versions: NormVersion[]
}

/** 新建/编辑条目的提交体；编辑时额外带 expectedVersion 做乐观锁。 */
export interface NormEntryInput {
  packId: string
  code: string
  category: string
  ruleKind: NormRuleKind
  binding: NormBinding
  scenario: NormScenario
  sortOrder?: number
  tags: string[]
  title: string
  ruleText: string
  rationale?: string
  examples?: NormExample[]
  applicableSections: string[]
  sourceRef: string
  sourceQuote?: string
  sourceUrl?: string
}

export interface NormEntryUpdateInput extends NormEntryInput {
  expectedVersion: number
  state?: NormState
}

export interface NormPackInput {
  key: string
  name: string
  authority?: NormAuthority
  edition?: string
  sourceUrl?: string
  sourceNote?: string
  scenario?: NormScenario
}

export interface NormEntryQuery {
  q?: string
  category?: string
  scenario?: NormScenario
  tag?: string
  packId?: string
  binding?: NormBinding
  page?: number
  pageSize?: number
}

export const SCENARIO_LABELS: Record<NormScenario, string> = {
  all: '通用',
  undergraduate_thesis: '本科毕业论文',
  intro_paper: '学术小论文',
}

export const BINDING_LABELS: Record<NormBinding, string> = {
  must: '必须',
  should: '应当',
  recommended: '建议',
}

export function bindingLabel(binding: NormBinding): string {
  return BINDING_LABELS[binding] ?? binding
}

export function scenarioLabel(scenario: NormScenario): string {
  return SCENARIO_LABELS[scenario] ?? scenario
}
