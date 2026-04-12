import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getJson, postJson, deleteFetch } from './client'
import type { Folder, CreateFolderInput } from '@/types/files'

// Queries
export function useProjectFolders(projectId: string) {
  return useQuery({
    queryKey: ['folders', projectId],
    queryFn: () => getJson<Folder[]>(`/api/projects/${projectId}/folders`),
    enabled: !!projectId,
  })
}

// Mutations
export function useCreateFolder() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateFolderInput) => postJson<Folder>('/api/folders', data),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: ['folders', variables.projectId] })
    },
  })
}

export function useDeleteFolder(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (folderId: string) => deleteFetch(`/api/folders/${folderId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['folders', projectId] })
      // If we are deleting a folder, its documents also disappear
      queryClient.invalidateQueries({ queryKey: ['documents'] })
    },
  })
}
