/** 科研组织产物 API hooks（服务端产物需要登录；游客不隐式上传）。 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { deleteFetch, get, PAGE_QUERY_TIMEOUT_MS, post } from '@/services'
import { GuestCapabilityError } from '@/services/guest-local-workspace'
import { useSessionStore } from '@/store'
import type {
  AddResearchRevisionInput,
  CreateResearchArtifactInput,
  ResearchArtifact,
  ResearchArtifactKind,
  ResearchRevision,
} from '@/types/research'
import { RESEARCH } from './api-routes'

function requireSignedIn() {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError('资料整理')
}

const KEY = 'research-artifacts'

export const researchKeys = {
  list: (projectId: string, kind?: ResearchArtifactKind) => [KEY, projectId, kind ?? 'all'] as const,
}

export function useResearchArtifacts(
  projectId: string | undefined,
  kind?: ResearchArtifactKind,
  options?: { silent?: boolean },
) {
  return useQuery({
    queryKey: researchKeys.list(projectId ?? '', kind),
    enabled: Boolean(projectId),
    queryFn: async () => {
      requireSignedIn()
      const url = kind ? `${RESEARCH.list(projectId!)}?kind=${kind}` : RESEARCH.list(projectId!)
      return (await get<ResearchArtifact[]>(url, {
        timeout: PAGE_QUERY_TIMEOUT_MS,
        // 页面自带加载/错误态时不再叠加全局 toast（检查页曾一进页弹两条）。
        skipGlobalLoading: options?.silent,
      })) ?? []
    },
  })
}

export function useResearchRevision(artifactId: string | undefined, revision: number | undefined) {
  return useQuery({
    queryKey: [KEY, 'revision', artifactId, revision],
    enabled: Boolean(artifactId) && typeof revision === 'number',
    queryFn: async () => {
      requireSignedIn()
      return get<ResearchRevision>(RESEARCH.revision(artifactId!, revision!))
    },
  })
}

export function useCreateResearchArtifact(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: CreateResearchArtifactInput) => {
      requireSignedIn()
      return post<ResearchArtifact>(RESEARCH.create(projectId), input)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [KEY, projectId] }),
  })
}

export function useAddResearchRevision(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ artifactId, input }: { artifactId: string; input: AddResearchRevisionInput }) => {
      requireSignedIn()
      return post<ResearchArtifact>(RESEARCH.addRevision(artifactId), input)
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [KEY, projectId] }),
  })
}

export function useAdoptResearchRevision(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ artifactId, revision, expectedRevision }: { artifactId: string; revision: number; expectedRevision: number }) => {
      requireSignedIn()
      return post<ResearchArtifact>(RESEARCH.adopt(artifactId), { revision, expectedRevision })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [KEY, projectId] }),
  })
}

export function useRejectResearchRevision(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ artifactId, revision, expectedRevision }: { artifactId: string; revision: number; expectedRevision: number }) => {
      requireSignedIn()
      return post<ResearchArtifact>(RESEARCH.reject(artifactId), { revision, expectedRevision })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [KEY, projectId] }),
  })
}

export function useDeleteResearchArtifact(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (artifactId: string) => {
      requireSignedIn()
      return deleteFetch(RESEARCH.delete(artifactId))
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: [KEY, projectId] }),
  })
}
