import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { ProjectCompileState } from '@/store/compile-store'

import { CompileProgress } from '../compile-progress'

function compileState(overrides: Partial<ProjectCompileState> = {}): ProjectCompileState {
  return {
    phase: 'compiling',
    projectName: '论文一',
    jobId: 'job-1',
    buildId: null,
    progressMessage: '正在编译（第 1/5 遍）',
    license: null,
    resources: {},
    pdf: null,
    error: null,
    logs: null,
    warning: null,
    pendingUpload: null,
    retryingUpload: false,
    diagnosticStatus: null,
    diagnosticId: null,
    stage: 'compile',
    startedAt: Date.now(),
    formatCacheHit: null,
    ...overrides,
  }
}

describe('CompileProgress', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows the live stage, elapsed time and a working cancel action', () => {
    const onCancel = vi.fn()
    render(
      <CompileProgress
        state={compileState({ stage: 'compile', progressMessage: '正在编译（第 2/5 遍）' })}
        hasPreviousPdf={false}
        onCancel={onCancel}
      />,
    )

    expect(screen.getByText('正在编译（第 2/5 遍）')).toBeInTheDocument()
    expect(screen.getByText(/已用时/)).toBeInTheDocument()
    expect(screen.getByText('排版并生成 PDF')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(5)

    // 取消按钮必须排在阶段列表之前：320px 首屏要能直接看到带文字的操作。
    const cancelButton = screen.getByRole('button', { name: '取消编译' })
    const stageList = screen.getByRole('list', { name: '编译阶段' })
    expect(cancelButton.compareDocumentPosition(stageList) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    fireEvent.click(cancelButton)

    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('ticks the elapsed time every second', () => {
    vi.useFakeTimers()
    const startedAt = Date.now()
    render(
      <CompileProgress state={compileState({ startedAt })} hasPreviousPdf={false} onCancel={vi.fn()} />,
    )

    expect(screen.getByText(/已用时/)).toHaveTextContent('已用时 0:00')

    act(() => {
      vi.advanceTimersByTime(65_000)
    })

    expect(screen.getByText(/已用时/)).toHaveTextContent('已用时 1:05')
  })

  it('explains the first cold compile and the cached fast path', () => {
    const { rerender } = render(
      <CompileProgress state={compileState({ formatCacheHit: false })} hasPreviousPdf={false} onCancel={vi.fn()} />,
    )

    expect(screen.getByText(/首次冷编译/)).toHaveTextContent(/1–3 分钟/)

    rerender(
      <CompileProgress state={compileState({ formatCacheHit: true })} hasPreviousPdf={false} onCancel={vi.fn()} />,
    )

    expect(screen.getByText(/已命中浏览器格式缓存/)).toBeInTheDocument()
    expect(screen.queryByText(/首次冷编译/)).not.toBeInTheDocument()
  })

  it('counts loaded resources and names what is still downloading', () => {
    render(
      <CompileProgress
        state={compileState({
          stage: 'resource',
          resources: {
            'package:ctex.sty': { name: 'ctex.sty', kind: 'package', state: 'loading' },
            'package:amsmath.sty': { name: 'amsmath.sty', kind: 'package', state: 'loaded', cached: true },
          },
        })}
        hasPreviousPdf={false}
        onCancel={vi.fn()}
      />,
    )

    expect(screen.getByText(/已加载 1 项宏包\/字体资源，其中 1 项来自缓存/)).toBeInTheDocument()
    expect(screen.getByText(/正在下载：ctex.sty/)).toBeInTheDocument()
  })

  it('marks the save step and tells the user the previous preview will be replaced', () => {
    render(
      <CompileProgress
        state={compileState({ phase: 'uploading', stage: 'upload', progressMessage: '正在上传 PDF' })}
        hasPreviousPdf
        onCancel={vi.fn()}
      />,
    )

    expect(screen.getByText('保存编译结果')).toBeInTheDocument()
    expect(screen.getByText(/编译完成后会自动替换当前预览/)).toBeInTheDocument()
    // 上传阶段无法取消，不给出点了没反应的按钮。
    expect(screen.queryByRole('button', { name: '取消编译' })).not.toBeInTheDocument()
  })
})
