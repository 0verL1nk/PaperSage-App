/**
 * Centralized API route definitions.
 * All endpoint URLs are defined here to avoid scattering across service files.
 */

const V1 = '/api/v1'

// --- Auth ---
export const AUTH = {
  login: `${V1}/auth/login`,
  register: `${V1}/auth/register`,
  logout: `${V1}/auth/logout`,
  refresh: `${V1}/auth/refresh`,
} as const

// --- Projects ---
export const PROJECTS = {
  list: `${V1}/projects/`,
  detail: (id: string) => `${V1}/projects/${id}`,
  groups: `${V1}/projects/groups`,
  groupDetail: (id: string) => `${V1}/projects/groups/${id}`,
  trash: `${V1}/projects/trash`,
  restoreProject: (id: string) => `${V1}/projects/trash/project/${id}/restore`,
  deleteProjectPermanently: (id: string) => `${V1}/projects/trash/project/${id}`,
} as const

// --- Documents ---
export const DOCUMENTS = {
  listByProject: (projectId: string) => `${V1}/projects/${projectId}/documents`,
  listByFolder: (folderId: string) => `${V1}/folders/${folderId}/documents`,
  preview: (id: string) => `${V1}/documents/${id}/preview`,
  move: (id: string) => `${V1}/documents/${id}/move`,
  remark: (id: string) => `${V1}/documents/${id}/remark`,
  delete: (id: string) => `${V1}/documents/${id}`,
  upload: `${V1}/documents/upload`,
} as const

// --- Folders ---
export const FOLDERS = {
  listByProject: (projectId: string) => `${V1}/projects/${projectId}/folders`,
  create: `${V1}/folders`,
  delete: (id: string) => `${V1}/folders/${id}`,
} as const

// --- LaTeX ---
export const LATEX = {
  files: (projectId: string) => `${V1}/projects/${projectId}/latex-files`,
  fileContent: (projectId: string, path: string) =>
    `${V1}/projects/${projectId}/latex-files/${encodeURIComponent(path)}`,
  compile: (projectId: string) => `${V1}/projects/${projectId}/latex/compile`,
  outputZip: (projectId: string, buildId: string) =>
    `${V1}/projects/${projectId}/latex/output/${buildId}/output.zip`,
  outputPdf: (projectId: string, buildId: string) =>
    `${V1}/projects/${projectId}/latex/output/${buildId}/pdf`,
} as const
