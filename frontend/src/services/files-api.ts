import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import type { AxiosProgressEvent } from 'axios'
import { apiClient, getJson, postJson, deleteFetch, putJson, postMultipart } from './client'
import type { ProjectDocument, MoveDocumentInput, UpdateRemarkInput } from '@/types/files'
import { DOCUMENTS } from './api-routes'
import { useSessionStore } from '@/store'
import { confirmGuestDocument, deleteGuestDocument, getGuestDocumentBlob, listGuestDocuments, moveGuestDocument, saveGuestDocument, updateGuestDocumentRemark } from './guest-local-workspace'

export function fetchDocumentPreviewBlob(documentId: string) {
  if (useSessionStore.getState().isGuest) return getGuestDocumentBlob(documentId)
  return apiClient.get<Blob>(DOCUMENTS.preview(documentId), { responseType: 'blob' }).then((response) => response.data)
}

function mapDocKeys(raw: Record<string, unknown>): ProjectDocument {
  return {
    id: raw.id as string,
    name: raw.name as string,
    type: raw.type as string,
    size: raw.size as number,
    status: raw.status as string,
    projectId: (raw.project_id ?? raw.projectId) as string,
    folderId: (raw.folder_id ?? raw.folderId) as string | null,
    remark: (raw.remark ?? null) as string | null,
    createdAt: (raw.created_at ?? raw.createdAt) as string,
    updatedAt: (raw.updated_at ?? raw.updatedAt) as string,
  }
}

export function useDocuments(projectId: string, folderId: string | null) {
  return useQuery({
    queryKey: ['documents', projectId, folderId],
    queryFn: async () => {
      if (useSessionStore.getState().isGuest) return listGuestDocuments(projectId, folderId)
      const url = folderId
        ? DOCUMENTS.listByFolder(folderId)
        : DOCUMENTS.listByProject(projectId)
      const raw = await getJson<Record<string, unknown>[]>(url)
      return raw.map(mapDocKeys)
    },
    enabled: !!projectId,
  })
}

export function useDeleteDocument(projectId: string, folderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentId: string) => useSessionStore.getState().isGuest
      ? deleteGuestDocument(documentId)
      : deleteFetch(DOCUMENTS.delete(documentId)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] }),
  })
}

export function useMoveDocument(projectId: string, currentFolderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ documentId, data }: { documentId: string; data: MoveDocumentInput }) =>
      useSessionStore.getState().isGuest
        ? moveGuestDocument(documentId, data.folderId)
        : putJson<ProjectDocument>(DOCUMENTS.move(documentId), { folder_id: data.folderId }),
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
      useSessionStore.getState().isGuest
        ? updateGuestDocumentRemark(documentId, data.remark)
        : putJson<ProjectDocument>(DOCUMENTS.remark(documentId), data),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] }),
  })
}

/** staged 上传响应：文档行 + 重复标记 */
export type StagedUploadResult = ProjectDocument & { duplicate?: boolean }

/** staged → uploaded（入库） */
export function useConfirmDocument(projectId: string, folderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (documentId: string) => useSessionStore.getState().isGuest
      ? confirmGuestDocument(documentId)
      : postJson<ProjectDocument>(DOCUMENTS.confirm(documentId)),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] }),
  })
}

export function useUploadDocument(projectId: string, folderId: string | null) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async ({ file, remark, staged, onUploadProgress }: {
      file: File; remark?: string; staged?: boolean
      onUploadProgress?: (e: AxiosProgressEvent) => void
    }) => {
      if (useSessionStore.getState().isGuest) {
        const document = await saveGuestDocument(projectId, file, folderId, remark)
        return { ...document, duplicate: false }
      }
      const formData = new FormData()
      formData.append('file', file)
      formData.append('projectId', projectId)
      if (folderId) formData.append('folderId', folderId)
      if (remark) formData.append('remark', remark)
      if (staged) formData.append('staged', 'true')
      // 上传有逐文件进度条，不再触发全局加载指示（skipGlobalLoading），
      // 否则长时间上传会一直挂着全局 loading，观感像页面卡死
      return postMultipart<StagedUploadResult>(
        DOCUMENTS.upload, formData, { onUploadProgress, skipGlobalLoading: true },
      )
    },
    // staged 行不进默认列表；confirm 入库/丢弃时再失效
    onSuccess: (_data, vars) => {
      if (!vars.staged) qc.invalidateQueries({ queryKey: ['documents', projectId, folderId] })
    },
  })
}
