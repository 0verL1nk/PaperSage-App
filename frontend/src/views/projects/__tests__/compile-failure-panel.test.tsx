import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import type { CompileIssue } from '@/lib/latex/compile-log'
import * as agentApi from '@/services/agent-api'
import { CompileFailurePanel, type CompileFailurePanelProps } from '../compile-failure-panel'

const ISSUES: CompileIssue[] = [
  {
    severity: 'error',
    filePath: 'chapters/intro.tex',
    line: 42,
    message: 'Undefined control sequence.',
    source: 'latex',
  },
  {
    severity: 'error',
    filePath: null,
    line: 7,
    message: "I couldn't open database file refs.bib",
    source: 'bibtex',
  },
  {
    severity: 'warning',
    filePath: 'main.tex',
    line: 3,
    message: 'Reference `fig:1` undefined on input line 3.',
    source: 'latex',
  },
]

function props(overrides: Partial<CompileFailurePanelProps> = {}): CompileFailurePanelProps {
  return {
    projectId: 'p1',
    error: '! Undefined control sequence.',
    logs: '--- pdflatex pass 1 ---\n(./chapters/intro.tex\n! Undefined control sequence.\nl.42 \\badcommand',
    issues: ISSUES,
    rootFile: 'main.tex',
    compiler: 'pdflatex',
    failureKind: 'latex',
    diagnosticStatus: null,
    diagnosticId: null,
    isMobile: false,
    isCompiling: false,
    isGuest: false,
    onRetryCompile: vi.fn(),
    onReinitResources: vi.fn(),
    onSelectIssue: vi.fn(),
    onBeforeApply: vi.fn().mockResolvedValue(undefined),
    onApplied: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  vi.restoreAllMocks()
})

describe('CompileFailurePanel', () => {
  it('lists located errors and only makes navigable ones clickable', () => {
    const onSelect = vi.fn()
    render(<CompileFailurePanel {...props({ onSelectIssue: onSelect })} />)

    expect(screen.getByText('chapters/intro.tex:42')).toBeInTheDocument()
    expect(screen.getByText('Undefined control sequence.')).toBeInTheDocument()
    expect(screen.getByText(/BibTeX 日志只给出辅助文件位置/)).toBeInTheDocument()

    const navigate = screen.getAllByTitle('跳转到报错位置')
    expect(navigate).toHaveLength(1)
    fireEvent.click(navigate[0])
    expect(onSelect).toHaveBeenCalledWith(ISSUES[0])
  })

  it('keeps AI diagnosis disabled for guests and says why', () => {
    render(<CompileFailurePanel {...props({ isGuest: true })} />)
    const buttons = screen.getAllByRole('button', { name: /AI 诊断/ })
    expect(buttons).toHaveLength(2)
    buttons.forEach((button) => expect(button).toBeDisabled())
    expect(screen.getByText('登录后可使用 AI 诊断与修复建议。')).toBeInTheDocument()
  })

  it('shows the model root cause and applies the reviewed diff only after confirmation', async () => {
    const diagnose = vi.spyOn(agentApi, 'diagnoseCompileError').mockResolvedValue({
      rootCause: '未定义命令 \\badcommand，可能是拼写错误。',
      patch: {
        proposalId: 'prop-1',
        filePath: 'main.tex',
        oldText: '\\badcommand',
        newText: '\\textbf{badcommand}',
        note: '换成本文可用的命令。',
        status: 'pending',
      },
      notice: null,
    })
    const apply = vi.spyOn(agentApi, 'applyProposal').mockResolvedValue({ status: 'applied' })
    const onBeforeApply = vi.fn().mockResolvedValue(undefined)
    const onApplied = vi.fn()
    render(<CompileFailurePanel {...props({ onBeforeApply, onApplied })} />)

    fireEvent.click(screen.getAllByRole('button', { name: /AI 诊断/ })[0])
    expect(await screen.findByText(/未定义命令/)).toBeInTheDocument()
    expect(diagnose).toHaveBeenCalledWith('p1', expect.objectContaining({
      filePath: 'chapters/intro.tex',
      line: 42,
      errorMessage: 'Undefined control sequence.',
      compiler: 'pdflatex',
      rootFile: 'main.tex',
    }))
    expect(apply).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '应用修复' }))
    await waitFor(() => expect(apply).toHaveBeenCalledWith('prop-1'))
    expect(onBeforeApply).toHaveBeenCalledTimes(1)
    expect(onApplied).toHaveBeenCalledWith('main.tex')
    expect(await screen.findByText(/修复已应用，请重新编译验证/)).toBeInTheDocument()
  })

  it('renders no fabricated suggestion when diagnosis fails', async () => {
    vi.spyOn(agentApi, 'diagnoseCompileError').mockRejectedValue(
      new Error('尚未配置 AI 模型或选择无效：请检查设置中的服务商和模型'),
    )
    render(<CompileFailurePanel {...props()} />)

    fireEvent.click(screen.getAllByRole('button', { name: /AI 诊断/ })[0])
    expect(await screen.findByText(/尚未配置 AI 模型/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /去设置中配置模型/ })).toHaveAttribute('href', '/settings')
    expect(screen.queryByRole('button', { name: '应用修复' })).not.toBeInTheDocument()
  })

  it('surfaces the notice and offers no apply button when the model gives no usable patch', async () => {
    vi.spyOn(agentApi, 'diagnoseCompileError').mockResolvedValue({
      rootCause: '导言区缺少宏包。',
      patch: null,
      notice: '模型给出的定位片段与当前稿件不一致，未生成可应用修改',
    })
    render(<CompileFailurePanel {...props()} />)

    fireEvent.click(screen.getAllByRole('button', { name: /AI 诊断/ })[0])
    expect(await screen.findByText(/不一致，未生成可应用修改/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '应用修复' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '忽略' })).not.toBeInTheDocument()
  })
})
