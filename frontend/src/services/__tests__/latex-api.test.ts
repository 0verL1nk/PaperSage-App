import { describe, it, expect, vi, beforeEach } from 'vitest'

// Mock react-pdf to avoid DOMMatrix errors in test env
vi.mock('react-pdf', () => ({
  pdfjs: {
    getDocument: vi.fn(),
    GlobalWorkerOptions: {},
    PDFDataRangeTransport: class PDFDataRangeTransport {},
    version: '5.4.296',
  },
}))

// Mock store for getAccessToken used in getPdfRangeSource
vi.mock('@/store', () => ({
  getAccessToken: vi.fn().mockReturnValue('test-token'),
}))

// Mock client module
vi.mock('../client', () => ({
  getJson: vi.fn(),
  postJson: vi.fn(),
  putJson: vi.fn(),
  deleteFetch: vi.fn(),
  downloadBlob: vi.fn(),
}))

// Mock JSZip — loadAsync is a static method on JSZip constructor
vi.mock('jszip', () => {
  const pdfFile = { dir: false, name: 'output.pdf', async: vi.fn().mockResolvedValue(new Blob(['pdf'], { type: 'application/pdf' })) }
  const logFile = { dir: false, name: 'output.log', async: vi.fn() }

  const MockJSZip = vi.fn().mockImplementation(() => ({
    files: {},
  })) as ReturnType<typeof vi.fn> & { loadAsync: ReturnType<typeof vi.fn>; support: Record<string, unknown> }
  MockJSZip.loadAsync = vi.fn().mockResolvedValue({
    files: { 'output.pdf': pdfFile, 'output.log': logFile },
  })
  MockJSZip.support = {}
  return { default: MockJSZip }
})

import { getJson, postJson, putJson, deleteFetch, downloadBlob } from '../client'
import {
  fetchLatexFiles,
  fetchLatexFileContent,
  saveLatexFile,
  createLatexFile,
  deleteLatexFile,
  compileLatex,
  getOutputZip,
  extractPdfFromZip,
} from '../latex-api'

const mockedGetJson = vi.mocked(getJson)
const mockedPostJson = vi.mocked(postJson)
const mockedPutJson = vi.mocked(putJson)
const mockedDeleteFetch = vi.mocked(deleteFetch)
const mockedDownloadBlob = vi.mocked(downloadBlob)

describe('latex-api', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  describe('fetchLatexFiles', () => {
    it('calls GET /api/v1/projects/{id}/latex-files', async () => {
      const mockFiles = [{ id: '1', file_path: 'main.tex', content_type: 'text/x-tex', size_bytes: 100, last_saved_at: '2024-01-01' }]
      mockedGetJson.mockResolvedValue({ files: mockFiles })

      const result = await fetchLatexFiles('proj-1')

      expect(mockedGetJson).toHaveBeenCalledWith('/api/v1/projects/proj-1/latex-files')
      expect(result).toEqual(mockFiles)
    })
  })

  describe('fetchLatexFileContent', () => {
    it('calls GET with encoded file path', async () => {
      const mockData = { path: 'sections/intro.tex', content: 'hello' }
      mockedGetJson.mockResolvedValue(mockData)

      const result = await fetchLatexFileContent('proj-1', 'sections/intro.tex')

      expect(mockedGetJson).toHaveBeenCalledWith('/api/v1/projects/proj-1/latex-files/sections%2Fintro.tex')
      expect(result).toEqual(mockData)
    })
  })

  describe('saveLatexFile', () => {
    it('calls PUT with content body', async () => {
      mockedPutJson.mockResolvedValue(undefined)

      await saveLatexFile('proj-1', 'main.tex', 'updated content')

      expect(mockedPutJson).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/latex-files/main.tex',
        { content: 'updated content' },
      )
    })
  })

  describe('createLatexFile', () => {
    it('calls POST with path and optional content', async () => {
      const mockResponse = { id: '2', file_path: 'new.tex', content_type: 'text/x-tex', size_bytes: 0, last_saved_at: '' }
      mockedPostJson.mockResolvedValue(mockResponse)

      const result = await createLatexFile('proj-1', 'new.tex', 'initial')

      expect(mockedPostJson).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/latex-files',
        { path: 'new.tex', content: 'initial' },
      )
      expect(result).toEqual(mockResponse)
    })

    it('works without content', async () => {
      mockedPostJson.mockResolvedValue({} as never)

      await createLatexFile('proj-1', 'empty.tex')

      expect(mockedPostJson).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/latex-files',
        { path: 'empty.tex', content: undefined },
      )
    })
  })

  describe('deleteLatexFile', () => {
    it('calls DELETE with encoded path', async () => {
      mockedDeleteFetch.mockResolvedValue(undefined)

      await deleteLatexFile('proj-1', 'old.tex')

      expect(mockedDeleteFetch).toHaveBeenCalledWith('/api/v1/projects/proj-1/latex-files/old.tex')
    })
  })

  describe('compileLatex', () => {
    it('calls POST compile with correct payload', async () => {
      const mockResponse = { status: 'success', build_id: 'build-1', output_files: [], error: null, logs: null }
      mockedPostJson.mockResolvedValue(mockResponse)

      const files = [{ path: 'main.tex', content: '\\documentclass{article}' }]
      const result = await compileLatex('proj-1', files, 'main.tex', 'xelatex')

      expect(mockedPostJson).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/latex/compile',
        { root_file: 'main.tex', compiler: 'xelatex', files },
      )
      expect(result).toEqual(mockResponse)
    })
  })

  describe('getOutputZip', () => {
    it('calls GET and returns blob', async () => {
      const mockBlob = new Blob(['zip-data'], { type: 'application/zip' })
      mockedDownloadBlob.mockResolvedValue(mockBlob)

      const result = await getOutputZip('proj-1', 'build-1')

      expect(mockedDownloadBlob).toHaveBeenCalledWith(
        '/api/v1/projects/proj-1/latex/output/build-1/output.zip',
      )
      expect(result).toBe(mockBlob)
    })
  })

  describe('extractPdfFromZip', () => {
    it('extracts PDF from zip and returns blob URL', async () => {
      const mockCreateObjectURL = vi.fn().mockReturnValue('blob:http://localhost/fake')
      vi.spyOn(URL, 'createObjectURL').mockImplementation(mockCreateObjectURL)

      const result = await extractPdfFromZip(new Blob(['zip']))

      expect(result).toBe('blob:http://localhost/fake')
      expect(mockCreateObjectURL).toHaveBeenCalled()
    })

    it('throws when no PDF found in zip', async () => {
      const JSZip = (await import('jszip')).default
      // Override static loadAsync to return zip with no PDF
      vi.mocked(JSZip.loadAsync).mockResolvedValueOnce({
        files: { 'output.log': { dir: false, name: 'output.log', async: vi.fn() } },
      } as never)

      await expect(extractPdfFromZip(new Blob(['zip']))).rejects.toThrow('No PDF file found in output zip')
    })
  })
})
