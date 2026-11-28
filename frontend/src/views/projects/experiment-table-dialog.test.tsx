import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const generateExperimentAnalysis = vi.fn()
vi.mock('@/services/experiment-api', () => ({ generateExperimentAnalysis: (...args: unknown[]) => generateExperimentAnalysis(...args) }))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

import { ExperimentTableDialog } from './experiment-table-dialog'
import { useSettingsStore } from '@/store/settings-store'
import { useSessionStore } from '@/store/session-store'
import type { AllSettings } from '@/services/settings-api'

const settingsWithModel: AllSettings = {
  ui: { theme: 'system', language: 'zh-CN', sidebar_collapsed: false },
  editor: { font_size: 14, tab_size: 2, auto_save: true, auto_save_interval: 30, word_wrap: true },
  agent: {
    active_provider_id: 'p', temperature: 0.7, writing_style: 'academic', memory_enabled: true,
    providers: [{
      id: 'p', name: 'Provider', type: 'openai_compatible',
      base_url: 'https://example.invalid', api_key: 'secret', models: ['A'], active_model: 'A',
    }],
  },
}

const analysisResult = {
  paragraphs: [{
    text: '本文方法准确率达到 92.5%。',
    citations: [{ row: 1, column: 1, value: '92.5', rowLabel: 'Ours', columnLabel: '准确率 (%)' }],
  }],
  normRefs: [{ cite: 'N-001@v1', title: '图表规范', rule: '表格需自明', source: 'GB/T 7714' }],
  context: { normsCount: 1, planStatus: 'adopted', warnings: [] },
  model: { name: 'Provider', source: 'user-settings' },
}

function renderDialog(onInsert = vi.fn()) {
  render(
    <ExperimentTableDialog
      open
      onOpenChange={vi.fn()}
      projectId="project-1"
      activeFilePath="main.tex"
      canInsert
      onInsert={onInsert}
      onBeforeGenerate={vi.fn().mockResolvedValue(undefined)}
    />,
  )
  return onInsert
}

async function parseFixture(user: ReturnType<typeof userEvent.setup>) {
  fireEvent.change(screen.getByPlaceholderText(/方法,准确率,F1/), {
    target: { value: '方法,准确率\nBaseline,88.1\nOurs,92.5' },
  })
  await user.click(screen.getByRole('button', { name: '解析数据' }))
}

beforeEach(() => {
  generateExperimentAnalysis.mockReset()
  useSessionStore.setState({ isGuest: false } as never)
  useSettingsStore.setState({ settings: structuredClone(settingsWithModel), error: null })
})

describe('ExperimentTableDialog', () => {
  it('解析粘贴数据、预览并插入确定性生成的 booktabs 表格', async () => {
    const user = userEvent.setup()
    const onInsert = renderDialog()
    await parseFixture(user)

    expect(screen.getByText('Ours')).toBeInTheDocument()
    expect(document.querySelector('pre')?.textContent).toContain('\\toprule')

    await user.click(screen.getByRole('button', { name: '插入到编辑器' }))
    expect(onInsert).toHaveBeenCalledTimes(1)
    const latex = onInsert.mock.calls[0][0] as string
    expect(latex).toContain('\\toprule')
    expect(latex).toContain('Baseline & 88.1 \\\\')
    expect(latex).toContain('\\label{tab:results}')
  })

  it('解析失败时给出可行动的错误，不生成表格', async () => {
    const user = userEvent.setup()
    renderDialog()
    fireEvent.change(screen.getByPlaceholderText(/方法,准确率,F1/), {
      target: { value: '方法,准确率\nA,90,多余' },
    })
    await user.click(screen.getByRole('button', { name: '解析数据' }))

    expect(screen.getByRole('alert')).toHaveTextContent('第 2 行有 3 列，表头只有 2 列')
  })

  it('生成分析草稿、展示来源 chip，编辑后插入', async () => {
    generateExperimentAnalysis.mockResolvedValue(analysisResult)
    const user = userEvent.setup()
    const onInsert = renderDialog()
    await parseFixture(user)

    await user.click(screen.getByRole('tab', { name: '结果分析' }))
    await user.click(screen.getByRole('button', { name: /基于这张表生成分析草稿/ }))

    await waitFor(() => expect(generateExperimentAnalysis).toHaveBeenCalledTimes(1))
    const [projectId, body] = generateExperimentAnalysis.mock.calls[0]
    expect(projectId).toBe('project-1')
    expect(body).toMatchObject({
      language: 'zh',
      table: { headers: ['方法', '准确率'], rows: [['Baseline', '88.1'], ['Ours', '92.5']] },
    })

    expect(await screen.findByDisplayValue('本文方法准确率达到 92.5%。')).toBeInTheDocument()
    expect(screen.getByText('Ours · 准确率 (%) = 92.5')).toBeInTheDocument()
    expect(screen.getByText('N-001@v1')).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('第 1 段（可编辑）'), { target: { value: '本文方法准确率达到 92.5%，提升明显。' } })
    await user.click(screen.getByRole('button', { name: '插入到编辑器' }))
    // 插入正文前逐段转义：% 必须变成 \%，否则 LaTeX 会把后文当注释。
    expect(onInsert).toHaveBeenCalledWith('本文方法准确率达到 92.5\\%，提升明显。')
  })

  it('未配置模型时显式提示，表格通道仍可用', async () => {
    useSettingsStore.setState({ settings: structuredClone({
      ...settingsWithModel,
      agent: { ...settingsWithModel.agent, providers: [] },
    }) })
    const user = userEvent.setup()
    renderDialog()
    await parseFixture(user)

    await user.click(screen.getByRole('tab', { name: '结果分析' }))
    expect(screen.getByText('尚未配置 AI 模型')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '去设置' })).toBeInTheDocument()
  })

  it('游客使用分析时提示需登录，不发起请求', async () => {
    useSessionStore.setState({ isGuest: true } as never)
    const user = userEvent.setup()
    renderDialog()
    await parseFixture(user)

    await user.click(screen.getByRole('tab', { name: '结果分析' }))
    expect(screen.getByText(/实验分析需要登录后连接云端模型/)).toBeInTheDocument()
    expect(generateExperimentAnalysis).not.toHaveBeenCalled()
  })
})
