import { postJson, type ApiRequestConfig } from './client'
import { AGENT } from './api-routes'
import { useSessionStore } from '@/store'
import { GuestCapabilityError } from './guest-local-workspace'
import type { ProposalNormRef } from './agent-api'
import type { BoldMode } from '@/lib/experiment-table/types'

export interface ExperimentAnalysisTable {
  headers: string[]
  rows: string[][]
  caption: string
  label: string
  bold: BoldMode[]
}

export interface ExperimentAnalysisRequest {
  table: ExperimentAnalysisTable
  language: 'zh' | 'en'
  sectionId?: string
  focus?: string
}

export interface ExperimentCitation {
  row: number
  column: number
  value: string
  rowLabel: string
  columnLabel: string
}

export interface ExperimentParagraph {
  text: string
  citations: ExperimentCitation[]
}

/** 服务端已核对的引用与规范；模型自报的标签不采信。 */
export interface ExperimentAnalysisResult {
  paragraphs: ExperimentParagraph[]
  normRefs: ProposalNormRef[]
  context: {
    normsCount: number
    planStatus: string
    sectionId?: string
    sectionTitle?: string
    warnings: string[]
  }
  model: { name: string; source: string }
}

export async function generateExperimentAnalysis(
  projectId: string,
  body: ExperimentAnalysisRequest,
  signal?: AbortSignal,
): Promise<ExperimentAnalysisResult> {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError('实验分析')
  const config: ApiRequestConfig<ExperimentAnalysisRequest> = {
    timeout: 120_000,
    signal,
    // 面板自己展示加载/错误态，不要叠加全局错误 toast。
    skipGlobalLoading: true,
  }
  return postJson<ExperimentAnalysisResult, ExperimentAnalysisRequest>(
    AGENT.experimentAnalysis(projectId),
    body,
    config,
  )
}
