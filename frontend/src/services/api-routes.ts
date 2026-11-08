/**
 * Centralized API route definitions.
 * All endpoint URLs are defined here to avoid scattering across service files.
 */

const V1 = '/api/v1'

// --- Auth ---
export const AUTH = {
  login: `${V1}/auth/login`,
  me: `${V1}/auth/me`,
  register: `${V1}/auth/register`,
  logout: `${V1}/auth/logout`,
  refresh: `${V1}/auth/refresh`,
  passwordResetRequest: `${V1}/auth/password-reset/request`,
  passwordResetConfirm: `${V1}/auth/password-reset/confirm`,
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

// --- Project writing state ---
export const WRITING_STATE = {
  project: (projectId: string) => `${V1}/projects/${projectId}/writing-state`,
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
  confirm: (documentId: string) => `${V1}/documents/${documentId}/confirm`,
} as const

// --- Folders ---
export const FOLDERS = {
  listByProject: (projectId: string) => `${V1}/projects/${projectId}/folders`,
  create: `${V1}/folders`,
  delete: (id: string) => `${V1}/folders/${id}`,
} as const

// --- LaTeX ---
export const LATEX = {
  files: (projectId: string) => `${V1}/projects/${projectId}/latex-files/`,
  fileContent: (projectId: string, path: string) =>
    `${V1}/projects/${projectId}/latex-files/${encodeURIComponent(path)}`,
  renameFile: (projectId: string, path: string) =>
    `${V1}/projects/${projectId}/latex-files/${encodeURIComponent(path)}/rename`,
  renameFolder: (projectId: string) =>
    `${V1}/projects/${projectId}/latex-files/folder/rename`,
  deleteFolder: (projectId: string) =>
    `${V1}/projects/${projectId}/latex-files/folder`,
  moveFile: (projectId: string, path: string) =>
    `${V1}/projects/${projectId}/latex-files/${encodeURIComponent(path)}/move`,
  compile: (projectId: string) => `${V1}/projects/${projectId}/latex/compile/`,
  browserCompile: (projectId: string) => `${V1}/projects/${projectId}/latex/compile/browser`,
  browserCompileResult: (projectId: string, jobId: string) =>
    `${V1}/projects/${projectId}/latex/compile/${jobId}/browser-result`,
  browserCompileFailure: (projectId: string, jobId: string) =>
    `${V1}/projects/${projectId}/latex/compile/${jobId}/browser-failure`,
  compileStatus: (projectId: string, jobId: string) =>
    `${V1}/projects/${projectId}/latex/compile/${jobId}/status/`,
  compileHistory: (projectId: string) =>
    `${V1}/projects/${projectId}/latex/compile/history/`,
  outputZip: (projectId: string, buildId: string) =>
    `${V1}/projects/${projectId}/latex/output/${buildId}/output.zip`,
  outputPdf: (projectId: string, buildId: string) =>
    `${V1}/projects/${projectId}/latex/output/${buildId}/pdf`,
} as const

// --- Settings ---
export const SETTINGS = {
  all: `${V1}/settings`,
  category: (category: string) => `${V1}/settings/${category}`,
  agentTest: `${V1}/settings/agent/test`,
} as const

// --- Agent ---
export const AGENT = {
  conversations: (projectId: string) => `${V1}/projects/${projectId}/agent/conversations`,
  conversation: (conversationId: string) => `${V1}/agent/conversations/${conversationId}`,
  messages: (conversationId: string) => `${V1}/agent/conversations/${conversationId}/messages`,
  /** 模式 B：先建 Run 再订阅事件 */
  createRun: (conversationId: string) => `${V1}/agent/conversations/${conversationId}/runs`,
  runEvents: (runId: string, afterSeq?: number) =>
    `${V1}/agent/runs/${runId}/events${afterSeq ? `?afterSeq=${afterSeq}` : ''}`,
  runEvidence: (runId: string) => `${V1}/agent/runs/${runId}/evidence`,
  cancelRun: (runId: string) => `${V1}/agent/runs/${runId}/cancel`,
  retryRun: (runId: string) => `${V1}/agent/runs/${runId}/retry`,
  proposals: (projectId: string, status?: string) =>
    `${V1}/projects/${projectId}/agent/proposals${status ? `?status=${status}` : ''}`,
  applyProposal: (id: string) => `${V1}/agent/change-proposals/${id}/apply`,
  rejectProposal: (id: string) => `${V1}/agent/change-proposals/${id}/reject`,
  artifact: (id: string) => `${V1}/agent/artifacts/${id}`,
} as const
