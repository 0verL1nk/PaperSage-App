import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mutateAsync = vi.fn()
vi.mock('@/services/projects-api', () => ({ useCreateProject: () => ({ mutateAsync, isPending: false }) }))

import { CreateProjectDialog } from './CreateProjectDialog'

describe('CreateProjectDialog scenario selection', () => {
  beforeEach(() => mutateAsync.mockReset().mockResolvedValue({}))

  it('submits the selected research scenario with the project', async () => {
    render(<CreateProjectDialog open onOpenChange={vi.fn()} groups={[]} />)
    fireEvent.change(screen.getByLabelText('项目名称'), { target: { value: '入门论文项目' } })
    fireEvent.change(screen.getByLabelText('论文场景'), { target: { value: 'intro_paper' } })
    fireEvent.click(screen.getByRole('button', { name: '创建项目' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({
      name: '入门论文项目', description: undefined, group_id: undefined, scenario: 'intro_paper',
    }))
  })
})
