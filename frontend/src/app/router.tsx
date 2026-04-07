import type { QueryClient } from '@tanstack/react-query'
import {
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from '@tanstack/react-router'
import { AuthPage } from '@/features/auth'
import { MainLayout } from '@/features/layout/MainLayout'
import { WorkspaceLayout } from '@/features/layout/WorkspaceLayout'
import { hasSessionToken } from '@/shared/stores/session-store'

export type AppRouterContext = {
  queryClient: QueryClient
}

// Auth guard function
function requireAuth() {
  if (!hasSessionToken()) {
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
  component: () => <Outlet />,
  beforeLoad: () => {
    // Check session restoration on app load
  },
})

// Index/home route - redirect to documents or auth
const indexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  beforeLoad: () => {
    // Redirect to documents if logged in, otherwise go to auth
    if (hasSessionToken()) {
      throw redirect({ to: '/app/documents' })
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
const authRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/auth',
  beforeLoad: () => {
    if (hasSessionToken()) {
      throw redirect({ to: '/app/documents' })
    }
  },
  component: AuthPage,
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

// /app/documents route
const documentsRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/documents',
  component: function Documents() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Documents</h1>
        <p className="mt-2 text-muted-foreground">Project list placeholder</p>
      </div>
    )
  },
})

// /app/trash route
const trashRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/trash',
  component: function Trash() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Trash</h1>
        <p className="mt-2 text-muted-foreground">Trash placeholder</p>
      </div>
    )
  },
})

// /app/profile route
const profileRoute = createRoute({
  getParentRoute: () => appLayoutRoute,
  path: '/app/profile',
  component: function Profile() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Profile</h1>
        <p className="mt-2 text-muted-foreground">Profile placeholder</p>
      </div>
    )
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

// Project root route
const projectRootRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId',
  component: function ProjectRoot() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Project</h1>
        <p className="mt-2 text-muted-foreground">Select a module from the tabs above</p>
      </div>
    )
  },
})

// Project uploading route
const projectUploadingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/uploading',
  component: function ProjectUploading() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Uploading</h1>
        <p className="mt-2 text-muted-foreground">Uploading module placeholder</p>
      </div>
    )
  },
})

// Project coding route
const projectCodingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/coding',
  component: function ProjectCoding() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Coding</h1>
        <p className="mt-2 text-muted-foreground">Coding module placeholder</p>
      </div>
    )
  },
})

// Project classifying route
const projectClassifyingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/classifying',
  component: function ProjectClassifying() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Classifying</h1>
        <p className="mt-2 text-muted-foreground">Classifying module placeholder</p>
      </div>
    )
  },
})

// Project logic-chain route
const projectLogicChainRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/logic-chain',
  component: function ProjectLogicChain() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Logic Chain</h1>
        <p className="mt-2 text-muted-foreground">Logic chain module placeholder</p>
      </div>
    )
  },
})

// Project generating route
const projectGeneratingRoute = createRoute({
  getParentRoute: () => projectLayoutRoute,
  path: '/project/$projectId/generating',
  component: function ProjectGenerating() {
    return (
      <div className="p-6">
        <h1 className="text-2xl font-bold">Generating</h1>
        <p className="mt-2 text-muted-foreground">Generating module placeholder</p>
      </div>
    )
  },
})

// Build route tree
const routeTree = rootRoute.addChildren([
  indexRoute,
  authRoute,
  appLayoutRoute.addChildren([documentsRoute, trashRoute, profileRoute]),
  projectLayoutRoute.addChildren([
    projectRootRoute,
    projectUploadingRoute,
    projectCodingRoute,
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
    main: ['/app/documents', '/app/trash', '/app/profile'],
    project: [
      '/project/:projectId',
      '/project/:projectId/uploading',
      '/project/:projectId/coding',
      '/project/:projectId/classifying',
      '/project/:projectId/logic-chain',
      '/project/:projectId/generating',
    ],
  } as const
}