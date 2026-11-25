import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'

import type { ChapterReviewPayload, ResearchArtifact, ReviewDimension, ReviewFinding } from '@/types/research'

const mocks = vi.hoisted(() => ({
  useLatexFiles: vi.fn(),
  useLatexFileContent: vi.fn(),
  refetch: vi.fn(),
  reportsRefetch: vi.fn(),
  send: vi.fn().mockResolvedValue(undefined),
  cancel: vi.fn().mockResolvedValue(undefined),
  chatState: { streaming: false, cancelling: false, restoring: false, restoreFailed: false, error: null as string | null },
  sessionState: { isGuest: false },
  settingsState: {
    settings: {
      agent: { providers: [{ id: 'p', api_key: 'k', base_url: 'u', models: ['m'] }] },
    } as { agent?: { providers: unknown[]; server_fallback?: { configured: boolean } } } | null,
    fetchSettings: vi.fn(),
  },
  reportsState: { data: [] as ResearchArtifact[], isPending: false, error: null as Error | null },
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ projectId: 'project-1' }),
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}))
vi.mock('@/hooks/use-latex', () => ({ useLatexFiles: mocks.useLatexFiles, useLatexFileContent: mocks.useLatexFileContent }))
vi.mock('./ai-panel', () => ({
  AiPanel: ({ mode }: { mode: string }) => <div data-testid="review-agent" data-mode={mode} />,
}))
vi.mock('@/services/research-api', () => ({
  useResearchArtifacts: () => ({
    data: mocks.reportsState.data, isPending: mocks.reportsState.isPending,
    error: mocks.reportsState.error, refetch: mocks.reportsRefetch,
  }),
}))
vi.mock('@/store/chat-store', () => ({
  useChatStore: () => ({ ...mocks.chatState, send: mocks.send, cancel: mocks.cancel }),
}))
vi.mock('@/store/session-store', () => ({
  useSessionStore: (selector: (state: { isGuest: boolean }) => unknown) => selector(mocks.sessionState),
}))
vi.mock('@/store/settings-store', () => ({
  useSettingsStore: (selector: (state: typeof mocks.settingsState) => unknown) => selector(mocks.settingsState),
}))

import { ReviewPage } from './ReviewPage'

const mainFile = { id: 'main.tex', file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 40, last_saved_at: '' }
const validMain = String.raw`\documentclass{article}
\begin{document}
正文
\end{document}`

function reviewArtifact(options: {
  id?: string
  updatedAt?: string
  dimensions?: ReviewDimension[]
  findings?: ReviewFinding[]
  norms?: ChapterReviewPayload['input']['norms']
} = {}): ResearchArtifact {
  const payload: ChapterReviewPayload = {
    input: options.norms
      ? { planStatus: 'adopted', normsAvailable: true, norms: options.norms }
      : { planStatus: 'adopted', normsAvailable: false, normsNote: '学术规范库尚未接入' },
    dimensions: options.dimensions ?? [
      { key: 'goal_conclusion', status: 'findings' },
      { key: 'evidence_support', status: 'insufficient', note: '缺少资料原文' },
      { key: 'cross_section', status: 'checked' },
      { key: 'norm_compliance', status: 'not_checked', note: '规范库尚未接入' },
    ],
    findings: options.findings ?? [{
      id: 'f1', dimension: 'goal_conclusion', severity: 'high',
      sectionId: 's1', sectionTitle: '方法', filePath: 'chapters/method.tex', lineStart: 2, lineEnd: 2,
      quote: '本研究共收集 120 份有效样本。', problem: '方法与结论的样本量不一致', nextStep: '统一两处样本量',
      basis: [
        { kind: 'text', ref: 'chapters/method.tex', quote: '本研究共收集 120 份有效样本。' },
        { kind: 'plan', ref: 's1', quote: '说明方法与样本量' },
      ],
      anchorVerified: true, confidence: 'high',
    }],
    sourcesRead: [
      { kind: 'latex', name: 'chapters/method.tex' },
      { kind: 'plan', name: '章节计划 v1（已采用）' },
    ],
  }
  const id = options.id ?? 'report-1'
  return {
    id, projectId: 'project-1', kind: 'chapter_review', state: 'active',
    currentRevision: 1, adoptedRevision: null,
    createdAt: options.updatedAt ?? '2026-11-23T01:00:00Z',
    updatedAt: options.updatedAt ?? '2026-11-23T01:00:00Z',
    current: {
      id: `${id}-rev`, revision: 1, status: 'candidate', origin: 'agent',
      createdAt: options.updatedAt ?? '2026-11-23T01:00:00Z', payload,
    },
  }
}

describe('ReviewPage', () => {
  beforeEach(() => {
    mocks.useLatexFiles.mockReset().mockReturnValue({ data: [mainFile], isPending: false, error: null, refetch: mocks.refetch })
    mocks.useLatexFileContent.mockReset().mockReturnValue({ data: { content: validMain }, isPending: false, error: null })
    mocks.refetch.mockReset()
    mocks.reportsRefetch.mockReset()
    mocks.send.mockClear()
    mocks.cancel.mockClear()
    mocks.chatState.streaming = false
    mocks.chatState.cancelling = false
    mocks.chatState.restoring = false
    mocks.chatState.restoreFailed = false
    mocks.chatState.error = null
    mocks.sessionState.isGuest = false
    mocks.settingsState.settings = {
      agent: { providers: [{ id: 'p', api_key: 'k', base_url: 'u', models: ['m'] }] },
    }
    mocks.reportsState.data = []
    mocks.reportsState.isPending = false
    mocks.reportsState.error = null
  })

  it('separates the deterministic structure checks from the AI review section', () => {
    render(<ReviewPage />)
    expect(screen.getByRole('heading', { name: '提交前，把容易漏的地方过一遍' })).toBeInTheDocument()
    expect(screen.getByText('基础结构检查')).toBeInTheDocument()
    expect(screen.getByText('本地确定性检查')).toBeInTheDocument()
    expect(screen.getByText('跨章节检查')).toBeInTheDocument()
    expect(screen.getByText('AI 模型审阅')).toBeInTheDocument()
    expect(screen.getByText('LaTeX 文档结构')).toBeInTheDocument()
    expect(screen.getByText('还没有跨章节检查报告')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /运行跨章节检查/ })).toBeEnabled()
    expect(screen.getByTestId('review-agent')).toHaveAttribute('data-mode', 'review')
  })

  it('starts a review through the shared chat store', () => {
    render(<ReviewPage />)
    fireEvent.click(screen.getByRole('button', { name: /运行跨章节检查/ }))
    expect(mocks.send).toHaveBeenCalledWith('project-1', expect.stringContaining('get_chapter_review_context'))
  })

  it('shows progress and allows cancelling while the review runs', () => {
    mocks.chatState.streaming = true
    render(<ReviewPage />)
    expect(screen.getByText('正在读取材料并生成检查报告')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /取消检查/ }))
    expect(mocks.cancel).toHaveBeenCalledOnce()
  })

  it('renders findings with expandable evidence and refuses a pass when coverage is incomplete', () => {
    mocks.reportsState.data = [reviewArtifact()]
    render(<ReviewPage />)
    expect(screen.getByText('方法与结论的样本量不一致')).toBeInTheDocument()
    expect(screen.getByText('查看依据（2）')).toBeInTheDocument()
    expect(screen.getByText('引文已核对')).toBeInTheDocument()
    expect(screen.getByText(/本次覆盖不完整（2\/4 个维度完成）/)).toBeInTheDocument()
    expect(screen.getAllByText(/缺少资料原文/).length).toBeGreaterThan(0)
    expect(screen.queryByText(/未发现问题/)).not.toBeInTheDocument()
    expect(screen.queryByText(/全部通过/)).not.toBeInTheDocument()
    expect(screen.getByText(/本次读取的材料（2）/)).toBeInTheDocument()
  })

  it('shows only a qualified result when all four dimensions are covered and no findings exist', () => {
    mocks.reportsState.data = [reviewArtifact({
      dimensions: [
        { key: 'goal_conclusion', status: 'checked' },
        { key: 'evidence_support', status: 'checked' },
        { key: 'cross_section', status: 'checked' },
        { key: 'norm_compliance', status: 'checked' },
      ],
      findings: [],
    })]
    render(<ReviewPage />)
    expect(screen.getByText(/本次四个维度均已完成，未发现问题/)).toBeInTheDocument()
    expect(screen.getByText(/仍需结合导师意见与原始文献人工核验/)).toBeInTheDocument()
  })

  it('lists the norms that were in effect, with clause text and source on demand', () => {
    mocks.reportsState.data = [reviewArtifact({
      norms: [{
        id: 'entry-1',
        code: 'GBT7714-TYPE-MARKS',
        version: 2,
        title: '按文献类型标注类型标识',
        rule: '期刊文章用 [J] 标注。',
        source: 'GB/T 7714—2015',
      }],
    })]
    render(<ReviewPage />)
    expect(screen.getByText(/本次生效的规范依据 · 1 条/)).toBeInTheDocument()
    expect(screen.getByText(/\[N-GBT7714-TYPE-MARKS@v2\] 按文献类型标注类型标识/)).toBeInTheDocument()
    expect(screen.getByText('期刊文章用 [J] 标注。')).toBeInTheDocument()
    expect(screen.getByText('出处：GB/T 7714—2015')).toBeInTheDocument()
  })

  it('compares the selected report with the previous one', () => {
    mocks.reportsState.data = [
      reviewArtifact({
        id: 'new', updatedAt: '2026-11-23T02:00:00Z',
        dimensions: [
          { key: 'goal_conclusion', status: 'checked' },
          { key: 'evidence_support', status: 'insufficient', note: '缺少资料原文' },
          { key: 'cross_section', status: 'findings' },
          { key: 'norm_compliance', status: 'not_checked', note: '规范库尚未接入' },
        ],
        findings: [{
          id: 'f-new', dimension: 'cross_section', severity: 'medium', sectionTitle: '结论',
          problem: '结论与结果重复表述', nextStep: '合并重复段落',
          basis: [{ kind: 'text', ref: 'chapters/conclusion.tex', quote: '重复表述' }],
        }],
      }),
      reviewArtifact({ id: 'old', updatedAt: '2026-11-23T01:00:00Z' }),
    ]
    render(<ReviewPage />)
    expect(screen.getByLabelText('查看历史报告')).toBeInTheDocument()
    expect(screen.getByText(/新增 1 条，已消失 1 条，仍存在 0 条/)).toBeInTheDocument()
  })

  it('sorts history by parsed time across mixed ISO offsets', () => {
    mocks.reportsState.data = [
      reviewArtifact({
        id: 'tz-old', updatedAt: '2026-11-23T05:00:00+08:00',
        findings: [{
          id: 'old-f', dimension: 'goal_conclusion', severity: 'high', sectionTitle: '方法',
          problem: '旧报告的独有问题', nextStep: '旧下一步',
          basis: [{ kind: 'text', ref: 'chapters/method.tex', quote: '旧依据' }],
        }],
      }),
      reviewArtifact({
        id: 'tz-new', updatedAt: '2026-11-23T03:30:00+00:00',
        dimensions: [
          { key: 'goal_conclusion', status: 'checked' },
          { key: 'evidence_support', status: 'insufficient', note: '缺少资料原文' },
          { key: 'cross_section', status: 'findings' },
          { key: 'norm_compliance', status: 'not_checked', note: '规范库尚未接入' },
        ],
        findings: [{
          id: 'new-f', dimension: 'cross_section', severity: 'medium', sectionTitle: '结论',
          problem: '新报告的独有问题', nextStep: '新下一步',
          basis: [{ kind: 'text', ref: 'chapters/conclusion.tex', quote: '新依据' }],
        }],
      }),
    ]
    render(<ReviewPage />)
    expect(screen.getByText('新报告的独有问题')).toBeInTheDocument()
    expect(screen.queryByText('旧报告的独有问题')).not.toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '查看历史报告' })).toHaveValue('tz-new')
    expect(screen.getByText(/新增 1 条，已消失 1 条，仍存在 0 条/)).toBeInTheDocument()
  })

  it('humanizes the comparison block as well as the report cards', () => {
    mocks.reportsState.data = [
      reviewArtifact({
        id: 'cmp-new', updatedAt: '2026-11-23T03:30:00+00:00',
        dimensions: [
          { key: 'goal_conclusion', status: 'checked' },
          { key: 'evidence_support', status: 'insufficient', note: '缺少资料原文' },
          { key: 'cross_section', status: 'findings' },
          { key: 'norm_compliance', status: 'not_checked', note: '规范库尚未接入' },
        ],
        findings: [{
          id: 'cmp-new-f', dimension: 'cross_section', severity: 'high', sectionTitle: '结论',
          problem: '新问题：计划 s2 的 not_read 论断', nextStep: '补齐 s2 的证据',
          basis: [{ kind: 'text', ref: 'chapters/conclusion.tex', quote: '新依据' }],
        }],
      }),
      reviewArtifact({
        id: 'cmp-old', updatedAt: '2026-11-23T05:00:00+08:00',
        findings: [{
          id: 'cmp-old-f', dimension: 'goal_conclusion', severity: 'medium', sectionTitle: '方法',
          problem: '旧问题：计划 s4 的 not_read 论断', nextStep: '补齐 s4 的证据',
          basis: [{ kind: 'text', ref: 'chapters/method.tex', quote: '旧依据' }],
        }],
      }),
    ]
    render(<ReviewPage />)
    expect(screen.getByText(/新增：/, { selector: 'p' })).toHaveTextContent(/对应章节/)
    expect(screen.getByText(/已消失：/, { selector: 'p' })).toHaveTextContent(/对应章节/)
    expect(screen.queryByText(/not_read/)).not.toBeInTheDocument()
    expect(screen.queryByText(/计划 s2/)).not.toBeInTheDocument()
    expect(screen.queryByText(/计划 s4/)).not.toBeInTheDocument()
  })

  it('renders model notes and plan references in student-facing Chinese', () => {
    const artifact = reviewArtifact()
    artifact.current!.payload = {
      input: { planStatus: 'adopted', planSections: [{ id: 's3', title: '实验结果' }], normsAvailable: false },
      dimensions: [
        { key: 'goal_conclusion', status: 'findings' },
        { key: 'evidence_support', status: 'insufficient', note: 'norms.available=false；status=adopted，revision=1；请核对计划 s3 的 not_read 论断' },
        { key: 'cross_section', status: 'checked' },
        { key: 'norm_compliance', status: 'not_checked', note: '规范库未接入' },
      ],
      findings: [{
        id: 'f1', dimension: 'goal_conclusion', severity: 'high', sectionId: 's3', sectionTitle: '实验结果',
        problem: '计划 s3 的结论缺少依据', nextStep: '补齐 s3 的证据',
        basis: [{ kind: 'plan', ref: 's3', quote: '结果是否支持结论？' }],
      }],
      sourcesRead: [],
    } as ChapterReviewPayload
    mocks.reportsState.data = [artifact]
    render(<ReviewPage />)
    expect(screen.getAllByText(/学术规范库尚未接入/).length).toBeGreaterThan(0)
    expect(screen.queryByText(/norms\.available/)).not.toBeInTheDocument()
    expect(screen.queryByText(/not_read/)).not.toBeInTheDocument()
    expect(screen.getAllByText(/「实验结果」/).length).toBeGreaterThan(0)
    expect(screen.queryByText(/计划 s3/)).not.toBeInTheDocument()
  })

  it('points to settings when no model is configured, without pretending the check can run', () => {
    mocks.settingsState.settings = { agent: { providers: [] } }
    render(<ReviewPage />)
    expect(screen.getByText(/尚未配置 AI 模型/)).toBeInTheDocument()
    expect(screen.getByText('去设置模型')).toBeInTheDocument()
    expect(screen.getByText(/先点上方「去设置模型」/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /运行跨章节检查/ })).toBeDisabled()
  })

  it('tells guests to sign in instead of offering a dead button', () => {
    mocks.sessionState.isGuest = true
    render(<ReviewPage />)
    expect(screen.getByText(/登录后可使用跨章节检查/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /运行跨章节检查/ })).not.toBeInTheDocument()
  })

  it('surfaces report loading errors in Chinese with technical details collapsed', () => {
    mocks.reportsState.error = new Error('offline')
    render(<ReviewPage />)
    expect(screen.getByText(/历史报告加载失败：暂时无法读取数据/)).toBeInTheDocument()
    expect(screen.getByText('offline')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重试读取/ }))
    expect(mocks.reportsRefetch).toHaveBeenCalledOnce()
  })

  it('isolates legacy reports whose dimension status does not match the findings', () => {
    mocks.reportsState.data = [
      reviewArtifact({ id: 'valid', updatedAt: '2026-11-23T10:13:00Z' }),
      reviewArtifact({ id: 'legacy', updatedAt: '2026-11-23T09:58:00Z', findings: [] }),
    ]
    render(<ReviewPage />)
    expect(screen.getByText(/已隔离 1 份修复前的无效报告/)).toBeInTheDocument()
    expect(screen.getByText(/修复后的新报告不受影响/)).toBeInTheDocument()
    expect(screen.getByText('方法与结论的样本量不一致')).toBeInTheDocument()
    expect(screen.queryByLabelText('查看历史报告')).not.toBeInTheDocument()
  })

  it('never presents a legacy invalid report as a valid result when it is the only one', () => {
    mocks.reportsState.data = [reviewArtifact({ id: 'legacy', findings: [] })]
    render(<ReviewPage />)
    expect(screen.getByText(/已隔离 1 份修复前的无效报告/)).toBeInTheDocument()
    expect(screen.getByText('还没有有效的跨章节检查报告')).toBeInTheDocument()
    expect(screen.queryByText('查看历史报告')).not.toBeInTheDocument()
    expect(screen.queryByText(/本次覆盖不完整/)).not.toBeInTheDocument()
  })

  it('offers a fix path when the project has no main file', () => {
    mocks.useLatexFiles.mockReturnValue({ data: [], isPending: false, error: null, refetch: mocks.refetch })
    mocks.useLatexFileContent.mockReturnValue({ data: undefined, isPending: false, error: null })
    render(<ReviewPage />)
    expect(screen.getByText(/项目中没有 main\.tex/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /打开论文撰写/ })).toBeInTheDocument()
  })

  it('surfaces file-list errors and retries the query', () => {
    mocks.useLatexFiles.mockReturnValue({ data: undefined, isPending: false, error: new Error('IndexedDB index missing'), refetch: mocks.refetch })
    render(<ReviewPage />)
    expect(screen.getByText(/暂时无法读取数据/)).toBeInTheDocument()
    expect(screen.getByText('IndexedDB index missing')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /重试读取/ }))
    expect(mocks.refetch).toHaveBeenCalledOnce()
  })
})
