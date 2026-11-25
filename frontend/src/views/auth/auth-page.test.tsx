import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  loginWithPassword,
  registerWithEmail,
  sendEmailCode,
  verifyEmailCode,
} from '@/services/auth-api'
import { initializeGuestWorkspace } from '@/services/guest-local-workspace'
import { AuthPage } from '@/views/auth/auth-page'
import { setSessionTokens, useSessionStore } from '@/store'

const navigateMock = vi.fn()
const toastSuccessMock = vi.fn()
const toastErrorMock = vi.fn()

vi.mock('@/services/auth-api', () => ({
  loginWithPassword: vi.fn(),
  registerWithEmail: vi.fn(),
  sendEmailCode: vi.fn(),
  verifyEmailCode: vi.fn(),
  getEmailDomains: vi.fn().mockResolvedValue(['hit.edu.cn']),
  resendVerification: vi.fn(),
}))
vi.mock('@/services/guest-local-workspace', () => ({ initializeGuestWorkspace: vi.fn() }))

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
    Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; className?: string }) => (
      <a href={to} {...props}>{children}</a>
    ),
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
    vi.mocked(registerWithEmail).mockReset()
    vi.mocked(sendEmailCode).mockReset()
    vi.mocked(verifyEmailCode).mockReset()
    vi.mocked(initializeGuestWorkspace).mockReset().mockResolvedValue(undefined)
    useSessionStore.setState({
      accessToken: '',
      refreshToken: '',
      restored: true,
    })
  })

  it('defaults to email code login', () => {
    renderAuthPage()

    expect(screen.getByRole('button', { name: '验证码登录' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: '密码登录' })).toHaveAttribute('aria-pressed', 'false')
    expect(screen.getByLabelText('邮箱')).toHaveAttribute('autocomplete', 'email')
    expect(screen.getByLabelText('验证码')).toHaveAttribute('autocomplete', 'one-time-code')
    expect(screen.getByRole('button', { name: '获取验证码' })).toBeVisible()
  })

  it('labels credential fields for browser autofill in password mode', async () => {
    const user = userEvent.setup()
    renderAuthPage()

    await user.click(screen.getByRole('button', { name: '密码登录' }))

    expect(screen.getByLabelText('邮箱或用户名')).toHaveAttribute('autocomplete', 'username')
    expect(screen.getByLabelText('密码')).toHaveAttribute('autocomplete', 'current-password')
  })

  it('sends a code and starts the 60s countdown', async () => {
    const user = userEvent.setup()
    vi.mocked(sendEmailCode).mockResolvedValue({ message: '验证码已发送，请查收邮件。' })
    renderAuthPage()

    await user.type(screen.getByLabelText('邮箱'), 'student@hit.edu.cn')
    await user.click(screen.getByRole('button', { name: '获取验证码' }))

    // TanStack Query v5 会把 mutation context 作为第二个参数传给 mutationFn。
    await waitFor(() => expect(sendEmailCode).toHaveBeenCalledWith('student@hit.edu.cn', expect.anything()))
    const countdown = await screen.findByRole('button', { name: /重新发送（\d+s）/ })
    expect(countdown).toBeDisabled()
  })

  it('verifies the code, stores tokens and navigates to projects', async () => {
    const user = userEvent.setup()
    vi.mocked(verifyEmailCode).mockResolvedValue({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
      isNewUser: false,
    })
    renderAuthPage()

    await user.type(screen.getByLabelText('邮箱'), 'student@hit.edu.cn')
    await user.type(screen.getByLabelText('验证码'), '123456')
    await user.click(screen.getByRole('button', { name: '登录 / 自动注册' }))

    await waitFor(() => {
      expect(verifyEmailCode).toHaveBeenCalledWith('student@hit.edu.cn', '123456')
    })
    expect(useSessionStore.getState().accessToken).toBe('access-token')
    expect(useSessionStore.getState().refreshToken).toBe('refresh-token')
    expect(navigateMock).toHaveBeenCalledWith({ to: '/app/projects' })
    expect(toastSuccessMock).toHaveBeenCalledWith('登录成功', expect.anything())
  })

  it('tells a first-time user that the account was created and how to set a password', async () => {
    const user = userEvent.setup()
    vi.mocked(verifyEmailCode).mockResolvedValue({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
      isNewUser: true,
    })
    renderAuthPage()

    await user.type(screen.getByLabelText('邮箱'), 'new@hit.edu.cn')
    await user.type(screen.getByLabelText('验证码'), '123456')
    await user.click(screen.getByRole('button', { name: '登录 / 自动注册' }))

    await waitFor(() => expect(toastSuccessMock).toHaveBeenCalledWith(
      '账号已创建',
      expect.objectContaining({ description: expect.stringContaining('账号与安全') }),
    ))
  })

  it('clears the stale email error after a successful send', async () => {
    const user = userEvent.setup()
    vi.mocked(sendEmailCode).mockResolvedValue({ message: '验证码已发送，请查收邮件。' })
    renderAuthPage()

    await user.click(screen.getByRole('button', { name: '获取验证码' }))
    expect(await screen.findByText('请输入邮箱')).toBeVisible()

    await user.type(screen.getByLabelText('邮箱'), 'student@hit.edu.cn')
    await user.click(screen.getByRole('button', { name: '获取验证码' }))

    await waitFor(() => expect(sendEmailCode).toHaveBeenCalledWith('student@hit.edu.cn', expect.anything()))
    await waitFor(() => expect(screen.queryByText('请输入邮箱')).not.toBeInTheDocument())
  })

  it('accepts only digits in the verification code field', async () => {
    const user = userEvent.setup()
    renderAuthPage()

    const codeInput = screen.getByLabelText('验证码')
    await user.type(codeInput, '12ab56cd')

    expect(codeInput).toHaveValue('1256')
  })

  it('shows wrong-code errors inside the code form', async () => {
    const user = userEvent.setup()
    vi.mocked(verifyEmailCode).mockRejectedValue(new Error('验证码错误或已过期，请重新获取。'))
    renderAuthPage()

    await user.type(screen.getByLabelText('邮箱'), 'student@hit.edu.cn')
    await user.type(screen.getByLabelText('验证码'), '000000')
    await user.click(screen.getByRole('button', { name: '登录 / 自动注册' }))

    expect(await screen.findByText('验证码错误或已过期，请重新获取。')).toBeVisible()
    expect(useSessionStore.getState().accessToken).toBe('')
    expect(navigateMock).not.toHaveBeenCalled()
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

  it('offers guest mode as a direct, clearly labeled secondary action', async () => {
    renderAuthPage()

    const guestButton = screen.getByRole('button', { name: '以游客身份体验 PaperSage' })
    expect(guestButton).toHaveTextContent('免注册体验 PaperSage')
    expect(screen.getByText(/项目仅保存在此设备/)).toBeInTheDocument()
    await userEvent.click(guestButton)

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/app/projects' }))
    expect(initializeGuestWorkspace).toHaveBeenCalledOnce()
    expect(useSessionStore.getState()).toMatchObject({ isGuest: true, accessToken: '' })
  })

  it('shows the backend selector on the public auth page', () => {
    renderAuthPage()
    expect(screen.getByRole('button', { name: /切换服务器，当前：在线/ })).toBeVisible()
  })

  it('shows registration failures in the active registration panel', async () => {
    const user = userEvent.setup()
    vi.mocked(registerWithEmail).mockRejectedValue(new Error('验证邮件发送失败，请稍后重试；账号已保留，重试注册会重新发送。'))

    renderAuthPage()
    await user.click(screen.getByRole('tab', { name: '注册新账号' }))
    await user.type(screen.getByLabelText('学校邮箱'), 'student@hit.edu.cn')
    await user.type(screen.getByLabelText('设置密码'), 'PaperSage123')
    await user.type(screen.getByLabelText('确认密码'), 'PaperSage123')
    await user.click(screen.getByRole('button', { name: '创建我的账号' }))

    expect(await screen.findByText('验证邮件发送失败，请稍后重试；账号已保留，重试注册会重新发送。')).toBeVisible()
    expect(toastSuccessMock).not.toHaveBeenCalled()
  })

  it('stores tokens after a successful password login', async () => {
    const user = userEvent.setup()

    vi.mocked(loginWithPassword).mockResolvedValue({
      accessToken: 'access-token',
      refreshToken: 'refresh-token',
    })

    renderAuthPage()
    await user.click(screen.getByRole('button', { name: '密码登录' }))

    await user.type(screen.getByLabelText('邮箱或用户名'), 'alice')
    await user.type(screen.getByLabelText('密码'), 'Pass1234')
    await user.click(screen.getByRole('button', { name: '立即登录' }))

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
