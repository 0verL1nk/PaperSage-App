import { useEffect } from 'react'

import { useCompileStore, type ProjectCompileState } from '@/store/compile-store'

/**
 * Compile state for one project. The manager owns it outside React, so it
 * survives leaving the writing page; mounting here only tells the manager
 * that this project's inline progress is on screen (suppresses the toast).
 */
export function useProjectCompile(projectId: string): ProjectCompileState | null {
  useEffect(() => {
    const store = useCompileStore.getState()
    store.setActiveWritingProject(projectId)
    return () => {
      if (useCompileStore.getState().activeWritingProjectId === projectId) {
        useCompileStore.getState().setActiveWritingProject(null)
      }
    }
  }, [projectId])

  return useCompileStore((state) => state.projects[projectId] ?? null)
}
