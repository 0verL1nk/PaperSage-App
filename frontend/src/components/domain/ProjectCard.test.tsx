import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import type { Project } from '@/types/projects'
import { ProjectCard } from './ProjectCard'

vi.mock('@/services/projects-api', () => ({
  useDeleteProject: () => ({ mutateAsync: vi.fn(), isPending: false }),
}))

const project: Project = {
  id: 'p1',
  name: '测试项目',
  description: null,
  owner_id: 'u1',
  group_id: null,
  status: 'active',
  scenario: 'undergraduate_thesis',
  is_deleted: false,
  deleted_at: null,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  group: null,
}

describe('ProjectCard', () => {
  it('opens the project from a real button and keeps the delete action out of it', () => {
    const onClick = vi.fn()
    render(<ProjectCard project={project} onClick={onClick} />)

    const open = screen.getByRole('button', { name: '测试项目' })
    expect(open.querySelector('button')).toBeNull()
    fireEvent.click(open)
    expect(onClick).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('button', { name: '测试项目 的更多操作' })).toBeInTheDocument()
  })
})
