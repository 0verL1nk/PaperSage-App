import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  fetchLatexFiles,
  fetchLatexFileContent,
  saveLatexFile,
  createLatexFile,
  deleteLatexFile,
  compileLatex,
  getPdfRangeSource,
  type CompileResponse,
  type PdfRangeSource,
} from '@/services/latex-api'

// Query keys
const latexKeys = {
  files: (projectId: string) => ['latex-files', projectId] as const,
  content: (projectId: string, filePath: string) =>
    ['latex-file-content', projectId, filePath] as const,
}

// --- Queries ---

export function useLatexFiles(projectId: string) {
  return useQuery({
    queryKey: latexKeys.files(projectId),
    queryFn: () => fetchLatexFiles(projectId),
    enabled: !!projectId,
  })
}

export function useLatexFileContent(projectId: string, filePath: string | null) {
  return useQuery({
    queryKey: latexKeys.content(projectId, filePath ?? ''),
    queryFn: () => fetchLatexFileContent(projectId, filePath!),
    enabled: !!projectId && !!filePath,
  })
}

// --- Mutations ---

export function useSaveLatexFile(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ filePath, content }: { filePath: string; content: string }) =>
      saveLatexFile(projectId, filePath, content),
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
    mutationFn: ({ path, content }: { path: string; content?: string }) =>
      createLatexFile(projectId, path, content),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useDeleteLatexFile(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (filePath: string) => deleteLatexFile(projectId, filePath),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: latexKeys.files(projectId) })
    },
  })
}

export function useLatexCompile() {
  return useMutation<
    CompileResponse,
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
