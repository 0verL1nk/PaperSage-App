/**
 * TrashPage tests
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

// Mock router
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
}))

// Mock the API hooks
vi.mock('@/services/projects-api', () => ({
  useTrash: vi.fn(),
  useRestoreProject: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
  usePermanentDeleteProject: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
}))

// Mock lucide icons
vi.mock('lucide-react', () => ({
  Trash2: () => <span>Trash2</span>,
  RotateCcw: () => <span>RotateCcw</span>,
  AlertTriangle: () => <span>AlertTriangle</span>,
  AlertCircle: () => <span>AlertCircle</span>,
  ArrowLeft: () => <span>ArrowLeft</span>,
  FolderGit2: () => <span>FolderGit2</span>,
  XIcon: () => <span>XIcon</span>,
}))

// Mock ConfirmDialog
vi.mock('@/components/ui/confirm-dialog', () => ({
  ConfirmDialog: () => null,
}))

import { useTrash } from '@/services/projects-api'
import { TrashPage } from './TrashPage'

// Test wrapper
function TestWrapper({ children }: { children: ReactNode }) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  return (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
}

describe('TrashPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should render trash page', () => {
    vi.mocked(useTrash).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as never)

    render(
      <TestWrapper>
        <TrashPage />
      </TestWrapper>
    )

    expect(screen.getByText('回收站')).toBeInTheDocument()
  })

  it('should display deleted items when loaded', async () => {
    vi.mocked(useTrash).mockReturnValue({
      data: [
        {
          type: 'project',
          id: 'trash-1',
          name: 'Deleted Project 1',
          deleted_at: '2024-01-01T00:00:00Z',
        },
      ],
      isLoading: false,
      error: null,
    } as never)

    render(
      <TestWrapper>
        <TrashPage />
      </TestWrapper>
    )

    await waitFor(() => {
      expect(screen.getByText('Deleted Project 1')).toBeInTheDocument()
    })
  })

  it('should show empty state when no items', async () => {
    vi.mocked(useTrash).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as never)

    render(
      <TestWrapper>
        <TrashPage />
      </TestWrapper>
    )

    await waitFor(() => {
      expect(screen.getByText('回收站是空的')).toBeInTheDocument()
    })
  })
})
