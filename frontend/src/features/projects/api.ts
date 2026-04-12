/** Projects API hooks. */
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { deleteFetch, get, post, put } from '@/shared/api'
import type { Project, ProjectGroup } from './types'

// Projects

export function useProjects(groupId?: string, includeDeleted = false) {
  return useQuery({
    queryKey: ['projects', groupId, includeDeleted],
    queryFn: async () => {
      const params = new URLSearchParams()
      if (groupId) params.set('groupId', groupId)
      if (includeDeleted) params.set('includeDeleted', 'true')

      const queryString = params.toString()
      const baseUrl = '/api/projects/'
      const url = queryString ? `${baseUrl}?${queryString}` : baseUrl

      const data = await get<Project[]>(url)
      return data ?? []
    },
  })
}

export function useProject(projectId: string) {
  return useQuery({
    queryKey: ['project', projectId],
    queryFn: async () => {
      const data = await get<Project>(`/api/projects/${projectId}`)
      return data
    },
  })
}

export function useCreateProject() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (payload: { name: string; description?: string; group_id?: string }) => {
      const data = await post<Project>('/api/projects/', payload)
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

export function useUpdateProject(projectId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (payload: { name?: string; description?: string; group_id?: string; status?: string }) => {
      const data = await put<Project>(`/api/projects/${projectId}`, payload)
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['project', projectId] })
    },
  })
}

export function useDeleteProject(projectId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => {
      await deleteFetch(`/api/projects/${projectId}`)
      return { id: projectId }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

// Groups

export function useGroups() {
  return useQuery({
    queryKey: ['groups'],
    queryFn: async () => {
      const data = await get<ProjectGroup[]>('/api/projects/groups')
      return data ?? []
    },
  })
}

export function useCreateGroup() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (payload: { name: string }) => {
      const data = await post<ProjectGroup>('/api/projects/groups', payload)
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
    },
  })
}

export function useUpdateGroup(groupId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (payload: { name?: string; sort_order?: number }) => {
      const data = await put<ProjectGroup>(`/api/projects/groups/${groupId}`, payload)
      return data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
    },
  })
}

export function useDeleteGroup(groupId: string) {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async () => {
      await deleteFetch(`/api/projects/groups/${groupId}`)
      return { id: groupId }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['groups'] })
      queryClient.invalidateQueries({ queryKey: ['projects'] })
    },
  })
}

// Trash

export function useTrash() {
  return useQuery({
    queryKey: ['trash'],
    queryFn: async () => {
      const data = await get<TrashItem[]>('/api/projects/trash')
      return data ?? []
    },
  })
}

export function useRestoreProject() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (projectId: string) => {
      await post(`/api/projects/trash/project/${projectId}/restore`, {})
      return { id: projectId }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['projects'] })
      queryClient.invalidateQueries({ queryKey: ['trash'] })
    },
  })
}

export function usePermanentDeleteProject() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (projectId: string) => {
      await deleteFetch(`/api/projects/trash/project/${projectId}`)
      return { id: projectId }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['trash'] })
    },
  })
}

// Types

export interface TrashItem {
  type: string
  id: string // UUID as string
  name: string
  deleted_at: string
  original_data?: Record<string, unknown>
}