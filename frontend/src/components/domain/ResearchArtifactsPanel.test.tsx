import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'

import { ResearchArtifactsPanel } from './ResearchArtifactsPanel'
import { FrontendApiError } from '@/services/errors'
import type { ResearchArtifact } from '@/types/research'

vi.mock('sonner', () => ({ toast: { error: vi.fn(), success: vi.fn() } }))

const adopt = vi.fn()
const reject = vi.fn()
const remove = vi.fn()
const addRevision = vi.fn()
const create = vi.fn()
const refetch = vi.fn()

const state: { artifacts: ResearchArtifact[]; isLoading: boolean; error: Error | null } = {
  artifacts: [], isLoading: false, error: null,
}

vi.mock('@/services/research-api', () => ({
  useResearchArtifacts: () => ({ data: state.artifacts, isLoading: state.isLoading, error: state.error, refetch }),
  useCreateResearchArtifact: () => ({ mutateAsync: create, isPending: false }),
  useAddResearchRevision: () => ({ mutateAsync: addRevision, isPending: false }),
  useAdoptResearchRevision: () => ({ mutateAsync: adopt, isPending: false }),
  useRejectResearchRevision: () => ({ mutateAsync: reject, isPending: false }),
  useDeleteResearchArtifact: () => ({ mutateAsync: remove, isPending: false }),
}))

vi.mock('@/hooks/use-latex', () => ({
  useLatexFiles: () => ({ data: [{ file_path: 'main.tex' }, { file_path: 'chapters/method.tex' }] }),
}))

function planArtifact(): ResearchArtifact {
  return {
    id: 'a1', projectId: 'p1', kind: 'chapter_plan', state: 'active',
    currentRevision: 2, adoptedRevision: 1, createdAt: '', updatedAt: '',
    current: {
      id: 'r2', revision: 2, status: 'candidate', origin: 'agent', createdAt: '',
      payload: { sections: [{ id: 's1', title: '引言', responsibility: '提出问题', order: 1 }] },
    },
    adopted: {
      id: 'r1', revision: 1, status: 'adopted', origin: 'user', createdAt: '',
      payload: { sections: [{ id: 's1', title: '旧引言', responsibility: '旧职责', order: 1 }] },
    },
    revisions: [],
  }
}

function backendError(message: string, status = 404) {
  return new FrontendApiError(message, { kind: 'backend', status })
}

describe('ResearchArtifactsPanel', () => {
  beforeEach(() => {
    state.artifacts = []
    state.isLoading = false
    state.error = null
    adopt.mockReset().mockResolvedValue({})
    reject.mockReset().mockResolvedValue({})
    remove.mockReset().mockResolvedValue({})
    addRevision.mockReset().mockResolvedValue({})
    create.mockReset().mockResolvedValue({ id: 'a-new' })
    refetch.mockReset()
    vi.mocked(toast.error).mockClear()
    vi.mocked(toast.success).mockClear()
    vi.restoreAllMocks()
  })

  it('shows an honest empty state', () => {
    render(<ResearchArtifactsPanel projectId="p1" />)
    expect(screen.getByText(/还没有组织产物/)).toBeInTheDocument()
  })

  it('renders the current revision and adopts it', async () => {
    state.artifacts = [planArtifact()]
    render(<ResearchArtifactsPanel projectId="p1" />)
    fireEvent.click(screen.getByText('章节计划（1 节）'))
    expect(screen.getByText(/引言/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '采用' }))
    expect(adopt).toHaveBeenCalledWith({ artifactId: 'a1', revision: 2, expectedRevision: 2 })
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('已采用该修订'))
  })

  it('surfaces load failures instead of pretending empty', () => {
    state.error = new Error('offline')
    render(<ResearchArtifactsPanel projectId="p1" />)
    expect(screen.getByRole('alert')).toHaveTextContent('产物加载失败')
    fireEvent.click(screen.getByRole('button', { name: '重试' }))
    expect(refetch).toHaveBeenCalledTimes(1)
  })

  it('shows a Chinese, readable error when adopting fails', async () => {
    state.artifacts = [planArtifact()]
    adopt.mockRejectedValueOnce(backendError('来源版本不存在或已被删除'))
    render(<ResearchArtifactsPanel projectId="p1" />)
    fireEvent.click(screen.getByText('章节计划（1 节）'))
    fireEvent.click(screen.getByRole('button', { name: '采用' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('采用失败', expect.objectContaining({
      description: '来源版本不存在或已被删除',
    })))
    const description = vi.mocked(toast.error).mock.calls[0][1]?.description
    expect(String(description)).not.toContain('FrontendApiError')
    expect(toast.success).not.toHaveBeenCalled()
  })

  it('disables the action and shows progress while adopting', async () => {
    let finish!: (value: unknown) => void
    adopt.mockReturnValueOnce(new Promise((resolve) => { finish = resolve }))
    state.artifacts = [planArtifact()]
    render(<ResearchArtifactsPanel projectId="p1" />)
    fireEvent.click(screen.getByText('章节计划（1 节）'))
    fireEvent.click(screen.getByRole('button', { name: '采用' }))

    const pending = screen.getByRole('button', { name: /采用中/ })
    expect(pending).toBeDisabled()
    expect(screen.getByRole('button', { name: '拒绝' })).toBeDisabled()

    finish({})
    await waitFor(() => expect(screen.getByRole('button', { name: '采用' })).toBeEnabled())
  })

  it('keeps unsaved chapter edits when saving a revision fails', async () => {
    state.artifacts = [planArtifact()]
    addRevision.mockRejectedValueOnce(backendError('产物已在其他位置更新，请基于最新版本重试', 409))
    render(<ResearchArtifactsPanel projectId="p1" />)
    fireEvent.click(screen.getByText('章节计划（1 节）'))
    fireEvent.click(screen.getByRole('button', { name: '编辑计划' }))

    const title = screen.getByLabelText('章节 1 标题')
    fireEvent.change(title, { target: { value: '改过的标题' } })
    fireEvent.click(screen.getByRole('button', { name: '保存新修订' }))

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('保存修订失败', expect.objectContaining({
      description: '产物已在其他位置更新，请基于最新版本重试',
    })))
    expect(screen.getByLabelText('章节 1 标题')).toHaveValue('改过的标题')
    expect(screen.getByRole('button', { name: '收起编辑' })).toBeInTheDocument()
  })

  it('asks for confirmation before deleting an artifact', async () => {
    state.artifacts = [planArtifact()]
    const confirm = vi.spyOn(window, 'confirm').mockReturnValueOnce(false).mockReturnValueOnce(true)
    render(<ResearchArtifactsPanel projectId="p1" />)
    fireEvent.click(screen.getByText('章节计划（1 节）'))

    fireEvent.click(screen.getByRole('button', { name: '删除产物' }))
    expect(remove).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '删除产物' }))
    expect(confirm).toHaveBeenCalledTimes(2)
    await waitFor(() => expect(remove).toHaveBeenCalledWith('a1'))
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith('已删除产物'))
  })
})
