import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { getCurrentUser } from '@/services/auth-api'
import { useSetPasswordMutation } from '@/hooks/auth-hooks'
import { AccountSettings } from './AccountSettings'
import '@/i18n'

const { mutateAsync } = vi.hoisted(() => ({ mutateAsync: vi.fn() }))

vi.mock('@/services/auth-api', () => ({
  getCurrentUser: vi.fn(),
}))
vi.mock('@/hooks/auth-hooks', () => ({
  useSetPasswordMutation: vi.fn(),
}))
vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

function renderAccountSettings() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <AccountSettings />
    </QueryClientProvider>,
  )
}

describe('AccountSettings', () => {
  beforeEach(() => {
    vi.mocked(getCurrentUser).mockReset()
    mutateAsync.mockReset().mockResolvedValue({ hasPassword: true })
    vi.mocked(useSetPasswordMutation).mockReturnValue({
      mutateAsync,
      isPending: false,
    } as unknown as ReturnType<typeof useSetPasswordMutation>)
  })

  it('lets a code-registered user set a password without the old one', async () => {
    const user = userEvent.setup()
    vi.mocked(getCurrentUser).mockResolvedValue({
      username: 's001',
      email: 's001@example.edu.cn',
      emailVerified: true,
      hasPassword: false,
      isGuest: false,
      isSuperuser: false,
    })

    renderAccountSettings()

    expect(await screen.findByText('设置登录密码')).toBeVisible()
    expect(screen.queryByLabelText('当前密码')).not.toBeInTheDocument()

    await user.type(screen.getByLabelText('新密码'), 'NewPass123')
    await user.type(screen.getByLabelText('确认新密码'), 'NewPass123')
    await user.click(screen.getByRole('button', { name: '设置密码' }))

    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({
      currentPassword: undefined,
      newPassword: 'NewPass123',
    }))
  })

  it('requires the current password when one is already set', async () => {
    const user = userEvent.setup()
    vi.mocked(getCurrentUser).mockResolvedValue({
      username: 'alice',
      email: 'alice@example.edu.cn',
      emailVerified: true,
      hasPassword: true,
      isGuest: false,
      isSuperuser: false,
    })

    renderAccountSettings()

    expect(await screen.findByText('修改密码')).toBeVisible()
    await user.type(screen.getByLabelText('新密码'), 'NewPass123')
    await user.type(screen.getByLabelText('确认新密码'), 'NewPass123')
    await user.click(screen.getByRole('button', { name: '保存新密码' }))

    expect(await screen.findByText('请输入当前密码')).toBeVisible()
    expect(mutateAsync).not.toHaveBeenCalled()

    await user.type(screen.getByLabelText('当前密码'), 'OldPass123')
    await user.click(screen.getByRole('button', { name: '保存新密码' }))
    await waitFor(() => expect(mutateAsync).toHaveBeenCalledWith({
      currentPassword: 'OldPass123',
      newPassword: 'NewPass123',
    }))
  })

  it('shows an actionable error state when the account cannot be loaded', async () => {
    vi.mocked(getCurrentUser).mockRejectedValue(new Error('网络连接失败，请检查网络后重试。'))

    renderAccountSettings()

    expect(await screen.findByText(/账号信息加载失败/)).toBeVisible()
    expect(screen.getByRole('button', { name: '重新加载' })).toBeVisible()
  })
})
