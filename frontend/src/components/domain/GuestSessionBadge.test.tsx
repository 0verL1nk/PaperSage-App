import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { useSessionStore } from '@/store'
import { GuestSessionBadge, OPEN_GUEST_IMPORT_EVENT } from '@/components/domain/GuestSessionBadge'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children, ...props }: { to: string; children: React.ReactNode; className?: string }) => <a href={to} {...props}>{children}</a>,
}))

describe('GuestSessionBadge', () => {
  beforeEach(() => {
    useSessionStore.setState({ accessToken: '', refreshToken: '', isGuest: true, guestImportPending: true, tokenType: 'Bearer', restored: true })
  })

  it('labels local-only content and links to account login/import', () => {
    render(<GuestSessionBadge />)
    expect(screen.getByText('本机游客 · 未同步')).toBeInTheDocument()
    expect(screen.getByText(/清除站点数据可能无法恢复/)).toBeInTheDocument()
    const importLinks = screen.getAllByRole('link', { name: /登录并导入/ })
    expect(importLinks.length).toBeGreaterThan(0)
    for (const link of importLinks) expect(link).toHaveAttribute('href', '/auth')
    expect(screen.queryByText(/到期|额度|云端会话/)).not.toBeInTheDocument()
  })

  it('offers a review action after account login while retaining pending status', () => {
    useSessionStore.setState({ accessToken: 'account-token', refreshToken: 'refresh', isGuest: false, guestImportPending: true, tokenType: 'Bearer', restored: true })
    const eventSpy = vi.spyOn(window, 'dispatchEvent')
    render(<GuestSessionBadge />)
    fireEvent.click(screen.getByRole('button', { name: /检查并导入/ }))
    expect(eventSpy).toHaveBeenCalledWith(expect.objectContaining({ type: OPEN_GUEST_IMPORT_EVENT }))
    expect(useSessionStore.getState().guestImportPending).toBe(true)
  })

  it('renders nothing when there is no active guest workspace or import to review', () => {
    useSessionStore.setState({ accessToken: 'account-token', refreshToken: 'refresh', isGuest: false, guestImportPending: false, tokenType: 'Bearer', restored: true })
    const { container } = render(<GuestSessionBadge />)
    expect(container).toBeEmptyDOMElement()
  })
})
