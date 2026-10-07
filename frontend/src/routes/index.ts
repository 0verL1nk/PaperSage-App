export const appRouteManifest = {
  public: ['/', '/auth', '/guest'],
  main: ['/app/documents', '/app/trash', '/app/profile'],
  project: [
    '/project/:projectId/prepare',
    '/project/:projectId/review',
    '/project/:projectId/uploading',
    '/project/:projectId/writing',
  ],
} as const
