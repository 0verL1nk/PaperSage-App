import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { initializeGuestWorkspace } from '@/services/guest-local-workspace'
import { useSessionStore } from '@/store'
import { GuestWorkspacePage } from '@/views/auth/guest-workspace-page'

const navigateMock = vi.fn()
vi.mock('@/services/guest-local-workspace', () => ({ initializeGuestWorkspace: vi.fn() }))
vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>('@tanstack/react-router')
  return {
    ...actual,
    useNavigate: () => navigateMock,
    Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; className?: string }) => <a href={to} {...props}>{children}</a>,
  }
})

describe('GuestWorkspacePage', () => {
  beforeEach(() => {
    navigateMock.mockReset()
    window.history.replaceState({}, '', '/guest')
    window.localStorage.clear()
    useSessionStore.setState({ accessToken: '', refreshToken: '', isGuest: false, guestImportPending: false, tokenType: 'Bearer', restored: true })
    vi.mocked(initializeGuestWorkspace).mockReset().mockResolvedValue(undefined)
  })

  it('activates guest mode immediately on a direct /guest visit', async () => {
    render(<GuestWorkspacePage />)

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/app/projects' }))
    expect(useSessionStore.getState()).toMatchObject({ accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true })
    expect(window.localStorage.getItem('papersage_local_guest_mode')).toBe('true')
    expect(window.localStorage.getItem('papersage_access_token')).toBeNull()
    expect(initializeGuestWorkspace).toHaveBeenCalledOnce()
    expect(screen.queryByRole('button', { name: '开始游客体验' })).not.toBeInTheDocument()
  })

  it('prevents guest mode when IndexedDB is unavailable', async () => {
    vi.mocked(initializeGuestWorkspace).mockRejectedValue(new Error('此浏览器不支持本地游客工作区（IndexedDB）。'))
    render(<GuestWorkspacePage />)
    expect(await screen.findByText(/此浏览器不支持本地游客工作区/)).toBeInTheDocument()
    expect(initializeGuestWorkspace).toHaveBeenCalledOnce()
    expect(useSessionStore.getState().isGuest).toBe(false)
    expect(navigateMock).not.toHaveBeenCalled()
    expect(screen.getByRole('button', { name: '重试' })).toBeInTheDocument()
  })
})
