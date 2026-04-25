import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getJson, postJson, deleteFetch } from './client'
import type { Folder, CreateFolderInput } from '@/types/files'
import { FOLDERS } from './api-routes'

export function useProjectFolders(projectId: string) {
  return useQuery({
    queryKey: ['folders', projectId],
    queryFn: () => getJson<Folder[]>(FOLDERS.listByProject(projectId)),
    enabled: !!projectId,
  })
}

export function useCreateFolder() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (data: CreateFolderInput) => postJson<Folder>(FOLDERS.create, data),
    onSuccess: (_, variables) =>
      qc.invalidateQueries({ queryKey: ['folders', variables.projectId] }),
  })
}

export function useDeleteFolder(projectId: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (folderId: string) => deleteFetch(FOLDERS.delete(folderId)),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['folders', projectId] })
      qc.invalidateQueries({ queryKey: ['documents'] })
    },
  })
}
