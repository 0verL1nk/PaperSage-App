import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { verifyEmail } from '@/services/auth-api'
import { useSessionStore } from '@/store'
import { VerifyEmailPage } from '@/views/auth/verify-email-page'

const navigateMock = vi.fn()
vi.mock('@/services/auth-api', () => ({ verifyEmail: vi.fn() }))
vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return { ...actual, useNavigate: () => navigateMock, useSearch: () => ({ token: 'verify-token' }) }
})

describe('VerifyEmailPage with local guest data', () => {
  beforeEach(() => {
    navigateMock.mockReset()
    vi.mocked(verifyEmail).mockReset()
    useSessionStore.setState({ accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true, tokenType: 'Bearer', restored: true })
  })

  it('keeps browser-local projects available and sends the user to login after verification', async () => {
    vi.mocked(verifyEmail).mockResolvedValue({ verified: true })
    render(<VerifyEmailPage />)

    expect(await screen.findByText(/游客项目仍保存在当前浏览器本机/)).toBeInTheDocument()
    expect(useSessionStore.getState()).toMatchObject({ isGuest: true, guestImportPending: true, accessToken: '' })
    expect(verifyEmail).toHaveBeenCalledOnce()

    fireEvent.click(screen.getByRole('button', { name: '去登录' }))
    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/auth' }))
  })
})
