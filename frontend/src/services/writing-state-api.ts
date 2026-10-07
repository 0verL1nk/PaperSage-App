/** Project-level preparation notes and user-maintained phase state. */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { get, put } from '@/services'
import { useSessionStore } from '@/store'
import type { ProjectScenario } from '@/types/projects'
import { WRITING_STATE } from './api-routes'
import { getGuestWritingState, saveGuestWritingState } from './guest-local-workspace'

export interface WritingBrief {
  topic: string
  background: string
  questions: string[]
  goals: string[]
  scope_out: string[]
  objects_and_data: string
  methods: string
  key_chapters: string[]
  milestones: string[]
  constraints: string[]
  known_facts: string[]
  assumptions: string[]
  unverified: string[]
}

export interface WritingRequirement {
  id: string
  text: string
  source: string
  note: string
}

export type WritingPhase = 'prepare' | 'write' | 'review'
export type PhaseStatus = 'not_started' | 'in_progress' | 'done'

export interface WritingState {
  project_id: string
  scenario: ProjectScenario
  brief: WritingBrief
  requirements: WritingRequirement[]
  phases: Record<WritingPhase, PhaseStatus>
  current_phase: WritingPhase
  revision: number
}

export type SaveWritingState = Omit<WritingState, 'project_id' | 'scenario'>

export const writingStateKeys = {
  project: (projectId: string) => ['writing-state', projectId] as const,
}

export function useWritingState(projectId: string) {
  return useQuery({
    queryKey: writingStateKeys.project(projectId),
    queryFn: () => useSessionStore.getState().isGuest ? getGuestWritingState(projectId) : get<WritingState>(WRITING_STATE.project(projectId)),
    enabled: Boolean(projectId),
  })
}

export function useSaveWritingState(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (state: SaveWritingState) =>
      useSessionStore.getState().isGuest ? saveGuestWritingState(projectId, state) : put<WritingState>(WRITING_STATE.project(projectId), state),
    onSuccess: (state) => {
      queryClient.setQueryData(writingStateKeys.project(projectId), state)
    },
  })
}

export function getConflictState(error: unknown): WritingState | null {
  if (!(error instanceof Error) || !('details' in error)) return null
  const details = (error as Error & { details?: unknown }).details
  if (!details || typeof details !== 'object' || !('data' in details)) return null
  const data = (details as { data?: unknown }).data
  return data && typeof data === 'object' && 'revision' in data
    ? (data as WritingState)
    : null
}
