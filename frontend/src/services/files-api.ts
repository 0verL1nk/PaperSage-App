import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { AxiosProgressEvent } from 'axios'
import { getJson, deleteFetch, putJson, postMultipart } from './client'
import type { ProjectDocument, MoveDocumentInput, UpdateRemarkInput } from '@/types/files'
import { DOCUMENTS } from './api-routes'

export function useDocuments(projectId: string, folderId: string | null) {
  return useQuery({
    queryKey: ['documents', projectId, folderId],
    queryFn: () => {
      const url = folderId
        ? DOCUMENTS.listByFolder(folderId)
        : DOCUMENTS.listByProject(projectId)
      return getJson<ProjectDocument[]>(url)
    },
    enabled: !!projectId,
  })
}

export function useDocumentPreview(documentId: string | null) {
  return useQuery({
    queryKey: ['document-preview', documentId],
    queryFn: () => getJson<{ url: string }>(DOCUMENTS.preview(documentId!)),
    enabled: !!documentId,
  })
}

export function useDeleteDocument(projectId: string, folderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentId: string) => deleteFetch(DOCUMENTS.delete(documentId)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] }),
  })
}

export function useMoveDocument(projectId: string, currentFolderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, data }: { documentId: string; data: MoveDocumentInput }) =>
      putJson<ProjectDocument>(DOCUMENTS.move(documentId), data),
    onSuccess: (_, variables) => {
      qc.invalidateQueries({ queryKey: ['documents', projectId, currentFolderId] })
      qc.invalidateQueries({ queryKey: ['documents', projectId, variables.data.folderId] })
    },
  })
}

export function useUpdateRemark(projectId: string, folderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, data }: { documentId: string; data: UpdateRemarkInput }) =>
      putJson<ProjectDocument>(DOCUMENTS.remark(documentId), data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] }),
  })
}

export function useUploadDocument(projectId: string, folderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ file, remark, onUploadProgress }: {
      file: File; remark?: string; onUploadProgress?: (e: AxiosProgressEvent) => void
    }) => {
      const formData = new FormData()
      formData.append('file', file)
      formData.append('projectId', projectId)
      if (folderId) formData.append('folderId', folderId)
      if (remark) formData.append('remark', remark)
      return postMultipart<ProjectDocument>(DOCUMENTS.upload, formData, { onUploadProgress })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] }),
  })
}
