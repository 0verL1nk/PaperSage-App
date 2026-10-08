import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mutateAsync = vi.fn()
vi.mock('@/services/projects-api', () => ({ useCreateProject: () => ({ mutateAsync, isPending: false }) }))

import { CreateProjectDialog } from './CreateProjectDialog'

describe('CreateProjectDialog scenario selection', () => {
  beforeEach(() => mutateAsync.mockReset().mockResolvedValue({}))

  it('submits the selected research scenario with the project', async () => {
    const user = userEvent.setup()
    render(<CreateProjectDialog open onOpenChange={vi.fn()} groups={[]} />)
    fireEvent.change(screen.getByLabelText('项目名称'), { target: { value: '入门论文项目' } })
    await user.click(screen.getByRole('combobox', { name: '论文场景' }))
    await user.click(screen.getByRole('option', { name: '入门级学术小论文' }))
    await user.click(screen.getByRole('button', { name: '创建项目' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({
      name: '入门论文项目', description: undefined, group_id: undefined, scenario: 'intro_paper',
    }))
  })
})
