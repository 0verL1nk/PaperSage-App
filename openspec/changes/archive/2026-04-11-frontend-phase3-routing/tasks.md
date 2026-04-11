## 1. Route Configuration

- [x] 1.1 Create `src/routes/__root.tsx` with root route and global providers
- [x] 1.2 Configure router with route tree structure
- [x] 1.3 Add auth guard using session-store
- [x] 1.4 Define `/app/documents` route
- [x] 1.5 Define `/app/trash` route
- [x] 1.6 Define `/app/profile` route
- [x] 1.7 Define `/project/:projectId` root route
- [x] 1.8 Define `/project/:projectId/uploading` route
- [x] 1.9 Define `/project/:projectId/coding` route
- [x] 1.10 Define `/project/:projectId/classifying` route
- [x] 1.11 Define `/project/:projectId/logic-chain` route
- [x] 1.12 Define `/project/:projectId/generating` route

## 2. Main Layout Component

- [x] 2.1 Create `MainLayout.tsx` component
- [x] 2.2 Implement header with branding
- [x] 2.3 Implement sidebar with navigation links
- [x] 2.4 Implement content area with Outlet
- [x] 2.5 Add basic responsive styles

## 3. Workspace Layout Component

- [x] 3.1 Create `WorkspaceLayout.tsx` component
- [x] 3.2 Extract projectId from route params
- [x] 3.3 Implement back navigation to `/app/documents`
- [x] 3.4 Implement home button to project root
- [x] 3.5 Implement tab navigation for project modules

## 4. Placeholder Pages

- [x] 4.1 Create Documents placeholder page
- [x] 4.2 Create Trash placeholder page
- [x] 4.3 Create Profile placeholder page
- [x] 4.4 Create Classifying placeholder page
- [x] 4.5 Create LogicChain placeholder page
- [x] 4.6 Create Generating placeholder page

## 5. Integration & Verification

- [x] 5.1 Connect routes to layout components
- [x] 5.2 Verify all routes are accessible
- [x] 5.3 Verify unauthenticated users are redirected to `/auth`
- [x] 5.4 Verify auth route redirects authenticated users away
- [x] 5.5 Run `pnpm build` to verify no build errors
- [x] 5.6 Run `pnpm typecheck` to verify no type errors