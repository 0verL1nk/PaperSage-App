import { create } from 'zustand'

import type { BrowserLatexProgress, BrowserLatexResourceEvent } from '@/lib/latex/swiftlatex-browser-compiler'

/** Where a project's browser compile stands. success/failed/cancelled are terminal. */
export type CompilePhase = 'preparing' | 'compiling' | 'uploading' | 'success' | 'failed' | 'cancelled'

/** 失败诊断的异步上报状态；null = 本次编译没有触发上报（成功/取消）。 */
export type DiagnosticStatus = 'pending' | 'sent' | 'failed'

export type BrowserEngineLicense = NonNullable<Extract<BrowserLatexProgress, { stage: 'convert' }>['license']>

/** A finished PDF whose server upload failed; kept so retry does not recompile. */
export interface PendingBrowserUpload {
  jobId: string
  pdf: Uint8Array
  logs: string
}

export interface ProjectCompileState {
  phase: CompilePhase
  projectName: string
  jobId: string | null
  buildId: string | null
  progressMessage: string | null
  license: BrowserEngineLicense | null
  resources: Record<string, BrowserLatexResourceEvent>
  /** Latest PDF compiled in this session, so the preview survives leaving the page. */
  pdf: Uint8Array | null
  error: string | null
  logs: string | null
  warning: string | null
  pendingUpload: PendingBrowserUpload | null
  retryingUpload: boolean
  diagnosticStatus: DiagnosticStatus | null
  /** 服务端返回的诊断编号；上报成功后展示给用户便于反馈。 */
  diagnosticId: string | null
}

const ACTIVE_PHASES: ReadonlySet<CompilePhase> = new Set(['preparing', 'compiling', 'uploading'])

export function isCompileActive(phase: CompilePhase | undefined): boolean {
  return phase !== undefined && ACTIVE_PHASES.has(phase)
}

function freshCompileState(projectName: string): ProjectCompileState {
  return {
    phase: 'preparing',
    projectName,
    jobId: null,
    buildId: null,
    progressMessage: '正在保存并读取项目文件',
    license: null,
    resources: {},
    pdf: null,
    error: null,
    logs: null,
    warning: null,
    pendingUpload: null,
    retryingUpload: false,
    diagnosticStatus: null,
    diagnosticId: null,
  }
}

interface CompileStoreState {
  /** Compile state per project; the manager owns writes, the page reads. */
  projects: Record<string, ProjectCompileState>
  /** The writing page currently on screen — decides whether a finish needs a toast. */
  activeWritingProjectId: string | null
  begin: (projectId: string, projectName: string) => void
  patch: (projectId: string, patch: Partial<ProjectCompileState>) => void
  setActiveWritingProject: (projectId: string | null) => void
  reset: () => void
}

export const useCompileStore = create<CompileStoreState>()((set) => ({
  projects: {},
  activeWritingProjectId: null,
  begin: (projectId, projectName) => set((state) => ({
    projects: { ...state.projects, [projectId]: freshCompileState(projectName) },
  })),
  patch: (projectId, patch) => set((state) => {
    const current = state.projects[projectId]
    if (!current) return state
    return { projects: { ...state.projects, [projectId]: { ...current, ...patch } } }
  }),
  setActiveWritingProject: (projectId) => set({ activeWritingProjectId: projectId }),
  reset: () => set({ projects: {}, activeWritingProjectId: null }),
}))

/** True while leaving the page would lose a running compile or an unsaved PDF. */
export function hasUnfinishedCompile(state: Pick<CompileStoreState, 'projects'>): boolean {
  return Object.values(state.projects).some((project) =>
    isCompileActive(project.phase) || project.pendingUpload !== null)
}
