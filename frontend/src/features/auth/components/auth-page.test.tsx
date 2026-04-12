import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { loginWithPassword, registerWithPassword } from '@/features/auth/api'
import { AuthPage } from '@/features/auth/components/auth-page'
import { setSessionTokens, useSessionStore } from '@/shared/stores'

const navigateMock = vi.fn()
const toastSuccessMock = vi.fn()
const toastErrorMock = vi.fn()

vi.mock('@/features/auth/api', () => ({
  loginWithPassword: vi.fn(),
  registerWithPassword: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccessMock(...args),
    error: (...args: unknown[]) => toastErrorMock(...args),
  },
}))

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')

  return {
    ...actual,
    useNavigate: () => navigateMock,
  }
})

function renderAuthPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <AuthPage />
    </QueryClientProvider>,
  )
}

describe('AuthPage', () => {
  beforeEach(() => {
    navigateMock.mockReset()
    toastSuccessMock.mockReset()
    toastErrorMock.mockReset()
    vi.mocked(loginWithPassword).mockReset()
    vi.mocked(registerWithPassword).mockReset()
    useSessionStore.setState({
      accessToken: '',
      refreshToken: '',
      restored: true,
    })
  })

  it('redirects away from /auth when a session already exists', async () => {
    setSessionTokens({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
    })

    renderAuthPage()

    await waitFor(() => {
      expect(navigateMock).toHaveBeenCalledWith({
        to: '/app/projects',
      })
    })
  })

  it('validates password confirmation in register mode', async () => {
    const user = userEvent.setup()

    renderAuthPage()

    await user.click(screen.getByRole('tab', { name: '注册' }))
    await user.type(screen.getByLabelText('用户名'), 'alice')
    await user.type(screen.getByLabelText('密码'), 'Pass1234')
    await user.type(screen.getByLabelText('确认密码'), 'Pass4321')
    await user.click(screen.getByRole('button', { name: '创建账号' }))

    expect(await screen.findByText('两次输入的密码不一致')).toBeInTheDocument()
    expect(registerWithPassword).not.toHaveBeenCalled()
  })

  it('stores long and short tokens after a successful login', async () => {
    const user = userEvent.setup()

    vi.mocked(loginWithPassword).mockResolvedValue({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
    })

    renderAuthPage()

    await user.type(screen.getByLabelText('用户名'), 'alice')
    await user.type(screen.getByLabelText('密码'), 'Pass1234')
    await user.click(screen.getByRole('button', { name: '登录' }))

    await waitFor(() => {
      expect(vi.mocked(loginWithPassword).mock.calls[0]?.[0]).toEqual({
        username: 'alice',
        password: 'Pass1234',
      })
    })

    expect(useSessionStore.getState().accessToken).toBe('access-token')
    expect(useSessionStore.getState().refreshToken).toBe('refresh-token')
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/app/projects',
    })
  })
})
