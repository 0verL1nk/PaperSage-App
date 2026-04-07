export const appRouteManifest = {
  public: ['/', '/auth'],
  main: ['/app/documents', '/app/trash', '/app/profile'],
  project: [
    '/project/:projectId/uploading',
    '/project/:projectId/coding',
    '/project/:projectId/classifying',
    '/project/:projectId/logic-chain',
    '/project/:projectId/generating',
  ],
} as const
