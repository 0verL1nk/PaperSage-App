import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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

  it('activates durable browser-local guest mode without calling auth APIs', async () => {
    render(<GuestWorkspacePage />)
    fireEvent.click(screen.getByRole('button', { name: /开始游客体验/ }))

    await waitFor(() => expect(navigateMock).toHaveBeenCalledWith({ to: '/app/projects' }))
    expect(useSessionStore.getState()).toMatchObject({ accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true })
    expect(window.localStorage.getItem('papersage_local_guest_mode')).toBe('true')
    expect(window.localStorage.getItem('papersage_access_token')).toBeNull()
    expect(initializeGuestWorkspace).toHaveBeenCalledOnce()
    expect(screen.getByText(/不会上传到云端/)).toBeInTheDocument()
  })

  it('warns that clearing browser storage can lose local work', () => {
    render(<GuestWorkspacePage />)
    expect(screen.getByText(/清除浏览器站点数据、卸载浏览器或更换设备可能导致游客内容无法恢复/)).toBeInTheDocument()
  })

  it('prevents guest mode when IndexedDB is unavailable', async () => {
    vi.mocked(initializeGuestWorkspace).mockRejectedValue(new Error('此浏览器不支持本地游客工作区（IndexedDB）。'))
    render(<GuestWorkspacePage />)
    fireEvent.click(screen.getByRole('button', { name: /开始游客体验/ }))
    expect(await screen.findByText(/此浏览器不支持本地游客工作区/)).toBeInTheDocument()
    expect(initializeGuestWorkspace).toHaveBeenCalledOnce()
    expect(useSessionStore.getState().isGuest).toBe(false)
    expect(navigateMock).not.toHaveBeenCalled()
  })
})
