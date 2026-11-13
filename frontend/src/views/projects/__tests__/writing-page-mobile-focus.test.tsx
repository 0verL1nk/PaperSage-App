import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
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
vi.mock('@/hooks/use-compile-polling', () => ({
  useCompileStatusPolling: () => ({ data: undefined }),
  isTerminalStatus: () => true,
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
  cancelCompile: vi.fn(),
  getCompileHistory: vi.fn(),
  createBrowserCompileJob: vi.fn(),
  fetchLatexFileBytes: vi.fn(),
  fetchLatexFileContent: vi.fn(),
  reportBrowserCompileFailure: vi.fn(),
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
vi.mock('../writing-file-tree', () => ({
  WritingFileTree: () => (
    <div data-testid="file-tree">
      <button type="button">文件项</button>
      <input aria-label="重命名" />
    </div>
  ),
}))

import { WritingPage } from '../WritingPage'

describe('WritingPage mobile sidebar focus', () => {
  beforeEach(() => {
    window.innerWidth = 390
  })

  it('moves focus into the panel, closes on Escape, and returns focus to the trigger', async () => {
    const user = userEvent.setup()
    render(<WritingPage />)

    const trigger = screen.getByTestId('writing-tab-files')
    await user.click(trigger)

    expect(screen.getByRole('dialog', { name: '文件侧栏' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '关闭侧栏' })).toHaveFocus()

    await user.keyboard('{Escape}')

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    expect(trigger).toHaveFocus()
  })

  it('traps Tab and Shift+Tab inside the panel', async () => {
    const user = userEvent.setup()
    render(<WritingPage />)
    await user.click(screen.getByTestId('writing-tab-files'))

    const close = screen.getByRole('button', { name: '关闭侧栏' })
    const last = screen.getByLabelText('重命名')
    expect(close).toHaveFocus()

    last.focus()
    await user.tab()
    expect(close).toHaveFocus()

    await user.tab({ shift: true })
    expect(last).toHaveFocus()
  })

  it('does not close the panel when Escape is handled by an inline editor', async () => {
    const user = userEvent.setup()
    render(<WritingPage />)
    await user.click(screen.getByTestId('writing-tab-files'))

    screen.getByLabelText('重命名').focus()
    await user.keyboard('{Escape}')

    expect(screen.getByRole('dialog', { name: '文件侧栏' })).toBeInTheDocument()
  })
})
