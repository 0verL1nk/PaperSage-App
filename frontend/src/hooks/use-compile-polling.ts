import { useEffect, useRef, useState } from 'react'
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

/** A running job whose `updated_at` stops moving for this long is abandoned:
 * browser compiles refresh the job with heartbeats while they are alive. */
export const COMPILE_STATUS_STALL_MS = 5 * 60_000

const compileKeys = {
  status: (projectId: string, jobId: string) =>
    ['compile-status', projectId, jobId] as const,
}

/**
 * Poll compile job status with variable interval.
 * Automatically stops polling on terminal states (success/failed/timeout/cancelled)
 * and after `COMPILE_STATUS_STALL_MS` without any update, so a dead job can never
 * hold the editor in 「编译中」 forever.
 */
export function useCompileStatusPolling(
  projectId: string,
  jobId: string | null,
) {
  const stalledRef = useRef(false)
  const [stalledJobId, setStalledJobId] = useState<string | null>(null)
  const query = useQuery<CompileJobStatusResponse>({
    queryKey: compileKeys.status(projectId, jobId ?? ''),
    queryFn: () => getCompileStatus(projectId, jobId!),
    enabled: !!jobId,
    refetchInterval: (q) => {
      const data = q.state.data
      if (!data) return POLL_INTERVAL_PENDING_MS
      if (TERMINAL_STATUSES.has(data.status) || stalledRef.current) return false
      return data.status === 'running'
        ? POLL_INTERVAL_RUNNING_MS
        : POLL_INTERVAL_PENDING_MS
    },
  })

  const status = query.data?.status
  const updatedAt = query.data?.updated_at
  useEffect(() => {
    stalledRef.current = false
    if (!jobId || !status || TERMINAL_STATUSES.has(status)) return
    const timer = window.setTimeout(() => {
      stalledRef.current = true
      setStalledJobId(jobId)
    }, COMPILE_STATUS_STALL_MS)
    return () => window.clearTimeout(timer)
  }, [jobId, status, updatedAt])

  return { ...query, isStalled: jobId !== null && stalledJobId === jobId }
}

/** Check if a compile status is terminal (no further changes). */
export function isTerminalStatus(status: CompileJobStatus): boolean {
  return TERMINAL_STATUSES.has(status)
}
