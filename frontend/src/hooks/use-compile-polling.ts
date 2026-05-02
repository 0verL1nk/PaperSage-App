import { useQuery } from '@tanstack/react-query'
import {
  getCompileStatus,
  type CompileJobStatusResponse,
  type CompileJobStatus,
} from '@/services/latex-api'
import {
  POLL_INTERVAL_PENDING_MS,
  POLL_INTERVAL_RUNNING_MS,
  TERMINAL_STATUSES,
} from '@/views/projects/constants'

const compileKeys = {
  status: (projectId: string, jobId: string) =>
    ['compile-status', projectId, jobId] as const,
}

/**
 * Poll compile job status with variable interval.
 * Automatically stops polling on terminal states (success/failed/timeout/cancelled).
 */
export function useCompileStatusPolling(
  projectId: string,
  jobId: string | null,
) {
  return useQuery<CompileJobStatusResponse>({
    queryKey: compileKeys.status(projectId, jobId ?? ''),
    queryFn: () => getCompileStatus(projectId, jobId!),
    enabled: !!jobId,
    refetchInterval: (query) => {
      const data = query.state.data
      if (!data) return POLL_INTERVAL_PENDING_MS
      if (TERMINAL_STATUSES.has(data.status)) return false
      return data.status === 'running'
        ? POLL_INTERVAL_RUNNING_MS
        : POLL_INTERVAL_PENDING_MS
    },
  })
}

/** Check if a compile status is terminal (no further changes). */
export function isTerminalStatus(status: CompileJobStatus): boolean {
  return TERMINAL_STATUSES.has(status)
}
