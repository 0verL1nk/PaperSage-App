import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { AdoptedResearchSummary } from './AdoptedResearchSummary'
import type { ResearchArtifact } from '@/types/research'

const state: { artifacts: ResearchArtifact[] } = { artifacts: [] }

vi.mock('@/services/research-api', () => ({
  useResearchArtifacts: () => ({ data: state.artifacts }),
}))

function artifact(adopted: boolean): ResearchArtifact {
  return {
    id: adopted ? 'a1' : 'a2', projectId: 'p1', kind: 'chapter_plan', state: 'active',
    currentRevision: adopted ? 1 : 2, adoptedRevision: adopted ? 1 : null, createdAt: '', updatedAt: '',
    current: { id: 'r', revision: adopted ? 1 : 2, status: adopted ? 'adopted' : 'candidate', origin: 'agent', createdAt: '', payload: { sections: [{ id: 's1', title: adopted ? '已采用引言' : '候选引言', responsibility: '职责', order: 1 }] } },
    adopted: adopted ? { id: 'r', revision: 1, status: 'adopted', origin: 'agent', createdAt: '', payload: { sections: [{ id: 's1', title: '已采用引言', responsibility: '职责', order: 1 }] } } : undefined,
  }
}

describe('AdoptedResearchSummary', () => {
  it('renders nothing without adopted artifacts', () => {
    state.artifacts = [artifact(false)]
    const { container } = render(<AdoptedResearchSummary projectId="p1" />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the adopted plan, not the newer candidate', () => {
    state.artifacts = [artifact(true)]
    render(<AdoptedResearchSummary projectId="p1" />)
    expect(screen.getByText(/已采用的写作计划/)).toBeInTheDocument()
    expect(screen.getByText(/已采用引言/)).toBeInTheDocument()
    expect(screen.queryByText(/候选引言/)).not.toBeInTheDocument()
  })
})
