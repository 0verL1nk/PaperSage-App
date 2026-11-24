/** 学术规范资源库 API hooks（需要登录；游客不允许隐式写云端规范）。 */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { deleteFetch, get, PAGE_QUERY_TIMEOUT_MS, patchJson, post } from '@/services'
import { GuestCapabilityError } from '@/services/guest-local-workspace'
import { useSessionStore } from '@/store'
import { NORMS } from './api-routes'
import type {
  NormAdoption,
  NormAdoptionState,
  NormEntryPage,
  NormEntryQuery,
  NormPack,
} from '@/types/norms'

function requireSignedIn() {
  if (useSessionStore.getState().isGuest) throw new GuestCapabilityError('学术规范库')
}

const KEY = 'norms'

export const normsKeys = {
  packs: [KEY, 'packs'] as const,
  entries: (query: NormEntryQuery) => [KEY, 'entries', query] as const,
  project: (projectId: string) => [KEY, 'project', projectId] as const,
}

export function entriesURL(query: NormEntryQuery) {
  const params = new URLSearchParams()
  Object.entries(query).forEach(([key, value]) => {
    if (value === undefined || value === '') return
    params.set(key, String(value))
  })
  const suffix = params.toString()
  return suffix ? `${NORMS.entries}?${suffix}` : NORMS.entries
}

export function useNormPacks() {
  return useQuery({
    queryKey: normsKeys.packs,
    queryFn: async () => {
      requireSignedIn()
      return (await get<NormPack[]>(NORMS.packs, { timeout: PAGE_QUERY_TIMEOUT_MS })) ?? []
    },
  })
}

export function useNormEntries(query: NormEntryQuery, options?: { enabled?: boolean }) {
  return useQuery({
    queryKey: normsKeys.entries(query),
    enabled: options?.enabled ?? true,
    queryFn: async () => {
      requireSignedIn()
      return get<NormEntryPage>(entriesURL(query), {
        timeout: PAGE_QUERY_TIMEOUT_MS,
        // 抽屉自带加载态，不再叠加全局 loading。
        skipGlobalLoading: true,
      })
    },
  })
}

export function useProjectNorms(projectId: string | undefined) {
  return useQuery({
    queryKey: normsKeys.project(projectId ?? ''),
    enabled: Boolean(projectId),
    queryFn: async () => {
      requireSignedIn()
      return (await get<NormAdoption[]>(NORMS.projectNorms(projectId!), {
        timeout: PAGE_QUERY_TIMEOUT_MS,
      })) ?? []
    },
  })
}

export function useAdoptNorms(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { entryIds?: string[]; packId?: string }) => {
      requireSignedIn()
      return post<NormAdoption[]>(NORMS.adopt(projectId), input)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: normsKeys.project(projectId) })
    },
  })
}

export function useUpdateAdoption(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: { entryId: string; state?: NormAdoptionState; overrideText?: string }) => {
      requireSignedIn()
      return patchJson<NormAdoption>(NORMS.adoption(projectId, input.entryId), {
        state: input.state,
        overrideText: input.overrideText,
      })
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: normsKeys.project(projectId) })
    },
  })
}

export function useUnadoptNorm(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (entryId: string) => {
      requireSignedIn()
      await deleteFetch(NORMS.adoption(projectId, entryId))
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: normsKeys.project(projectId) })
    },
  })
}

/** 已采用条目按 entryId 建索引，供列表勾选态直接使用。 */
export function adoptionIndex(adoptions: NormAdoption[]): Map<string, NormAdoption> {
  return new Map(adoptions.map((adoption) => [adoption.entryId, adoption]))
}

/** 已采用/已启用数量，准备页卡片摘要用。 */
export function adoptionSummary(adoptions: NormAdoption[]) {
  const enabled = adoptions.filter((item) => item.state === 'enabled')
  return {
    total: adoptions.length,
    enabled: enabled.length,
    outdated: adoptions.filter((item) => item.hasNewerVersion).length,
    categories: [...new Set(enabled.map((item) => item.entry.category))].slice(0, 4),
  }
}
