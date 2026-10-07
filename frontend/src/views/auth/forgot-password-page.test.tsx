import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { requestPasswordReset } from '@/services/auth-api'
import { ForgotPasswordPage } from '@/views/auth/forgot-password-page'

vi.mock('@/services/auth-api', () => ({ requestPasswordReset: vi.fn() }))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; className?: string }) => <a href={to} {...props}>{children}</a>,
}))

describe('ForgotPasswordPage', () => {
  beforeEach(() => vi.mocked(requestPasswordReset).mockReset())

  it('shows the same non-enumerating confirmation after submitting an email', async () => {
    vi.mocked(requestPasswordReset).mockResolvedValue({ message: 'generic' })
    render(<ForgotPasswordPage />)
    fireEvent.change(screen.getByLabelText('邮箱地址'), { target: { value: 'alice@example.com' } })
    fireEvent.click(screen.getByRole('button', { name: '发送重置链接' }))

    await waitFor(() => expect(requestPasswordReset).toHaveBeenCalledWith('alice@example.com'))
    expect(await screen.findByRole('status')).toHaveTextContent('如果该邮箱已验证并关联 PaperSage 账号，我们会发送密码重置链接。')
    expect(screen.getByRole('link', { name: '返回登录' })).toHaveAttribute('href', '/auth')
  })
})
