import { VerifyEmailPage } from '@/views/auth/verify-email-page'
import { AdminPage } from '@/views/admin/AdminPage'
import type { QueryClient } from '@tanstack/react-query'
import {
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { AuthPage } from '@/views/auth/auth-page'
import { GuestWorkspacePage } from '@/views/auth/guest-workspace-page'
import { GuestImportGate } from '@/components/domain/GuestImportGate'
import { MainLayout } from '@/views/layout/MainLayout'
import { WorkspaceLayout } from '@/views/layout/WorkspaceLayout'
import { hasSessionToken, hasWorkspaceSession } from '@/store/session-store'
import { ProjectListPage } from '@/views/projects/ProjectListPage'
import { WritingPage } from '@/views/projects/WritingPage'
import { PreparePage } from '@/views/projects/PreparePage'
import { ReviewPage } from '@/views/projects/ReviewPage'
import { UploadingPage } from '@/views/uploading/UploadingPage'
import { TrashPage } from '@/views/trash/TrashPage'
import { UnderConstruction } from '@/components/shared/UnderConstruction'
import { SettingsPage } from '@/views/settings/SettingsPage'

export type AppRouterContext = {
  queryClient: QueryClient
}

// Auth guard function
function requireAuth() {
  if (!hasWorkspaceSession()) {
    throw redirect({
      to: '/auth',
      search: {
        redirect: window.location.pathname,
      },
    })
  }
}

// Root route - entry point
const rootRoute = createRootRouteWithContext<AppRouterContext>()({
  component: () => <><Outlet /><GuestImportGate /></>,
  beforeLoad: () => {
    // Check session restoration on app load
  },
})

// Index/home route - redirect to projects or auth
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    // Redirect to projects if logged in, otherwise go to auth
    if (hasWorkspaceSession()) {
      throw redirect({ to: '/app/projects' })
    } else {
      throw redirect({ to: '/auth' })
    }
  },
  component: function Index() {
    return (
      <MainLayout>
        <div className="flex h-full items-center justify-center">
          <div className="text-center">
            <h1 className="text-2xl font-bold">Welcome to PaperSage</h1>
            <p className="mt-2 text-muted-foreground">Please login to continue</p>
          </div>
        </div>
      </MainLayout>
    )
  },
})

// Auth route - public, redirects if already logged in
const adminRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/admin',
  component: AdminPage,
})

const authVerifyRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/auth/verify',
  component: VerifyEmailPage,
})

const authRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/auth',
  beforeLoad: () => {
    if (hasSessionToken()) {
      throw redirect({ to: '/app/projects' })
    }
  },
  component: AuthPage,
})

// Public entry that activates the browser-local guest workspace.
const guestRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/guest',
  component: GuestWorkspacePage,
})

// App layout wrapper for protected routes
const appLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'app-layout',
  component: function AppLayout() {
    return (
      <MainLayout>
        <Outlet />
      </MainLayout>
    )
  },
  beforeLoad: () => {
    requireAuth()
  },
})

// /app/documents route - under construction
const documentsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/documents',
  component: function Documents() {
    return (
      <UnderConstruction
        title="Documents"
        description="Reference management coming soon"
      />
    )
  },
})

// /app/projects route
const projectsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/projects',
  component: function Projects() {
    return <ProjectListPage />
  },
})

// /app/trash route
const trashRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/trash',
  component: function Trash() {
    return <TrashPage />
  },
})

// /app/settings route
const settingsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/settings',
  component: SettingsPage,
})

// /app/profile redirect to settings
const profileRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/profile',
  beforeLoad: () => {
    throw redirect({ to: '/app/settings' })
  },
})

// Project layout wrapper
const projectLayoutRoute = createRoute({
  getParentRoute: () => rootRoute,
  id: 'project-layout',
  component: function ProjectLayout() {
    return (
      <WorkspaceLayout>
        <Outlet />
      </WorkspaceLayout>
    )
  },
  beforeLoad: () => {
    requireAuth()
  },
})

// Project root route — start with research preparation.
const projectRootRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId',
  beforeLoad: ({ params }) => {
    throw redirect({ to: '/project/$projectId/prepare', params })
  },
})

// Project uploading route
const projectUploadingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/uploading',
  component: UploadingPage,
})

// Project writing route
const projectWritingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/writing',
  component: WritingPage,
})

const projectPrepareRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/prepare',
  component: PreparePage,
})

const projectReviewRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/review',
  component: ReviewPage,
})

// Redirect old planned URLs into the current three-stage workflow.
const projectClassifyingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/classifying',
  beforeLoad: ({ params }) => { throw redirect({ to: '/project/$projectId/prepare', params }) },
})

const projectLogicChainRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/logic-chain',
  beforeLoad: ({ params }) => { throw redirect({ to: '/project/$projectId/prepare', params }) },
})

const projectGeneratingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/generating',
  beforeLoad: ({ params }) => { throw redirect({ to: '/project/$projectId/prepare', params }) },
})

// Build route tree
const routeTree = rootRoute.addChildren([
  indexRoute,
  authVerifyRoute,
  authRoute,
  guestRoute,
  appLayoutRoute.addChildren([documentsRoute, projectsRoute, trashRoute, settingsRoute, profileRoute, adminRoute]),
  projectLayoutRoute.addChildren([
    projectRootRoute,
    projectPrepareRoute,
    projectUploadingRoute,
    projectWritingRoute,
    projectReviewRoute,
    projectClassifyingRoute,
    projectLogicChainRoute,
    projectGeneratingRoute,
  ]),
])

export const router = createRouter({
  routeTree,
  context: {
    queryClient: undefined as never,
  },
  defaultPreload: 'intent',
  // Global not found handler
  notFoundMode: 'root',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

export function AppRouterProvider({ queryClient }: AppRouterContext) {
  return <RouterProvider router={router} context={{ queryClient }} />
}

export function getPlannedRouteGroups() {
  return {
    public: ['/', '/auth'],
    main: ['/app/documents', '/app/trash', '/app/profile', '/app/settings'],
    project: [
      '/project/:projectId',
      '/project/:projectId/prepare',
      '/project/:projectId/uploading',
      '/project/:projectId/writing',
      '/project/:projectId/review',
    ],
  } as const
}
