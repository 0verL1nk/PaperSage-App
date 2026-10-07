import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getJson, postJson, deleteFetch } from './client'
import type { Folder, CreateFolderInput } from '@/types/files'
import { FOLDERS } from './api-routes'
import { useSessionStore } from '@/store'
import { createGuestFolder, deleteGuestFolder, listGuestFolders } from './guest-local-workspace'

function toSnakeFolder(data: CreateFolderInput) {
  return {
    name: data.name,
    project_id: data.projectId,
    parent_id: data.parentId ?? null,
  }
}

function mapFolderKeys(raw: Record<string, unknown>): Folder {
  return {
    id: raw.id as string,
    name: raw.name as string,
    parentId: (raw.parent_id ?? raw.parentId) as string | null,
    projectId: (raw.project_id ?? raw.projectId) as string | undefined,
    createdAt: (raw.created_at ?? raw.createdAt) as string | undefined,
    updatedAt: (raw.updated_at ?? raw.updatedAt) as string | undefined,
  }
}

export function useProjectFolders(projectId: string) {
  return useQuery({
    queryKey: ['folders', projectId],
    queryFn: async () => {
      if (useSessionStore.getState().isGuest) return listGuestFolders(projectId)
      const raw = await getJson<Record<string, unknown>[]>(FOLDERS.listByProject(projectId))
      return raw.map(mapFolderKeys)
    },
    enabled: !!projectId,
  })
}

export function useCreateFolder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateFolderInput) => useSessionStore.getState().isGuest
      ? createGuestFolder(data)
      : postJson<Folder>(FOLDERS.create, toSnakeFolder(data)),
    onSuccess: (_, variables) =>
      qc.invalidateQueries({ queryKey: ['folders', variables.projectId] }),
  })
}

export function useDeleteFolder(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (folderId: string) => useSessionStore.getState().isGuest
      ? deleteGuestFolder(folderId)
      : deleteFetch(FOLDERS.delete(folderId)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['folders', projectId] })
      qc.invalidateQueries({ queryKey: ['documents'] })
    },
  })
}
