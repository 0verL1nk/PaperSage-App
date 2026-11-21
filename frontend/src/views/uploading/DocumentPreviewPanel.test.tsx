import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { appEnv, setApiTarget, setCustomApiBaseUrl, setDesktopLocalApiBaseUrl, setDesktopLocalBackendError } from '@/lib/env'
import { apiClient } from '@/services/client'
import { useSessionStore } from '@/store'
import type { ProjectDocument } from '@/types/files'
import { DocumentPreviewPanel } from './DocumentPreviewPanel'

const { pdfDocument, fetchGuestPreview } = vi.hoisted(() => ({
  pdfDocument: vi.fn(),
  fetchGuestPreview: vi.fn(),
}))

vi.mock('react-pdf', () => ({
  Document: ({ file }: { file: { url: string } }) => {
    pdfDocument(file.url)
    return <div data-testid="pdf-preview" />
  },
  Page: () => null,
  pdfjs: { GlobalWorkerOptions: {}, version: '4.0.0' },
}))
vi.mock('@/services/files-api', () => ({ fetchDocumentPreviewBlob: fetchGuestPreview }))

const pdf: ProjectDocument = {
  id: 'doc-1', name: 'paper.pdf', type: 'application/pdf', size: 10, status: 'ready',
}
const backendError = '本地后端版本不匹配'

describe('document preview backend selection', () => {
  beforeEach(() => {
    localStorage.clear()
    useSessionStore.setState({ isGuest: false })
    pdfDocument.mockClear()
    fetchGuestPreview.mockReset()
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect() {} })
  })

  afterEach(() => {
    setDesktopLocalApiBaseUrl(appEnv.localApiBaseUrl)
    setDesktopLocalApiBaseUrl('')
    useSessionStore.setState({ isGuest: false })
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('shows the local failure instead of constructing a PDF request', () => {
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)

    render(<DocumentPreviewPanel document={pdf} onClose={() => {}} />)

    expect(pdfDocument).not.toHaveBeenCalled()
    expect(screen.getByText(`本地后端不可用：${backendError}`)).toBeInTheDocument()
    expect(screen.queryByText('读取文件中')).not.toBeInTheDocument()
  })

  it.each([
    'text/plain',
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  ])('does not start a %s preview when the local backend failed', (type) => {
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)
    const request = vi.spyOn(apiClient, 'get')

    render(<DocumentPreviewPanel document={{ ...pdf, type }} onClose={() => {}} />)

    expect(request).not.toHaveBeenCalled()
    expect(screen.getByText(`本地后端不可用：${backendError}`)).toBeInTheDocument()
  })

  it('still uses the same-origin URL for the explicit online target', () => {
    setDesktopLocalBackendError(backendError)
    setApiTarget('online')

    render(<DocumentPreviewPanel document={pdf} onClose={() => {}} />)

    expect(pdfDocument).toHaveBeenCalledWith(`${appEnv.apiBaseUrl}/api/v1/documents/doc-1/preview`)
  })

  it('still uses the explicit custom target after a local failure', () => {
    setDesktopLocalBackendError(backendError)
    setCustomApiBaseUrl('https://custom.example.test/')
    setApiTarget('custom')

    render(<DocumentPreviewPanel document={pdf} onClose={() => {}} />)

    expect(pdfDocument).toHaveBeenCalledWith('https://custom.example.test/api/v1/documents/doc-1/preview')
  })

  it('keeps the browser-local development URL', () => {
    setApiTarget('local')

    render(<DocumentPreviewPanel document={pdf} onClose={() => {}} />)

    expect(pdfDocument).toHaveBeenCalledWith(`${appEnv.localApiBaseUrl}/api/v1/documents/doc-1/preview`)
  })

  it('keeps browser-local guest previews available after a native failure', async () => {
    setApiTarget('local')
    setDesktopLocalBackendError(backendError)
    useSessionStore.setState({ isGuest: true })
    fetchGuestPreview.mockResolvedValue(new Blob(['pdf']))
    const OriginalURL = URL
    vi.stubGlobal('URL', class extends OriginalURL {
      static createObjectURL = () => 'blob:guest-document'
      static revokeObjectURL = () => {}
    })

    render(<DocumentPreviewPanel document={pdf} onClose={() => {}} />)

    expect(await screen.findByTestId('pdf-preview')).toBeInTheDocument()
    expect(pdfDocument).toHaveBeenCalledWith('blob:guest-document')
    expect(screen.queryByText(backendError)).not.toBeInTheDocument()
  })
})
