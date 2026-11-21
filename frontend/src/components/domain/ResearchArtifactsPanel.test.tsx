import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ResearchArtifactsPanel } from './ResearchArtifactsPanel'
import type { ResearchArtifact } from '@/types/research'

const adopt = vi.fn().mockResolvedValue({})
const reject = vi.fn().mockResolvedValue({})
const remove = vi.fn().mockResolvedValue({})
const addRevision = vi.fn().mockResolvedValue({})
const create = vi.fn().mockResolvedValue({ id: 'a-new' })

const state: { artifacts: ResearchArtifact[]; isLoading: boolean; error: Error | null } = {
  artifacts: [], isLoading: false, error: null,
}

vi.mock('@/services/research-api', () => ({
  useResearchArtifacts: () => ({ data: state.artifacts, isLoading: state.isLoading, error: state.error }),
  useCreateResearchArtifact: () => ({ mutateAsync: create, isPending: false }),
  useAddResearchRevision: () => ({ mutateAsync: addRevision, isPending: false }),
  useAdoptResearchRevision: () => ({ mutateAsync: adopt, isPending: false }),
  useRejectResearchRevision: () => ({ mutateAsync: reject, isPending: false }),
  useDeleteResearchArtifact: () => ({ mutateAsync: remove, isPending: false }),
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

describe('ResearchArtifactsPanel', () => {
  beforeEach(() => {
    state.artifacts = []
    state.isLoading = false
    state.error = null
    vi.clearAllMocks()
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
  })

  it('surfaces load failures instead of pretending empty', () => {
    state.error = new Error('offline')
    render(<ResearchArtifactsPanel projectId="p1" />)
    expect(screen.getByRole('alert')).toHaveTextContent('产物加载失败')
  })
})
