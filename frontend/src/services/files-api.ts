import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { getJson, deleteFetch, putJson, postMultipart } from './client'
import type { ProjectDocument, MoveDocumentInput, UpdateRemarkInput } from '@/types/files'

// Queries
export function useDocuments(projectId: string, folderId: string | null) {
  return useQuery({
    queryKey: ['documents', projectId, folderId],
    queryFn: () => {
      if (folderId) {
        return getJson<ProjectDocument[]>(`/api/folders/${folderId}/documents`)
      }
      return getJson<ProjectDocument[]>(`/api/projects/${projectId}/documents`)
    },
    enabled: !!projectId,
  })
}

export function useDocumentPreview(documentId: string | null) {
  return useQuery({
    queryKey: ['document-preview', documentId],
    queryFn: () => getJson<{ url: string }>(`/api/documents/${documentId}/preview`),
    enabled: !!documentId,
  })
}

// Mutations
export function useDeleteDocument(projectId: string, folderId: string | null) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (documentId: string) => deleteFetch(`/api/documents/${documentId}`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['documents', projectId, folderId] })
    },
  })
}

export function useMoveDocument(projectId: string, currentFolderId: string | null) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, data }: { documentId: string; data: MoveDocumentInput }) => 
      putJson<ProjectDocument>(`/api/documents/${documentId}/move`, data),
    onSuccess: (_, variables) => {
      // Invalidate current folder
      queryClient.invalidateQueries({ queryKey: ['documents', projectId, currentFolderId] })
      // Invalidate target folder
      queryClient.invalidateQueries({ queryKey: ['documents', projectId, variables.data.folderId] })
    },
  })
}

export function useUpdateRemark(projectId: string, folderId: string | null) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, data }: { documentId: string; data: UpdateRemarkInput }) => 
      putJson<ProjectDocument>(`/api/documents/${documentId}/remark`, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['documents', projectId, folderId] })
    },
  })
}

export function useUploadDocument(projectId: string, folderId: string | null) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ file, remark, onUploadProgress }: { file: File; remark?: string; onUploadProgress?: (progressEvent: any) => void }) => {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('projectId', projectId)
      if (folderId) {
        formData.append('folderId', folderId)
      }
      if (remark) {
        formData.append('remark', remark)
      }
      
      return postMultipart<ProjectDocument>('/api/documents/upload', formData, { onUploadProgress })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['documents', projectId, folderId] })
    },
  })
}
