import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { confirmPasswordReset } from '@/services/auth-api'
import { ResetPasswordPage } from '@/views/auth/reset-password-page'

vi.mock('@/services/auth-api', () => ({ confirmPasswordReset: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; className?: string }) => <a href={to} {...props}>{children}</a>,
}))

describe('ResetPasswordPage', () => {
  beforeEach(() => {
    vi.mocked(confirmPasswordReset).mockReset()
    window.history.replaceState({}, '', '/')
  })

  it('removes the token from the URL and requires matching passwords before reset', async () => {
    window.history.replaceState({}, '', '/auth/reset-password#token=opaque-token')
    vi.mocked(confirmPasswordReset).mockResolvedValue({ message: 'done' })
    render(<ResetPasswordPage />)
    expect(window.location.hash).toBe('')

    fireEvent.change(screen.getByLabelText('新密码'), { target: { value: 'NewSecret123' } })
    fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'Different123' } })
    fireEvent.click(screen.getByRole('button', { name: '更新密码' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('两次输入的密码不一致')
    expect(confirmPasswordReset).not.toHaveBeenCalled()

    fireEvent.change(screen.getByLabelText('确认新密码'), { target: { value: 'NewSecret123' } })
    fireEvent.click(screen.getByRole('button', { name: '更新密码' }))
    await waitFor(() => expect(confirmPasswordReset).toHaveBeenCalledWith('opaque-token', 'NewSecret123'))
    expect(await screen.findByRole('heading', { name: '密码已更新' })).toBeInTheDocument()
    expect(screen.getByText(/其他已登录会话也已失效/)).toBeInTheDocument()
  })

  it('does not allow submission without a reset token', () => {
    render(<ResetPasswordPage />)
    expect(screen.getByRole('button', { name: '更新密码' })).toBeDisabled()
    expect(screen.getByText(/链接无效或缺少凭据/)).toBeInTheDocument()
  })
})
