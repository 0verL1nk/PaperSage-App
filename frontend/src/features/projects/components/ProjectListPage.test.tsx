/**
 * ProjectListPage tests
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

// Mock router
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...props }: any) => (
    <a href={to} {...props}>{children}</a>
  ),
}))

// Mock the API hooks
vi.mock('../api', () => ({
  useProjects: vi.fn(),
  useGroups: vi.fn(),
  useDeleteProject: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
  useCreateProject: vi.fn(() => ({
    mutateAsync: vi.fn(),
    isPending: false,
  })),
}))

// Mock lucide icons
vi.mock('lucide-react', () => ({
  FolderOpen: () => <span>FolderOpen</span>,
  Plus: () => <span>Plus</span>,
  Trash2: () => <span>Trash2</span>,
  FileText: () => <span>FileText</span>,
  AlertTriangle: () => <span>AlertTriangle</span>,
  XIcon: () => <span>XIcon</span>,
  ChevronRight: () => <span>ChevronRight</span>,
}))

// Mock ConfirmDialog
vi.mock('@/components/ui/confirm-dialog', () => ({
  ConfirmDialog: () => null,
}))

import { useProjects, useGroups } from '../api'
import { ProjectListPage } from './ProjectListPage'

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

describe('ProjectListPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('should render project list page', () => {
    vi.mocked(useProjects).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as any)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    expect(screen.getByText('My Projects')).toBeInTheDocument()
  })

  it('should display projects when loaded', async () => {
    vi.mocked(useProjects).mockReturnValue({
      data: [
        {
          id: 'proj-1',
          name: 'Test Project 1',
          description: 'Description 1',
          owner_id: 'user-1',
          group_id: null,
          status: 'active',
          is_deleted: false,
          deleted_at: null,
          created_at: '2024-01-01T00:00:00Z',
          updated_at: '2024-01-01T00:00:00Z',
        },
      ],
      isLoading: false,
      error: null,
    } as any)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as any)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    await waitFor(() => {
      expect(screen.getByText('Test Project 1')).toBeInTheDocument()
    })
  })

  it('should show empty state when no projects', async () => {
    vi.mocked(useProjects).mockReturnValue({
      data: [],
      isLoading: false,
      error: null,
    } as any)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as any)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    await waitFor(() => {
      expect(screen.getByText('No projects yet')).toBeInTheDocument()
    })
  })

  it('should show loading state', () => {
    vi.mocked(useProjects).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as any)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as any)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    // Loading state shows skeletons
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })
})
