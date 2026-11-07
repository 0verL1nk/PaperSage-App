import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import type { ReactNode } from 'react'

const { useLatexFiles, useLatexFileContent, refetch } = vi.hoisted(() => ({
  useLatexFiles: vi.fn(),
  useLatexFileContent: vi.fn(),
  refetch: vi.fn(),
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ projectId: 'project-1' }),
  Link: ({ children }: { children: ReactNode }) => <a href="#">{children}</a>,
}))
vi.mock('@/hooks/use-latex', () => ({ useLatexFiles, useLatexFileContent }))
vi.mock('./ai-panel', () => ({
  AiPanel: ({ mode }: { mode: string }) => <div data-testid="review-agent" data-mode={mode} />,
}))

import { ReviewPage } from './ReviewPage'

const mainFile = { id: 'main.tex', file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 40, last_saved_at: '' }
const validMain = String.raw`\documentclass{article}
\begin{document}
正文
\end{document}`

describe('ReviewPage', () => {
  beforeEach(() => {
    useLatexFiles.mockReset().mockReturnValue({ data: [mainFile], isPending: false, error: null, refetch })
    useLatexFileContent.mockReset().mockReturnValue({ data: { content: validMain }, isPending: false, error: null })
    refetch.mockReset()
  })

  it('shows file-based checks and a direct path back to editing', () => {
    render(<ReviewPage />)
    expect(screen.getByRole('heading', { name: '提交前，把容易漏的地方过一遍' })).toBeInTheDocument()
    expect(screen.getByText('LaTeX 文档结构')).toBeInTheDocument()
    expect(screen.getAllByText('已通过').length).toBeGreaterThan(0)
    expect(screen.getByText(/不会替你判断研究结论或学术质量/)).toBeInTheDocument()
    expect(screen.queryByText(/阶段状态/)).not.toBeInTheDocument()
    expect(screen.getByTestId('review-agent')).toHaveAttribute('data-mode', 'review')
  })

  it('offers a fix path when the project has no main file', () => {
    useLatexFiles.mockReturnValue({ data: [], isPending: false, error: null, refetch })
    useLatexFileContent.mockReturnValue({ data: undefined, isPending: false, error: null })
    render(<ReviewPage />)
    expect(screen.getByText(/项目中没有 main\.tex/)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /打开论文撰写/ })).toBeInTheDocument()
  })

  it('surfaces file-list errors and retries the query', () => {
    useLatexFiles.mockReturnValue({ data: undefined, isPending: false, error: new Error('IndexedDB index missing'), refetch })
    render(<ReviewPage />)
    expect(screen.getByRole('alert')).toHaveTextContent('IndexedDB index missing')
    fireEvent.click(screen.getByRole('button', { name: /重试读取/ }))
    expect(refetch).toHaveBeenCalledOnce()
  })
})
