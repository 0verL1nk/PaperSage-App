import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  bundle: {
    bundleVersion: 1,
    bundleId: 'b142ab80-8901-48b1-a820-b80f26d9e119',
    exportedAt: '2026-11-23T00:00:00.000Z',
    importMappings: [],
    projects: [{
      project: {
        id: 'b142ab80-8901-48b1-a820-b80f26d9e120',
        name: '游客论文', description: null, owner_id: 'guest-local', group_id: null,
        status: 'active', scenario: 'undergraduate_thesis', is_deleted: false, deleted_at: null,
        created_at: '2026-11-23T00:00:00.000Z', updated_at: '2026-11-23T00:00:00.000Z',
      },
      writingState: null,
      folders: [], documents: [], latexFiles: [],
    }],
  },
}))

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/store', () => ({
  useSessionStore: (selector: (state: { accessToken: string; guestImportPending: boolean }) => unknown) =>
    selector({ accessToken: 'token-1', guestImportPending: true }),
  setGuestImportPending: vi.fn(),
}))
vi.mock('@/services/guest-workspace-import', () => ({
  getGuestImportBundle: vi.fn(async () => mocks.bundle),
  getAccountImportIdentity: vi.fn(async () => ({ accountId: 'alice', username: 'alice' })),
  importGuestWorkspace: vi.fn(),
}))
vi.mock('@/services/guest-local-workspace', () => ({ clearGuestWorkspace: vi.fn() }))
vi.mock('@/components/domain/GuestSessionBadge', () => ({ OPEN_GUEST_IMPORT_EVENT: 'papersage:open-guest-import' }))

import { GuestImportGate } from './GuestImportGate'

function renderWithClient(client: QueryClient) {
  return render(<QueryClientProvider client={client}><GuestImportGate /></QueryClientProvider>)
}

describe('GuestImportGate', () => {
  beforeEach(() => {
    window.sessionStorage.clear()
  })

  it('resets cached queries instead of destroying in-flight ones after login', async () => {
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const resetSpy = vi.spyOn(client, 'resetQueries')
    const removeSpy = vi.spyOn(client, 'removeQueries')

    renderWithClient(client)

    await waitFor(() => expect(resetSpy).toHaveBeenCalled())
    expect(resetSpy.mock.calls.some(([filters]) => JSON.stringify(filters?.queryKey) === '["projects"]')).toBe(true)
    expect(removeSpy).not.toHaveBeenCalled()
  })

  it('auto-opens once per tab session so a reload does not block the page again', async () => {
    const first = renderWithClient(new QueryClient())
    await waitFor(() => expect(screen.getByText('导入本机游客项目')).toBeInTheDocument())
    first.unmount()

    renderWithClient(new QueryClient())
    await waitFor(() => expect(screen.queryByText('导入本机游客项目')).not.toBeInTheDocument())
  })
})
