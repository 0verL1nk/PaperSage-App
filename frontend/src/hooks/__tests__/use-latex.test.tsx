import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'

// Mock the latex-api module
vi.mock('@/services/latex-api', () => ({
  fetchLatexFiles: vi.fn(),
  fetchLatexFileContent: vi.fn(),
  saveLatexFile: vi.fn(),
  createLatexFile: vi.fn(),
  deleteLatexFile: vi.fn(),
  compileLatex: vi.fn(),
  getPdfRangeSource: vi.fn(),
}))

// Mock react-pdf to avoid DOMMatrix errors in test env
vi.mock('react-pdf', () => ({
  pdfjs: {
    getDocument: vi.fn(),
    GlobalWorkerOptions: {},
    PDFDataRangeTransport: class PDFDataRangeTransport {},
    version: '5.4.296',
  },
}))

import {
  useLatexFiles,
  useLatexFileContent,
  useSaveLatexFile,
  useCreateLatexFile,
  useDeleteLatexFile,
  useLatexCompile,
  useLatexOutputPdf,
} from '../use-latex'
import {
  fetchLatexFiles,
  fetchLatexFileContent,
  saveLatexFile,
  createLatexFile,
  deleteLatexFile,
  compileLatex,
  getPdfRangeSource,
} from '@/services/latex-api'

const mockedFetchLatexFiles = vi.mocked(fetchLatexFiles)
const mockedFetchLatexFileContent = vi.mocked(fetchLatexFileContent)
const mockedSaveLatexFile = vi.mocked(saveLatexFile)
const mockedCreateLatexFile = vi.mocked(createLatexFile)
const mockedDeleteLatexFile = vi.mocked(deleteLatexFile)
const mockedCompileLatex = vi.mocked(compileLatex)
const mockedGetPdfRangeSource = vi.mocked(getPdfRangeSource)

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return function Wrapper({ children }: { children: ReactNode }) {
    return createElement(QueryClientProvider, { client: queryClient }, children)
  }
}

const mockFileList = [
  { id: '1', file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 100, last_saved_at: '2024-01-01' },
  { id: '2', file_path: 'sections/intro.tex', content_type: 'text/x-tex', size_bytes: 50, last_saved_at: '2024-01-01' },
]

describe('use-latex hooks', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('useLatexFiles', () => {
    it('fetches file list for a project', async () => {
      mockedFetchLatexFiles.mockResolvedValue(mockFileList)
      const { result } = renderHook(() => useLatexFiles('proj-1'), { wrapper: createWrapper() })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data).toEqual(mockFileList)
      expect(mockedFetchLatexFiles).toHaveBeenCalledWith('proj-1', { skipGlobalLoading: true })
    })

    it('is disabled when projectId is empty', () => {
      const { result } = renderHook(() => useLatexFiles(''), { wrapper: createWrapper() })
      expect(result.current.fetchStatus).toBe('idle')
    })
  })

  describe('useLatexFileContent', () => {
    it('fetches content when both projectId and filePath are provided', async () => {
      const mockContent = { path: 'main.tex', content: '\\documentclass{article}' }
      mockedFetchLatexFileContent.mockResolvedValue(mockContent)

      const { result } = renderHook(
        () => useLatexFileContent('proj-1', 'main.tex'),
        { wrapper: createWrapper() },
      )

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(result.current.data).toEqual(mockContent)
    })

    it('is disabled when filePath is null', () => {
      const { result } = renderHook(
        () => useLatexFileContent('proj-1', null),
        { wrapper: createWrapper() },
      )
      expect(result.current.fetchStatus).toBe('idle')
    })
  })

  describe('useSaveLatexFile', () => {
    it('calls saveLatexFile and invalidates queries', async () => {
      mockedSaveLatexFile.mockResolvedValue(undefined)

      const { result } = renderHook(() => useSaveLatexFile('proj-1'), { wrapper: createWrapper() })

      result.current.mutate({ filePath: 'main.tex', content: 'new content' })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockedSaveLatexFile).toHaveBeenCalledWith('proj-1', 'main.tex', 'new content', { skipGlobalLoading: true })
    })
  })

  describe('useCreateLatexFile', () => {
    it('calls createLatexFile', async () => {
      const mockCreated = { id: '3', file_path: 'new.tex', content_type: 'text/x-tex', size_bytes: 0, last_saved_at: '' }
      mockedCreateLatexFile.mockResolvedValue(mockCreated)

      const { result } = renderHook(() => useCreateLatexFile('proj-1'), { wrapper: createWrapper() })

      result.current.mutate({ path: 'new.tex', content: '' })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockedCreateLatexFile).toHaveBeenCalledWith('proj-1', 'new.tex', '')
    })
  })

  describe('useDeleteLatexFile', () => {
    it('calls deleteLatexFile', async () => {
      mockedDeleteLatexFile.mockResolvedValue(undefined)

      const { result } = renderHook(() => useDeleteLatexFile('proj-1'), { wrapper: createWrapper() })

      result.current.mutate('old.tex')

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockedDeleteLatexFile).toHaveBeenCalledWith('proj-1', 'old.tex')
    })
  })

  describe('useLatexCompile', () => {
    it('calls compileLatex and returns job submission', async () => {
      const mockResponse = { job_id: 'job-123', status: 'pending' as const }
      mockedCompileLatex.mockResolvedValue(mockResponse)

      const { result } = renderHook(() => useLatexCompile(), { wrapper: createWrapper() })

      result.current.mutate({
        projectId: 'proj-1',
        files: [{ path: 'main.tex', content: 'content' }],
        rootFile: 'main.tex',
        compiler: 'xelatex',
      })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockedCompileLatex).toHaveBeenCalledWith('proj-1', [{ path: 'main.tex', content: 'content' }], 'main.tex', 'xelatex')
      expect(result.current.data).toEqual(mockResponse)
    })
  })

  describe('useLatexOutputPdf', () => {
    it('creates range transport source for PDF', async () => {
      const mockRangeSource = { range: {} as unknown }
      mockedGetPdfRangeSource.mockResolvedValue(mockRangeSource)

      const { result } = renderHook(() => useLatexOutputPdf(), { wrapper: createWrapper() })

      result.current.mutate({ projectId: 'proj-1', buildId: 'b1' })

      await waitFor(() => expect(result.current.isSuccess).toBe(true))
      expect(mockedGetPdfRangeSource).toHaveBeenCalledWith('proj-1', 'b1')
      expect(result.current.data).toEqual(mockRangeSource)
    })
  })
})
