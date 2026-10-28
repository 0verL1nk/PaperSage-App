import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getHealth } = vi.hoisted(() => ({ getHealth: vi.fn() }))

vi.mock('@/services/client', () => ({
  apiClient: { get: getHealth },
}))

import { frontendBuildInfo } from '@/lib/build-info'
import { VersionInfo } from './VersionInfo'

function renderVersionInfo() {
  const queryClient = new QueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <VersionInfo />
    </QueryClientProvider>,
  )
}

describe('VersionInfo', () => {
  beforeEach(() => {
    getHealth.mockReset()
  })

  it('shows frontend build metadata and backend health metadata', async () => {
    getHealth.mockResolvedValue({
      status: 200,
      data: {
        status: 'ok',
        version: 'v2.4.0',
        commit: 'backend123',
        buildTime: '2026-10-28T12:00:00Z',
        goVersion: 'go1.25.0',
      },
    })

    renderVersionInfo()

    expect(screen.getByText('版本信息')).toBeInTheDocument()
    expect(screen.getByText(frontendBuildInfo.version)).toBeInTheDocument()
    expect(screen.getByText(frontendBuildInfo.commit)).toBeInTheDocument()
    expect(screen.getByText(frontendBuildInfo.buildTime)).toBeInTheDocument()

    await waitFor(() => {
      expect(screen.getByText('v2.4.0')).toBeInTheDocument()
    })
    expect(screen.getByText('backend123')).toBeInTheDocument()
    expect(screen.getByText('2026-10-28T12:00:00Z')).toBeInTheDocument()
    expect(screen.getByText('go1.25.0')).toBeInTheDocument()
    expect(getHealth).toHaveBeenCalledOnce()
    expect(getHealth).toHaveBeenCalledWith('/health', expect.objectContaining({
      skipGlobalLoading: true,
      skipAuthRefresh: true,
    }))
  })

  it('keeps frontend metadata visible when the backend health endpoint is unavailable', async () => {
    getHealth.mockRejectedValue(new Error('offline'))

    renderVersionInfo()

    expect(screen.getByText(frontendBuildInfo.version)).toBeInTheDocument()
    expect(await screen.findByRole('status')).toHaveTextContent('后端版本暂不可用')
    expect(getHealth).toHaveBeenCalledOnce()
  })
})
