import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ReactNode } from 'react'

import { WorkspaceLayout } from './WorkspaceLayout'
import { useSessionStore } from '@/store'
import { FrontendApiError } from '@/services/errors'

const projectResult: { data: unknown; isLoading: boolean; isError: boolean; error: unknown } = {
  data: undefined,
  isLoading: false,
  isError: true,
  error: new FrontendApiError('项目不存在或已被删除', { kind: 'http', status: 404 }),
}
const getGuestProject = vi.fn()

function wrap(ui: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={client}>{ui}</QueryClientProvider>
}

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({ projectId: 'local-1' }),
  useLocation: () => ({ pathname: '/project/local-1/prepare' }),
  Link: ({ children, to, ...props }: { children: ReactNode; to: string; [key: string]: unknown }) => <a href={to} {...props}>{children}</a>,
}))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))
vi.mock('@/services/projects-api', () => ({ useProject: () => projectResult }))
vi.mock('@/services/guest-local-workspace', () => ({ getGuestProject: (...args: unknown[]) => getGuestProject(...args) }))
vi.mock('@/components/desktop/desktop-titlebar', () => ({ DesktopAppMenu: () => null, WindowControls: () => null }))
vi.mock('@/components/domain/GuestSessionBadge', () => ({
  GuestModeChip: () => null,
  GuestSessionBadge: () => null,
  OPEN_GUEST_IMPORT_EVENT: 'papersage:open-guest-import',
}))

describe('WorkspaceLayout 未导入的游客项目', () => {
  beforeEach(() => {
    getGuestProject.mockReset()
    useSessionStore.setState({
      accessToken: 'token', refreshToken: 'refresh', isGuest: false,
      guestImportPending: false, tokenType: 'Bearer', restored: true,
    })
  })

  it('服务端 404 但本机有该项目时给出导入入口，而不是「项目不存在」', async () => {
    getGuestProject.mockResolvedValue({ id: 'local-1', name: '我的本机论文' })
    const eventSpy = vi.spyOn(window, 'dispatchEvent')
    render(wrap(<WorkspaceLayout><div>页面内容</div></WorkspaceLayout>))

    expect(await screen.findByText('这个项目还在这台设备上')).toBeInTheDocument()
    expect(screen.getByText('我的本机论文')).toBeInTheDocument()
    expect(screen.queryByText('页面内容')).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '导入到账号' }))
    expect(eventSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'papersage:open-guest-import' }))
  })

  it('本机没有该项目时照常渲染页面自己的错误态', () => {
    getGuestProject.mockRejectedValue(new Error('not found'))
    render(wrap(<WorkspaceLayout><div>页面内容</div></WorkspaceLayout>))

    expect(screen.getByText('页面内容')).toBeInTheDocument()
    expect(screen.queryByText('这个项目还在这台设备上')).not.toBeInTheDocument()
  })
})
