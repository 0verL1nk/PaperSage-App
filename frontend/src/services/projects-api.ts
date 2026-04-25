/** Projects API hooks. */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { deleteFetch, get, post, put } from '@/services'
import type { Project, ProjectGroup } from '@/types/projects'
import { PROJECTS } from './api-routes'

// Projects

export function useProjects(groupId?: string, includeDeleted = false) {
  return useQuery({
    queryKey: ['projects', groupId, includeDeleted],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (groupId) params.set('groupId', groupId)
      if (includeDeleted) params.set('includeDeleted', 'true')

      const qs = params.toString()
      const url = qs ? `${PROJECTS.list}?${qs}` : PROJECTS.list
      const data = await get<Project[]>(url)
      return data ?? []
    },
  })
}

export function useProject(projectId: string) {
  return useQuery({
    queryKey: ['project', projectId],
    queryFn: () => get<Project>(PROJECTS.detail(projectId)),
  })
}

export function useCreateProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name: string; description?: string; group_id?: string }) =>
      post<Project>(PROJECTS.list, payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

export function useUpdateProject(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name?: string; description?: string; group_id?: string; status?: string }) =>
      put<Project>(PROJECTS.detail(projectId), payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['project', projectId] })
    },
  })
}

export function useDeleteProject(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => deleteFetch(PROJECTS.detail(projectId)).then(() => ({ id: projectId })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

// Groups

export function useGroups() {
  return useQuery({
    queryKey: ['groups'],
    queryFn: async () => {
      const data = await get<ProjectGroup[]>(PROJECTS.groups)
      return data ?? []
    },
  })
}

export function useCreateGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name: string }) =>
      post<ProjectGroup>(PROJECTS.groups, payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['groups'] }),
  })
}

export function useUpdateGroup(groupId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name?: string; sort_order?: number }) =>
      put<ProjectGroup>(PROJECTS.groupDetail(groupId), payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['groups'] }),
  })
}

export function useDeleteGroup(groupId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => deleteFetch(PROJECTS.groupDetail(groupId)).then(() => ({ id: groupId })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['groups'] })
      qc.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

// Trash

export function useTrash() {
  return useQuery({
    queryKey: ['trash'],
    queryFn: async () => {
      const data = await get<TrashItem[]>(PROJECTS.trash)
      return data ?? []
    },
  })
}

export function useRestoreProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (projectId: string) =>
      post(PROJECTS.restoreProject(projectId), {}).then(() => ({ id: projectId })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['trash'] })
    },
  })
}

export function usePermanentDeleteProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (projectId: string) =>
      deleteFetch(PROJECTS.deleteProjectPermanently(projectId)).then(() => ({ id: projectId })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['trash'] }),
  })
}

// Types

export interface TrashItem {
  type: string
  id: string
  name: string
  deleted_at: string
  original_data?: Record<string, unknown>
}
