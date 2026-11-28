import JSZip from 'jszip'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { toast } from 'sonner'
import userEvent from '@testing-library/user-event'
import type { ReactNode } from 'react'

const mocks = vi.hoisted(() => ({
  autoSave: {
    setActiveFile: vi.fn(),
    applyServerContent: vi.fn(() => false),
    saveNow: vi.fn(async () => {}),
    onChange: vi.fn(),
    switchFile: vi.fn(),
  },
  saveWritingState: { mutate: vi.fn() },
  filesResult: { data: [] as unknown[], error: null, refetch: vi.fn() },
  createLatexFile: vi.fn(async () => ({})),
  uploadLatexFile: vi.fn(async () => ({})),
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ projectId: 'project-1' }),
}))
vi.mock('@/services/projects-api', () => ({ useProject: () => ({ data: { name: 'project-1' } }) }))
vi.mock('@/services/writing-state-api', () => ({
  useWritingState: () => ({ data: undefined }),
  useSaveWritingState: () => mocks.saveWritingState,
}))
vi.mock('@/hooks/use-latex', () => {
  const mutation = () => ({ mutate: vi.fn(), mutateAsync: vi.fn() })
  return {
    useLatexFiles: () => mocks.filesResult,
    useLatexFileContent: () => ({ data: undefined, isPending: false }),
    useSaveLatexFile: mutation,
    useCreateLatexFile: mutation,
    useDeleteLatexFile: mutation,
    useDeleteLatexFolder: mutation,
    useRenameLatexFile: mutation,
    useRenameLatexFolder: mutation,
    useMoveLatexFile: mutation,
    useLatexOutputPdf: mutation,
  }
})
vi.mock('@/hooks/use-auto-save', () => ({ useAutoSave: () => mocks.autoSave }))
// 本用例只测移动端焦点；建议 hook 需要 QueryClient，给它一个无副作用的替身。
vi.mock('@/hooks/use-writing-suggestion', () => ({
  useWritingSuggestion: () => ({
    pending: [], visible: [], pendingCount: 0, checking: false, notice: null,
    autoCheck: false, setAutoCheck: vi.fn(), checkNow: vi.fn(),
    notifyActivity: vi.fn(), notifyCursorMove: vi.fn(), refetch: vi.fn(),
    applyComment: vi.fn(), rejectComment: vi.fn(), undoComment: vi.fn(),
    isLoading: false, isError: false,
  }),
}))
vi.mock('@/lib/monaco-setup', () => ({}))
vi.mock('@monaco-editor/react', () => ({
  default: () => <div data-testid="monaco-editor" />,
  useMonaco: () => null,
}))
vi.mock('react-pdf', () => ({
  Document: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  Page: () => null,
  pdfjs: { GlobalWorkerOptions: {}, version: '4.0.0' },
}))
vi.mock('@/lib/pdf/pdf-js', () => ({ pdfDocumentOptions: {} }))
vi.mock('@/lib/monaco-latex-lsp', () => ({ registerLatexLsp: vi.fn() }))
vi.mock('@/app/theme-context', () => ({
  useTheme: () => ({ theme: 'light' }),
  resolveDark: () => false,
}))
vi.mock('@/components/ui/confirm-dialog', () => ({
  useConfirmDialog: () => ({ confirm: vi.fn(), ConfirmDialog: () => null }),
}))
vi.mock('@/components/domain/latex-environment-setup', () => ({ LatexEnvironmentSetup: () => null }))
vi.mock('@/lib/latex/swiftlatex-browser-compiler', () => ({ compileLatexInBrowser: vi.fn() }))
vi.mock('@/store/chat-store', () => ({
  useChatStore: (selector: (state: { pendingBrowserCompileRequest: null; consumeBrowserCompileRequest: () => void }) => unknown) =>
    selector({ pendingBrowserCompileRequest: null, consumeBrowserCompileRequest: vi.fn() }),
}))
vi.mock('@/services/latex-api', () => ({
  BROWSER_COMPILE_HEARTBEAT_MS: 60_000,
  createLatexFile: mocks.createLatexFile,
  uploadLatexFile: mocks.uploadLatexFile,
  cancelCompile: vi.fn(),
  getCompileHistory: vi.fn(),
  createBrowserCompileJob: vi.fn(),
  fetchLatexFileBytes: vi.fn(),
  fetchLatexFileContent: vi.fn(),
  reportBrowserCompileFailure: vi.fn(),
  sendBrowserCompileHeartbeat: vi.fn(),
  uploadBrowserCompileResult: vi.fn(),
}))
vi.mock('@/services/api-routes', () => ({
  LATEX: { outputZip: () => '/output.zip', fileContent: () => '/content' },
}))
vi.mock('@/services/client', () => ({ downloadBlob: vi.fn() }))
vi.mock('react-resizable-panels', () => ({
  Group: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  Panel: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  Separator: () => <div />,
  usePanelRef: () => ({ current: { expand: vi.fn() } }),
}))
vi.mock('../ai-panel', () => ({ AiPanel: () => <div data-testid="ai-panel" /> }))
// AI 面板顶部的已采用研究概要需要 QueryClient；本用例只测面板切换。
vi.mock('@/components/domain/AdoptedResearchSummary', () => ({ AdoptedResearchSummary: () => null }))
vi.mock('../writing-file-tree', () => ({
  WritingFileTree: () => (
    <div data-testid="file-tree">
      <button type="button">文件项</button>
      <input aria-label="重命名" />
    </div>
  ),
}))

import { WritingPage } from '../WritingPage'
import { prepareForUpdate } from '@/lib/update-prepare'
import { compileLatexInBrowser } from '@/lib/latex/swiftlatex-browser-compiler'
import { createBrowserCompileJob, fetchLatexFileContent, reportBrowserCompileFailure } from '@/services/latex-api'
import { useCompileStore } from '@/store/compile-store'

describe('WritingPage update preparation', () => {
  it('reports a Ctrl+S failure instead of leaving a rejected promise unhandled', async () => {
    mocks.autoSave.saveNow.mockClear().mockRejectedValueOnce(new Error('save failed'))
    const toastError = vi.spyOn(toast, 'error')
    const { unmount } = render(<WritingPage />)
    try {
      fireEvent.keyDown(window, { key: 's', ctrlKey: true })
      await waitFor(() => expect(toastError).toHaveBeenCalledWith('保存失败', { description: 'save failed' }))
    } finally {
      toastError.mockRestore()
      unmount()
    }
  })

  it('propagates save failures through the update bridge', async () => {
    mocks.autoSave.saveNow.mockClear().mockRejectedValueOnce(new Error('save failed'))
    const { unmount } = render(<WritingPage />)

    await expect(prepareForUpdate()).rejects.toThrow('save failed')
    expect(mocks.autoSave.saveNow).toHaveBeenCalledTimes(1)
    unmount()
    await expect(prepareForUpdate()).resolves.toBeUndefined()
  })

  it('registers its save promise while mounted and unregisters on unmount', () => {
    const pending = new Promise<void>(() => {})
    mocks.autoSave.saveNow.mockClear().mockReturnValueOnce(pending)
    const { unmount } = render(<WritingPage />)
    const waitUntil = vi.fn()

    window.dispatchEvent(new CustomEvent('papersage:prepare-update', { detail: { waitUntil } }))

    expect(mocks.autoSave.saveNow).toHaveBeenCalledTimes(1)
    expect(waitUntil).toHaveBeenCalledWith(pending)

    unmount()
    window.dispatchEvent(new CustomEvent('papersage:prepare-update', { detail: { waitUntil } }))
    expect(mocks.autoSave.saveNow).toHaveBeenCalledTimes(1)
    expect(waitUntil).toHaveBeenCalledTimes(1)
  })
})

describe('WritingPage mobile panes', () => {
  beforeEach(() => {
    window.innerWidth = 390
  })

  it('starts on the editor and switches panes from the bottom bar', async () => {
    const user = userEvent.setup()
    render(<WritingPage />)

    expect(screen.getByTestId('writing-pane-editor')).toHaveAttribute('aria-selected', 'true')
    expect(screen.queryByTestId('file-tree')).not.toBeInTheDocument()

    await user.click(screen.getByTestId('writing-pane-files'))
    expect(screen.getByTestId('writing-pane-files')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByTestId('file-tree')).toBeInTheDocument()

    await user.click(screen.getByTestId('writing-pane-ai'))
    expect(screen.getByTestId('writing-pane-ai')).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByTestId('ai-panel')).toBeInTheDocument()

    await user.click(screen.getByTestId('writing-pane-preview'))
    expect(screen.getByTestId('writing-pane-preview')).toHaveAttribute('aria-selected', 'true')
  })

  it('moves between panes with the arrow keys', async () => {
    const user = userEvent.setup()
    render(<WritingPage />)

    const editor = screen.getByTestId('writing-pane-editor')
    editor.focus()
    await user.keyboard('{ArrowRight}')
    expect(screen.getByTestId('writing-pane-preview')).toHaveAttribute('aria-selected', 'true')

    editor.focus()
    await user.keyboard('{ArrowLeft}')
    expect(screen.getByTestId('writing-pane-files')).toHaveAttribute('aria-selected', 'true')
  })
})

describe('WritingPage compile recovery', () => {
  beforeEach(() => {
    window.innerWidth = 1024
    mocks.filesResult.data = [{
      id: 'file-1',
      file_path: 'main.tex',
      content_type: 'text/x-tex',
      size_bytes: 24,
      last_saved_at: '2026-01-01T00:00:00Z',
    }]
    vi.mocked(createBrowserCompileJob).mockResolvedValue({ job_id: 'job-1', status: 'running' })
    vi.mocked(fetchLatexFileContent).mockResolvedValue({ path: 'main.tex', content: '\\documentclass{article}' })
    vi.mocked(reportBrowserCompileFailure).mockResolvedValue({ job_id: 'job-1', status: 'failed' })
  })

  afterEach(() => {
    mocks.filesResult.data = []
    vi.mocked(createBrowserCompileJob).mockReset()
    vi.mocked(compileLatexInBrowser).mockReset()
    vi.mocked(reportBrowserCompileFailure).mockReset()
    useCompileStore.getState().reset()
  })

  it('leaves 编译中 and offers a retry when the browser engine times out', async () => {
    vi.mocked(compileLatexInBrowser).mockRejectedValue(
      new Error('pdfLaTeX 浏览器引擎初始化超时：超过 2 分钟仍未就绪，已停止本次编译。'),
    )
    const { unmount } = render(<WritingPage />)

    fireEvent.click(screen.getByTestId('compile-button'))

    await waitFor(() => expect(screen.getByTestId('retry-browser-compile')).toBeInTheDocument())
    expect(screen.getByTestId('compile-button')).toBeEnabled()
    expect(screen.getByText(/编译超过预期时间仍未完成/)).toBeInTheDocument()
    await waitFor(() => expect(reportBrowserCompileFailure).toHaveBeenCalledWith(
      'project-1', 'job-1', expect.stringContaining('初始化超时'), expect.any(String),
    ))
    unmount()
  })

  it('手机端编译后自动切到预览进度，失败就地显示原因', async () => {
    window.innerWidth = 390
    vi.mocked(compileLatexInBrowser).mockRejectedValue(
      new Error('pdfLaTeX 浏览器引擎初始化超时：超过 2 分钟仍未就绪，已停止本次编译。'),
    )
    const toastError = vi.spyOn(toast, 'error')
    const { unmount } = render(<WritingPage />)
    try {
      await waitFor(() => expect(screen.getByTestId('writing-pane-preview')).toBeInTheDocument())
      fireEvent.click(screen.getByTestId('compile-button'))

      // 编译一开始就停在预览页，用户立即看到进度而不是留在编辑页猜。
      await waitFor(() => expect(screen.getByTestId('writing-pane-preview')).toHaveAttribute('aria-selected', 'true'))
      await waitFor(() => expect(screen.getByTestId('retry-browser-compile')).toBeInTheDocument())
      expect(screen.getByText(/编译超过预期时间仍未完成/)).toBeInTheDocument()
      // 失败卡片已在当前预览页可见，不再弹「查看原因」提示。
      expect(toastError).not.toHaveBeenCalledWith('编译失败', expect.anything())
    } finally {
      toastError.mockRestore()
      unmount()
    }
  })
})

describe('WritingPage LaTeX project import', () => {
  beforeEach(() => {
    window.innerWidth = 1024
    mocks.createLatexFile.mockClear()
    mocks.uploadLatexFile.mockClear()
    mocks.filesResult.refetch.mockClear()
  })

  it('unpacks an uploaded ZIP into nested project paths', async () => {
    const zip = new JSZip()
    zip.file('thesis.tex', '\\documentclass{book}')
    zip.file('body/ch1_intro.tex', 'chapter one')
    const file = new File([await zip.generateAsync({ type: 'arraybuffer' })], 'hithesis.zip', {
      type: 'application/zip',
    })

    render(<WritingPage />)
    fireEvent.change(screen.getByLabelText('上传文件或导入 ZIP 工程', { selector: 'input' }), {
      target: { files: [file] },
    })

    await waitFor(() => expect(mocks.filesResult.refetch).toHaveBeenCalled())
    expect(mocks.createLatexFile).toHaveBeenCalledWith('project-1', 'thesis.tex', '\\documentclass{book}')
    expect(mocks.createLatexFile).toHaveBeenCalledWith('project-1', 'body/ch1_intro.tex', 'chapter one')
    expect(mocks.createLatexFile).toHaveBeenCalledTimes(2)
    expect(mocks.uploadLatexFile).not.toHaveBeenCalled()
  })
})
