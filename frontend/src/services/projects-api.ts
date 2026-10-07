/** Projects API hooks. */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { deleteFetch, get, post, put } from '@/services'
import { useSessionStore } from '@/store'
import type { Project, ProjectGroup, ProjectScenario } from '@/types/projects'
import { PROJECTS } from './api-routes'
import { createGuestProject, deleteGuestProject, getGuestProject, GuestCapabilityError, listGuestProjects, permanentlyDeleteGuestProject, restoreGuestProject, updateGuestProject } from './guest-local-workspace'

// Projects

export function useProjects(groupId?: string, includeDeleted = false) {
  return useQuery({
    queryKey: ['projects', groupId, includeDeleted],
    queryFn: async () => {
      if (useSessionStore.getState().isGuest) return listGuestProjects(groupId, includeDeleted)
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
    queryFn: () => useSessionStore.getState().isGuest ? getGuestProject(projectId) : get<Project>(PROJECTS.detail(projectId)),
  })
}

export function useCreateProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name: string; description?: string; group_id?: string; scenario?: ProjectScenario }) =>
      useSessionStore.getState().isGuest ? createGuestProject(payload) : post<Project>(PROJECTS.list, payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

export function useUpdateProject(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name?: string; description?: string; group_id?: string; status?: string }) =>
      useSessionStore.getState().isGuest ? updateGuestProject(projectId, payload) : put<Project>(PROJECTS.detail(projectId), payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['project', projectId] })
    },
  })
}

export function useDeleteProject(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      if (useSessionStore.getState().isGuest) await deleteGuestProject(projectId)
      else await deleteFetch(PROJECTS.detail(projectId))
      return { id: projectId }
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['projects'] }),
  })
}

// Groups

export function useGroups() {
  return useQuery({
    queryKey: ['groups'],
    queryFn: async () => {
      if (useSessionStore.getState().isGuest) return []
      const data = await get<ProjectGroup[]>(PROJECTS.groups)
      return data ?? []
    },
  })
}

export function useCreateGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name: string }) => useSessionStore.getState().isGuest
      ? Promise.reject(new GuestCapabilityError('项目分组'))
      : post<ProjectGroup>(PROJECTS.groups, payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['groups'] }),
  })
}

export function useUpdateGroup(groupId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (payload: { name?: string; sort_order?: number }) => useSessionStore.getState().isGuest
      ? Promise.reject(new GuestCapabilityError('项目分组'))
      : put<ProjectGroup>(PROJECTS.groupDetail(groupId), payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['groups'] }),
  })
}

export function useDeleteGroup(groupId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => useSessionStore.getState().isGuest
      ? Promise.reject(new GuestCapabilityError('项目分组'))
      : deleteFetch(PROJECTS.groupDetail(groupId)).then(() => ({ id: groupId })),
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
      if (useSessionStore.getState().isGuest) {
        const projects = await listGuestProjects(undefined, true)
        return projects.filter((project) => project.is_deleted).map((project) => ({
          type: 'project', id: project.id, name: project.name, deleted_at: project.deleted_at ?? project.updated_at,
        }))
      }
      const data = await get<TrashItem[]>(PROJECTS.trash)
      return data ?? []
    },
  })
}

export function useRestoreProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (projectId: string) => {
      if (useSessionStore.getState().isGuest) await restoreGuestProject(projectId)
      else await post(PROJECTS.restoreProject(projectId), {})
      return { id: projectId }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['projects'] })
      qc.invalidateQueries({ queryKey: ['trash'] })
    },
  })
}

export function usePermanentDeleteProject() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (projectId: string) => {
      if (useSessionStore.getState().isGuest) await permanentlyDeleteGuestProject(projectId)
      else await deleteFetch(PROJECTS.deleteProjectPermanently(projectId))
      return { id: projectId }
    },
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
