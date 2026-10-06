import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchLatexFiles,
  fetchLatexFileContent,
  saveLatexFile,
  createLatexFile,
  deleteLatexFile,
  deleteLatexFolder,
  renameLatexFile,
  renameLatexFolder,
  moveLatexFile,
  compileLatex,
  getPdfRangeSource,
  type CompileJobSubmitResponse,
  type PdfRangeSource,
} from '@/services/latex-api'
import { LATEX } from '@/services/api-routes'

// Query keys（定义在 @/lib/query-keys，供 AI 面板等复用而不引入重依赖）
import { latexKeys } from '@/lib/query-keys'
export { latexKeys }

// --- Queries ---

export function useLatexFiles(projectId: string) {
  return useQuery({
    queryKey: latexKeys.files(projectId),
    queryFn: () => fetchLatexFiles(projectId, { skipGlobalLoading: true }),
    enabled: !!projectId,
  })
}

export function useLatexFileContent(projectId: string, filePath: string | null) {
  return useQuery({
    queryKey: latexKeys.content(projectId, filePath ?? ''),
    queryFn: () => fetchLatexFileContent(projectId, filePath!, { skipGlobalLoading: true }),
    enabled: !!projectId && !!filePath,
  })
}

// --- Mutations ---

export function useSaveLatexFile(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ filePath, content }: { filePath: string; content: string }) =>
      saveLatexFile(projectId, filePath, content, { skipGlobalLoading: true }),
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({
        queryKey: latexKeys.content(projectId, variables.filePath),
      })
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useCreateLatexFile(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ path, content, file }: { path: string; content?: string; file?: File }) => {
      if (file) {
        const formData = new FormData()
        formData.append('file', file)
        formData.append('path', path)
        const { postMultipart } = await import('@/services/client')
        return postMultipart(LATEX.files(projectId), formData)
      }
      return createLatexFile(projectId, path, content)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

/**
 * 文件树变更类 mutation 的共享工厂：成功后失效文件列表缓存。
 * delete/rename/move 等操作都遵循同一失效语义，收敛在此避免重复。
 */
function useInvalidateFilesMutation<TVars>(
  projectId: string,
  mutationFn: (vars: TVars) => Promise<unknown>,
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useDeleteLatexFile(projectId: string) {
  return useInvalidateFilesMutation(projectId, (filePath: string) =>
    deleteLatexFile(projectId, filePath),
  )
}

export function useDeleteLatexFolder(projectId: string) {
  return useInvalidateFilesMutation(projectId, (folderPath: string) =>
    deleteLatexFolder(projectId, folderPath),
  )
}

export function useRenameLatexFile(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ oldPath, newPath }: { oldPath: string; newPath: string }) =>
      renameLatexFile(projectId, oldPath, newPath),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useRenameLatexFolder(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ oldFolder, newFolder }: { oldFolder: string; newFolder: string }) =>
      renameLatexFolder(projectId, oldFolder, newFolder),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useMoveLatexFile(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ filePath, newDir }: { filePath: string; newDir: string }) =>
      moveLatexFile(projectId, filePath, newDir),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useLatexCompile() {
  return useMutation<
    CompileJobSubmitResponse,
    Error,
    {
      projectId: string
      files: Array<{ path: string; content: string }>
      rootFile: string
      compiler: string
    }
  >({
    mutationFn: ({ projectId, files, rootFile, compiler }) =>
      compileLatex(projectId, files, rootFile, compiler),
  })
}

export function useLatexOutputPdf() {
  return useMutation<
    PdfRangeSource,
    Error,
    { projectId: string; buildId: string }
  >({
    mutationFn: ({ projectId, buildId }) => {
      return getPdfRangeSource(projectId, buildId)
    },
  })
}
