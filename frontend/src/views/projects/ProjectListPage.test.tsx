/**
 * ProjectListPage tests
 */
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'

// Mock router
const mockNavigate = vi.fn()
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to, ...props }: { children: ReactNode; to: string; [key: string]: unknown }) => (
    <a href={to} {...props}>{children}</a>
  ),
  useNavigate: () => mockNavigate,
}))

// Mock the API hooks
vi.mock('@/services/projects-api', () => ({
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
  FolderGit2: () => <span>FolderGit2</span>,
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

import { useProjects, useGroups } from '@/services/projects-api'
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
    } as never)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as never)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    expect(screen.getByText('所有项目')).toBeInTheDocument()
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
    } as never)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as never)

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
    } as never)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as never)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    await waitFor(() => {
      expect(screen.getByText('暂无项目')).toBeInTheDocument()
    })
  })

  it('should show loading state', () => {
    vi.mocked(useProjects).mockReturnValue({
      data: undefined,
      isLoading: true,
      error: null,
    } as never)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as never)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    // Loading state shows skeletons
    expect(document.querySelector('.animate-pulse')).toBeInTheDocument()
  })

  it('should navigate to project when card is clicked', async () => {
    const projectId = 'proj-123'
    vi.mocked(useProjects).mockReturnValue({
      data: [
        {
          id: projectId,
          name: 'Test Project',
          description: 'Test Description',
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
    } as never)
    vi.mocked(useGroups).mockReturnValue({
      data: [],
      isLoading: false,
    } as never)

    render(
      <TestWrapper>
        <ProjectListPage />
      </TestWrapper>
    )

    await waitFor(() => {
      expect(screen.getByText('Test Project')).toBeInTheDocument()
    })

    // Click on the project card
    const card = screen.getByRole('button', { name: /test project/i })
    card.click()

    expect(mockNavigate).toHaveBeenCalledWith({
      to: '/project/$projectId',
      params: { projectId },
    })
  })
})
