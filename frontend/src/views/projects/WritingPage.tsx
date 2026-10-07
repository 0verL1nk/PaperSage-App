import { cn } from '@/lib/utils'
import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams } from '@tanstack/react-router'
import { useSaveWritingState, useWritingState } from '@/services/writing-state-api'
import {
  Search, Play, Download, Sparkles, Loader2,
  Plus, Upload, Files, Monitor, ZoomIn, ZoomOut, Maximize, AlertCircle,
  Check, Copy, Cloud, CloudUpload, FolderPlus,
  HardDrive,
} from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import Editor, { useMonaco } from '@monaco-editor/react'
import { Document, Page, pdfjs } from 'react-pdf'
import * as Resizable from 'react-resizable-panels'
import { registerLatexLsp } from '@/lib/monaco-latex-lsp'
import { useTheme } from '@/app/theme-context'
import { resolveDark } from '@/app/theme-context'
import { useConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  useLatexFiles, useLatexFileContent, useSaveLatexFile,
  useCreateLatexFile, useDeleteLatexFile,
  useDeleteLatexFolder, useRenameLatexFile, useRenameLatexFolder,
  useMoveLatexFile, useLatexCompile, useLatexOutputPdf,
} from '@/hooks/use-latex'
import { useCompileStatusPolling, isTerminalStatus } from '@/hooks/use-compile-polling'
import { useAutoSave } from '@/hooks/use-auto-save'
import type { PdfRangeSource, CompileJobStatus, LatexFileItem } from '@/services/latex-api'
import { LATEX } from '@/services/api-routes'
import { LoadingSpinner } from '@/components/shared/loading-spinner'
import { downloadBlob } from '@/services/client'
import { AiPanel } from './ai-panel'
import { useChatStore } from '@/store/chat-store'
import { useSessionStore } from '@/store'

function toPdfFile(source: PdfRangeSource) {
  return source as Parameters<typeof Document>[0]['file']
}
import { WritingFileTree, type FileTreeHandle } from './writing-file-tree'

pdfjs.GlobalWorkerOptions.workerSrc = `https://unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`
import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'
import JSZip from 'jszip'

// --- Constants ---

const ZOOM_RENDER_DEBOUNCE_MS = 400
const ZOOM_MIN = 0.4
const ZOOM_MAX = 3.0
const ZOOM_STEP = 0.15
const ZOOM_WHEEL_FACTOR_UP = 1.08
const ZOOM_WHEEL_FACTOR_DOWN = 0.92

const COMPILER_OPTIONS = [
  { value: 'pdflatex', label: 'pdfLaTeX' },
  { value: 'xelatex', label: 'XeLaTeX' },
  { value: 'lualatex', label: 'LuaLaTeX' },
] as const

const XELATEX_PACKAGES = ['ctex', 'fontspec', 'xeCJK', 'polyglossia']
const XELATEX_RE = new RegExp(`\\\\usepackage(?:\\[[^\\]]*\\])?\\{(?:${XELATEX_PACKAGES.join('|')})\\}`)
// % !TEX TS-program = xelatex  or  % !TEX program = xelatex
const TEX_MAGIC_RE = /^%\s*!TEX\s+(?:TS-)?program\s*=\s*(\S+)/mi

function detectCompiler(content: string): string {
  const magic = TEX_MAGIC_RE.exec(content)
  if (magic) {
    const prog = magic[1].toLowerCase()
    if (['pdflatex', 'xelatex', 'lualatex'].includes(prog)) return prog
  }
  return XELATEX_RE.test(content) ? 'xelatex' : 'pdflatex'
}

function getLanguageByExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (['tex', 'cls', 'sty', 'bst', 'def', 'lof', 'lot', 'toc'].includes(ext)) return 'latex'
  if (ext === 'bib') return 'bibtex'
  if (ext === 'md') return 'markdown'
  return 'plaintext'
}

function isEditableFile(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ['tex', 'bib', 'cls', 'sty', 'bst', 'txt', 'md', 'lof', 'lot', 'toc'].includes(ext)
}

function isImageFile(path: string): boolean {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp'].includes(ext)
}

function isPdfFile(path: string): boolean {
  return path.toLowerCase().endsWith('.pdf')
}

type SidebarTab = 'files' | 'ai'

// --- Main Component ---

export function WritingPage() {
  const { projectId = '' } = useParams({ strict: false })
  const { data: writingState } = useWritingState(projectId)
  const saveWritingState = useSaveWritingState(projectId)
  const phaseSyncStarted = useRef(false)

  useEffect(() => {
    if (!writingState || phaseSyncStarted.current) return
    phaseSyncStarted.current = true
    if (writingState.current_phase === 'write') return
    saveWritingState.mutate({
      brief: writingState.brief,
      requirements: writingState.requirements,
      phases: writingState.phases,
      current_phase: 'write',
      revision: writingState.revision,
    })
  }, [writingState, saveWritingState])

  // UI state
  const [activeTab, setActiveTab] = useState<SidebarTab>('files')
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null)
  const [compiler, setCompiler] = useState('pdflatex')
  const [numPages, setNumPages] = useState(0)
  const [displayScale, setDisplayScale] = useState(0.9)
  const [renderScale, setRenderScale] = useState(0.9)
  const [isMobile, setIsMobile] = useState(false)
  const [editorContent, setEditorContent] = useState('')

  // Refs
  const fileTreeRef = useRef<FileTreeHandle>(null)
  const previewContainerRef = useRef<HTMLDivElement>(null)
  const uploadInputRef = useRef<HTMLInputElement>(null)
  const renderDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const activeFilePathRef = useRef<string | null>(null)

  const monaco = useMonaco()
  // 编辑器主题跟随应用主题（Monaco 不吃 CSS token，需显式切换）
  const { theme } = useTheme()
  const [editorDark, setEditorDark] = useState(() => resolveDark(theme))
  useEffect(() => {
    setEditorDark(resolveDark(theme))
    if (theme !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const on = () => setEditorDark(mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  }, [theme])
  const { confirm, ConfirmDialog: DeleteConfirmDialog } = useConfirmDialog()

  // Data hooks
  const { data: files = [] } = useLatexFiles(projectId)
  const isEditable = activeFilePath ? isEditableFile(activeFilePath) : false
  const { data: fileContent, isPending: isContentLoading } = useLatexFileContent(projectId, isEditable ? activeFilePath : null)
  const saveMutation = useSaveLatexFile(projectId)
  const autoSave = useAutoSave(saveMutation)
  const createMutation = useCreateLatexFile(projectId)
  const deleteMutation = useDeleteLatexFile(projectId)
  const deleteFolderMutation = useDeleteLatexFolder(projectId)
  const renameMutation = useRenameLatexFile(projectId)
  const renameFolderMutation = useRenameLatexFolder(projectId)
  const moveMutation = useMoveLatexFile(projectId)
  const compileMutation = useLatexCompile()
  const outputPdfMutation = useLatexOutputPdf()

  // Async compile state
  const [compileJobId, setCompileJobId] = useState<string | null>(null)
  const compileStatus = useCompileStatusPolling(projectId, compileJobId)

  // 编译联动：助手调用 compile_project 后把 job id 交接过来，由这里启动轮询刷新预览。
  // 没有这一步，助手触发的编译结果只能靠用户手动再点一次编译才看得到。
  const pendingCompileJobId = useChatStore((s) => s.pendingCompileJobId)
  const consumeCompileJobId = useChatStore((s) => s.consumeCompileJobId)
  useEffect(() => {
    if (!pendingCompileJobId) return
    const id = consumeCompileJobId()
    if (id) setCompileJobId(id)
  }, [pendingCompileJobId, consumeCompileJobId])

  // Derived state
  const [pdfFileSource, setPdfFileSource] = useState<PdfRangeSource | null>(null)
  const [pdfBuildId, setPdfBuildId] = useState<string | null>(null)
  const [isDownloadingPdf, setIsDownloadingPdf] = useState(false)
  const [compileError, setCompileError] = useState<string | null>(null)
  const [compileLogs, setCompileLogs] = useState<string | null>(null)
  const [showFullLogs, setShowFullLogs] = useState(false)

  const isCompiling = compileMutation.isPending
    || (compileJobId !== null && compileStatus.data !== undefined && !isTerminalStatus(compileStatus.data.status))

  // React to terminal compile status — using ref to avoid setState-in-effect lint
  const prevJobIdRef = useRef<string | null>(null)
  useEffect(() => {
    const job = compileStatus.data
    if (!job || !isTerminalStatus(job.status as CompileJobStatus)) return
    // Only process each job once
    if (prevJobIdRef.current === job.job_id) return
    prevJobIdRef.current = job.job_id

    if (job.status === 'success' && job.build_id) {
      setCompileError(null)
      setCompileLogs(null)
      outputPdfMutation.mutate(
        { projectId, buildId: job.build_id },
        {
          onSuccess: (source) => {
            setPdfFileSource(source)
            setPdfBuildId(job.build_id!)
          },
        },
      )
    } else if (job.status === 'failed' || job.status === 'timeout') {
      setCompileError(job.error || (job.status === 'timeout' ? '编译超时' : '编译失败'))
      if (job.logs) setCompileLogs(job.logs)
    }

    setCompileJobId(null)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [compileStatus.data?.status, compileStatus.data?.job_id])

  // Monaco LaTeX LSP
  useEffect(() => {
    if (monaco) registerLatexLsp(monaco)
  }, [monaco])

  // Auto-select first file or main.tex
  const initializedRef = useRef(false)
  useEffect(() => {
    if (!initializedRef.current && files.length > 0) {
      initializedRef.current = true
      const mainFile = files.find((f) => f.file_path === 'main.tex')
      const target = mainFile?.file_path || files[0].file_path
      activeFilePathRef.current = target
      autoSave.setActiveFile(target)
      setActiveFilePath(target)
    }
  }, [files, autoSave])

  // Mobile detection
  useEffect(() => {
    const check = () => setIsMobile(window.innerWidth < 1024)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  // Sync editor with loaded file content (skip if user has local edits)
  useEffect(() => {
    if (fileContent?.content !== undefined) {
      const applied = autoSave.applyServerContent(fileContent.content)
      if (applied) {
        queueMicrotask(() => {
          setEditorContent(fileContent.content)
          if (activeFilePathRef.current === 'main.tex') {
            setCompiler(detectCompiler(fileContent.content))
          }
        })
      }
    }
  }, [fileContent, autoSave])

  // Debounced render scale sync
  useEffect(() => {
    if (renderDebounceRef.current) clearTimeout(renderDebounceRef.current)
    renderDebounceRef.current = setTimeout(() => setRenderScale(displayScale), ZOOM_RENDER_DEBOUNCE_MS)
    return () => { if (renderDebounceRef.current) clearTimeout(renderDebounceRef.current) }
  }, [displayScale])

  // Ctrl+Wheel zoom
  useEffect(() => {
    const container = previewContainerRef.current
    if (!container) return
    const handleWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault()
        const factor = -e.deltaY > 0 ? ZOOM_WHEEL_FACTOR_UP : ZOOM_WHEEL_FACTOR_DOWN
        setDisplayScale((prev) => Math.max(ZOOM_MIN, Math.min(prev * factor, ZOOM_MAX)))
      }
    }
    container.addEventListener('wheel', handleWheel, { passive: false })
    return () => container.removeEventListener('wheel', handleWheel)
  }, [])

  // Ctrl+S / Cmd+S interception
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault()
        autoSave.saveNow()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [autoSave])

  const handleEditorChange = (value: string | undefined) => {
    if (value === undefined) return
    autoSave.onChange(value)
  }

  const handleSelectFile = (filePath: string) => {
    autoSave.switchFile(filePath)
    activeFilePathRef.current = filePath
    setActiveFilePath(filePath)
    setEditorContent('')
  }

  const handleDownloadPdf = useCallback(async () => {
    if (!pdfBuildId || isDownloadingPdf) return
    setIsDownloadingPdf(true)
    try {
      const blob = await downloadBlob(LATEX.outputZip(projectId, pdfBuildId))
      const zip = await JSZip.loadAsync(blob)
      const pdfEntry = Object.values(zip.files).find(
        (f) => !f.dir && f.name.toLowerCase().endsWith('.pdf'),
      )
      if (!pdfEntry) {
        toast.error('未找到 PDF 文件')
        return
      }
      const pdfBlob = await pdfEntry.async('blob')
      const url = URL.createObjectURL(pdfBlob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'output.pdf'
      a.click()
      URL.revokeObjectURL(url)
    } catch {
      toast.error('下载失败')
    } finally {
      setIsDownloadingPdf(false)
    }
  }, [pdfBuildId, isDownloadingPdf, projectId])

  const handleCompile = async () => {
    setShowFullLogs(false)
    try {
      // Auto-save current file before compiling
      await autoSave.saveNow()

      const rootFile = files.find((f) => f.file_path === 'main.tex')?.file_path
        || activeFilePath
        || files[0]?.file_path

      if (!rootFile) return

      // Submit async compile job — backend loads files from storage
      const result = await compileMutation.mutateAsync({
        projectId, files: [], rootFile, compiler,
      })
      setCompileJobId(result.job_id)
    } catch (err) {
      setCompileError(err instanceof Error ? err.message : '编译失败')
    }
  }

  const handleTreeCreateFile = async (path: string, content?: string): Promise<LatexFileItem> => {
    const result: unknown = await createMutation.mutateAsync({ path, content: content ?? '' })
    setActiveFilePath(path)
    return result as LatexFileItem
  }

  const handleTreeRenameFile = async (oldPath: string, newPath: string) => {
    return renameMutation.mutateAsync({ oldPath, newPath })
  }

  const handleTreeRenameFolder = async (oldFolder: string, newFolder: string) => {
    return renameFolderMutation.mutateAsync({ oldFolder, newFolder })
  }

  const handleTreeMoveFile = async (filePath: string, newDir: string) => {
    return moveMutation.mutateAsync({ filePath, newDir })
  }

  const handleDeleteFile = (filePath: string) => {
    confirm({
      title: '删除文件',
      description: `确定删除 "${filePath}" 吗？此操作不可撤销。`,
      confirmText: '删除',
      variant: 'destructive',
      onConfirm: () => {
        deleteMutation.mutate(filePath, {
          onSuccess: () => {
            if (activeFilePath === filePath) {
              const dir = filePath.includes('/') ? filePath.substring(0, filePath.lastIndexOf('/')) : ''
              const sibling = files.find((f) => {
                if (f.file_path === filePath) return false
                if (!dir) return !f.file_path.includes('/')
                return f.file_path.startsWith(dir + '/') && !f.file_path.substring(dir.length + 1).includes('/')
              })
              const fallback = files.find((f) => f.file_path === 'main.tex' && f.file_path !== filePath)
              const next = sibling?.file_path ?? fallback?.file_path ?? null
              setActiveFilePath(next)
              activeFilePathRef.current = next
            }
          },
        })
      },
    })
  }

  const handleDeleteFolder = (folderPath: string) => {
    confirm({
      title: '删除文件夹',
      description: `确定删除文件夹 "${folderPath}" 吗？该文件夹下所有文件及子文件夹将被永久删除。`,
      confirmText: '删除',
      variant: 'destructive',
      onConfirm: () => {
        deleteFolderMutation.mutate(folderPath, {
          onSuccess: () => {
            if (activeFilePath?.startsWith(folderPath + '/') || activeFilePath === folderPath) {
              const fallback = files.find((f) => f.file_path === 'main.tex' && !f.file_path.startsWith(folderPath + '/'))
              const next = fallback?.file_path ?? null
              setActiveFilePath(next)
              activeFilePathRef.current = next
            }
          },
        })
      },
    })
  }

  const uploadFiles = useCallback((fileList: FileList | File[]) => {
    const files = Array.from(fileList)
    for (const file of files) {
      createMutation.mutate(
        { path: file.name, file },
        {
          onSuccess: () => { setActiveFilePath(file.name) },
          onError: (err) => { toast.error(`上传失败: ${file.name}`, { description: err.message }) },
        },
      )
    }
  }, [createMutation])

  const handleUploadClick = () => uploadInputRef.current?.click()
  const handleUploadChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files?.length) uploadFiles(e.target.files)
    e.target.value = ''
  }

  const [isDragOver, setIsDragOver] = useState(false)
  const handleDragOver = (e: React.DragEvent) => {
    if (!e.dataTransfer.types.includes('Files')) return
    e.preventDefault(); setIsDragOver(true)
  }
  const handleDragLeave = () => setIsDragOver(false)
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    setIsDragOver(false)
    if (e.dataTransfer.files.length) uploadFiles(e.dataTransfer.files)
  }

  const zoomIn = () => setDisplayScale((p) => Math.min(p + ZOOM_STEP, ZOOM_MAX))
  const zoomOut = () => setDisplayScale((p) => Math.max(p - ZOOM_STEP, ZOOM_MIN))
  const resetZoom = () => {
    if (previewContainerRef.current) {
      setDisplayScale((previewContainerRef.current.clientWidth - 100) / 600)
    }
  }

  const compilerLabel = COMPILER_OPTIONS.find((c) => c.value === compiler)?.label ?? compiler

  return (
    <div className="flex h-full w-full bg-background overflow-hidden select-none font-sans">
      {/* Activity Bar */}
      <aside className="flex-none w-12 border-r border-border/40 bg-muted/20 flex flex-col items-center py-4 gap-4 z-30">
        <TabButton testId="writing-tab-files" icon={<Files size={20} />} active={activeTab === 'files'} onClick={() => setActiveTab('files')} />
        <TabButton testId="writing-tab-ai" icon={<Sparkles size={20} />} active={activeTab === 'ai'} onClick={() => setActiveTab('ai')} />
        <div className="mt-auto flex flex-col gap-4">
          <button className="p-2 text-muted-foreground hover:text-foreground transition-all hover:scale-110"><Search size={20} /></button>
          <button className="p-2 text-muted-foreground hover:text-foreground transition-all hover:scale-110"><Monitor size={20} /></button>
        </div>
      </aside>

      <div className="flex-1 min-w-0">
        <Resizable.Group orientation="horizontal" id="writing-layout-v9">
          {!isMobile && (
            <>
              <Resizable.Panel defaultSize={18} minSize={12} collapsible>
                <aside className="h-full bg-card flex flex-col overflow-hidden border-r">
                  {activeTab === 'files' ? (
                    <div
                      className="flex-1 flex flex-col h-full overflow-hidden relative"
                      onDragOver={handleDragOver}
                      onDragLeave={handleDragLeave}
                      onDrop={handleDrop}
                    >
                      {isDragOver && (
                        <div className="absolute inset-0 z-10 bg-accent/10 border-2 border-dashed border-accent/40 rounded-lg flex items-center justify-center pointer-events-none">
                          <p className="text-xs text-accent-foreground/70 font-medium">拖放文件到此处上传</p>
                        </div>
                      )}
                      <div className="p-4 h-14 border-b border-border/40 flex items-center justify-between flex-none bg-card/50">
                        <h3 className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">Explorer</h3>
                        <div className="flex items-center gap-1">
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md" title="新建文件" onClick={() => fileTreeRef.current?.createLeaf()}><Plus size={14} /></Button>
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md" title="新建文件夹" onClick={() => fileTreeRef.current?.createInternal()}><FolderPlus size={14} /></Button>
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md" onClick={handleUploadClick}><Upload size={14} /></Button>
                          <input ref={uploadInputRef} type="file" multiple className="hidden" onChange={handleUploadChange} />
                        </div>
                      </div>
                      <WritingFileTree
                        ref={fileTreeRef}
                        files={files}
                        activeFilePath={activeFilePath}
                        onSelect={handleSelectFile}
                        onDeleteFile={handleDeleteFile}
                        onDeleteFolder={handleDeleteFolder}
                        onCreateFile={handleTreeCreateFile}
                        onRenameFile={handleTreeRenameFile}
                        onRenameFolder={handleTreeRenameFolder}
                        onMoveFile={handleTreeMoveFile}
                        onExternalDrop={(fileList) => uploadFiles(fileList)}
                      />
                    </div>
                  ) : (
                    <AiPanel projectId={projectId} />
                  )}
                </aside>
              </Resizable.Panel>
              <ResizeHandle />
            </>
          )}

          {/* Editor Panel */}
          <Resizable.Panel defaultSize={isMobile ? 100 : 42} minSize={25}>
            <section className="h-full flex flex-col min-w-0 bg-background relative select-text border-r">
              <div className="h-10 border-b border-border/40 flex-none flex items-center justify-between px-4 bg-muted/10">
                <div className="flex items-center gap-3">
                  <div className="flex items-center gap-1.5">
                    <span className="text-[10px] font-bold text-muted-foreground truncate max-w-[120px]">
                      {activeFilePath || '无文件'}
                    </span>
                    <SavingStatus isSaving={saveMutation.isPending} />
                  </div>
                </div>
                <div className="flex items-center gap-1">
                  <span className="text-[10px] font-mono text-muted-foreground mr-1">{compilerLabel}</span>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    data-testid="compile-button"
                    aria-label={isCompiling ? '编译中' : '编译'}
                    className="rounded-md text-green-600 hover:text-green-700 hover:bg-green-50 transition-all active:scale-90"
                    onClick={handleCompile}
                    disabled={isCompiling}
                  >
                    {isCompiling ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} className="fill-current" />}
                  </Button>
                </div>
              </div>
              <div className="flex-1 relative overflow-hidden">
                {!activeFilePath ? (
                  <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                    选择一个文件开始编辑
                  </div>
                ) : (isEditable && isContentLoading) ? (
                  <LoadingSpinner text="正在获取文件内容" className="h-full" />
                ) : isEditable ? (
                  <Editor
                    height="100%" language={getLanguageByExt(activeFilePath)} theme={editorDark ? 'vs-dark' : 'vs'}
                    value={editorContent} onChange={handleEditorChange}
                    options={{ minimap: { enabled: false }, fontSize: 14, lineHeight: 1.6, padding: { top: 20 }, wordWrap: 'on', automaticLayout: true, folding: true, lineNumbers: 'on', scrollbar: { vertical: 'visible', useShadows: false } }}
                  />
                ) : (
                  <BinaryFilePlaceholder path={activeFilePath} projectId={projectId} />
                )}
              </div>
            </section>
          </Resizable.Panel>

          {/* Preview Panel */}
          <Resizable.Panel defaultSize={40} minSize={20}>
            <aside className="h-full border-l border-border/40 bg-muted/20 flex flex-col relative min-w-0">
              <div className="h-10 border-b border-border/40 flex-none flex items-center justify-between px-4 bg-card/50">
                <span className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">Preview</span>
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={zoomOut}><ZoomOut size={14} /></Button>
                  <span className="text-[10px] font-bold text-muted-foreground w-10 text-center">{Math.round(displayScale * 100)}%</span>
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={zoomIn}><ZoomIn size={14} /></Button>
                  <div className="w-px h-3 bg-border mx-1" />
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={resetZoom}><Maximize size={14} /></Button>
                  {pdfBuildId && (
                    <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={handleDownloadPdf} disabled={isDownloadingPdf}>
                      {isDownloadingPdf ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
                    </Button>
                  )}
                </div>
              </div>

              <div ref={previewContainerRef} className="flex-1 overflow-y-auto p-8 bg-[#525659]/10 scrollbar-thin scrollbar-thumb-muted-foreground/20">
                {compileError ? (
                  <div className="max-w-lg mx-auto mt-16 rounded-xl bg-destructive/5 border border-destructive/20 p-6 select-text">
                    <div className="flex items-center justify-between mb-3 text-destructive">
                      <div className="flex items-center gap-2">
                        <AlertCircle size={16} />
                        <h4 className="text-sm font-bold">编译错误</h4>
                      </div>
                      <button
                        className="inline-flex items-center gap-1 text-[10px] text-muted-foreground hover:text-foreground transition-colors px-2 py-1 rounded hover:bg-muted/50"
                        onClick={() => {
                          const text = compileLogs ? `${compileError}\n\n--- Full Logs ---\n${compileLogs}` : compileError || ''
                          navigator.clipboard.writeText(text)
                          toast.success('已复制到剪贴板')
                        }}
                      >
                        <Copy size={10} />
                        复制
                      </button>
                    </div>
                    <pre className="text-[11px] text-destructive/80 whitespace-pre-wrap font-mono leading-relaxed">{compileError}</pre>
                    {compileLogs && (
                      <div className="mt-3">
                        <button
                          className="text-[10px] text-muted-foreground hover:text-foreground transition-colors"
                          onClick={() => setShowFullLogs(!showFullLogs)}
                        >
                          {showFullLogs ? '收起完整日志' : '查看完整日志'}
                        </button>
                        {showFullLogs && (
                          <pre className="mt-2 text-[10px] text-muted-foreground/70 whitespace-pre-wrap font-mono leading-relaxed max-h-80 overflow-y-auto bg-muted/30 rounded-lg p-3">
                            {compileLogs}
                          </pre>
                        )}
                      </div>
                    )}
                  </div>
                ) : pdfFileSource ? (
                  <div className="flex flex-col items-center gap-6 origin-top transition-transform duration-75 ease-out" style={{ transform: `scale(${displayScale / renderScale})` }}>
                    <Document
                      file={pdfFileSource ? toPdfFile(pdfFileSource) : null}
                      onLoadSuccess={({ numPages }) => setNumPages(numPages)}
                      loading={<LoadingSpinner text="Rendering..." />}
                      className="flex flex-col gap-6"
                    >
                      {Array.from({ length: numPages }, (_, i) => (
                        <div key={`p_${i + 1}`} className="shadow-2xl rounded-sm bg-white overflow-hidden">
                          <Page pageNumber={i + 1} scale={renderScale} renderTextLayer renderAnnotationLayer loading={null} />
                        </div>
                      ))}
                    </Document>
                  </div>
                ) : (
                  <EmptyPreview />
                )}
              </div>
            </aside>
          </Resizable.Panel>
        </Resizable.Group>
      </div>
      <DeleteConfirmDialog />
    </div>
  )
}

// --- Sub-components ---

function ResizeHandle() {
  return (
    <Resizable.Separator className="w-1 bg-transparent hover:bg-primary/20 active:bg-primary/40 transition-colors relative z-50">
      <div className="absolute inset-y-0 left-1/2 -translate-x-1/2 w-px bg-border/40" />
    </Resizable.Separator>
  )
}

function TabButton({ icon, active, onClick, testId }: { icon: React.ReactNode; active: boolean; onClick: () => void; testId?: string }) {
  return (
    <button
      data-testid={testId}
      onClick={onClick}
      className={cn("p-2 rounded-xl transition-all hover:scale-110", active ? "bg-background text-primary shadow-sm ring-1 ring-border/50" : "text-muted-foreground hover:text-foreground")}
    >
      {icon}
    </button>
  )
}

function EmptyPreview() {
  return (
    <div className="flex flex-col items-center justify-center text-center p-12 max-w-xs mt-32 mx-auto">
      <div className="w-16 h-16 rounded-3xl bg-background shadow-md flex items-center justify-center mb-6">
        <Play className="h-6 w-6 text-primary fill-primary/10" />
      </div>
      <h4 className="text-sm font-bold mb-2 text-muted-foreground">Ready to Compile</h4>
    </div>
  )
}

function BinaryFilePlaceholder({ path, projectId }: { path: string, projectId: string }) {
  const isImg = isImageFile(path)
  const isPdf = isPdfFile(path)
  const [imgUrl, setImgUrl] = useState<string | null>(null)
  const [loading, setLoading] = useState(isImg)

  // Use the API client to fetch the file with Auth headers
  useEffect(() => {
    if (!isImg) return
    let objectUrl: string | null = null

    async function loadPreview() {
      try {
        const url = LATEX.fileContent(projectId, path) + '?raw=true'
        const blob = await downloadBlob(url)
        objectUrl = URL.createObjectURL(blob)
        setImgUrl(objectUrl)
      } catch {
        toast.error('图片预览加载失败')
      } finally {
        setLoading(false)
      }
    }

    loadPreview()
    return () => { if (objectUrl) URL.revokeObjectURL(objectUrl) }
  }, [path, projectId, isImg])

  const handleDownload = async () => {
    try {
      toast.info('正在准备下载...')
      const url = LATEX.fileContent(projectId, path) + '?raw=true'
      const blob = await downloadBlob(url)
      const blobUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = blobUrl
      a.download = path.split('/').pop() || 'file'
      document.body.appendChild(a)
      a.click()
      document.body.removeChild(a)
      URL.revokeObjectURL(blobUrl)
    } catch {
      toast.error('下载失败', { description: '无法获取文件内容，请检查网络或权限' })
    }
  }

  return (
    <div className="flex flex-col items-center justify-center h-full p-12 bg-muted/5 text-center">
      <div className="w-20 h-20 rounded-3xl bg-background shadow-sm border border-border/40 flex items-center justify-center mb-6">
        {isImg ? <Monitor className="h-8 w-8 text-primary/60" /> : <Files size={32} className="text-muted-foreground/40" />}
      </div>
      <h3 className="text-sm font-bold mb-1 truncate max-w-xs">{path.split('/').pop()}</h3>
      <p className="text-xs text-muted-foreground mb-8">
        {isImg ? '图像预览' : isPdf ? 'PDF 文档 (编译输出)' : '该文件格式不支持在编辑器中直接查看'}
      </p>

      {isImg ? (
        <div className="max-w-md bg-white p-2 rounded-xl shadow-2xl border border-border/20 animate-in fade-in zoom-in duration-500 min-h-[100px] flex items-center justify-center">
          {loading ? (
            <Loader2 className="animate-spin text-muted-foreground/20" size={24} />
          ) : imgUrl ? (
            <img src={imgUrl} alt={path} className="max-h-[60vh] rounded-lg object-contain" />
          ) : (
            <div className="text-[10px] text-muted-foreground p-8">预览加载失败</div>
          )}
        </div>
      ) : (
        <Button variant="outline" size="sm" className="px-5" onClick={handleDownload}>
          下载文件
        </Button>
      )}
    </div>
  )
}

function SavingStatus({ isSaving }: { isSaving: boolean }) {
  const isGuest = useSessionStore((session) => session.isGuest)
  const [showSaved, setShowSaved] = useState(false)
  const lastSaving = useRef(isSaving)

  useEffect(() => {
    if (lastSaving.current && !isSaving) {
      const showTimer = setTimeout(() => setShowSaved(true), 0)
      const hideTimer = setTimeout(() => setShowSaved(false), 2000)
      return () => { clearTimeout(showTimer); clearTimeout(hideTimer) }
    }
    lastSaving.current = isSaving
  }, [isSaving])

  if (isSaving) {
    return (
      <div className="flex items-center gap-1 text-[10px] text-primary/60 animate-pulse">
        {isGuest ? <HardDrive size={12} /> : <CloudUpload size={12} />}
        <span>{isGuest ? 'Saving on this device...' : 'Saving...'}</span>
      </div>
    )
  }

  if (showSaved) {
    return (
      <div className="flex items-center gap-1 text-[10px] text-green-600/70 animate-in fade-in zoom-in duration-300">
        <Check size={12} />
        <span>{isGuest ? 'Saved on this device' : 'Saved'}</span>
      </div>
    )
  }

  return (
    <div className="flex items-center gap-1 text-[10px] text-muted-foreground/40">
      {isGuest ? <HardDrive size={12} /> : <Cloud size={12} />}
      <span>{isGuest ? 'Stored on this device' : 'Synced'}</span>
    </div>
  )
}
