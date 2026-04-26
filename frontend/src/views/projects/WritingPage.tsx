import { cn } from '@/lib/utils'
import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams } from '@tanstack/react-router'
import {
  MessageSquareText, Search, Play, Download, Sparkles, Loader2,
  Plus, Upload, Files, Monitor, ZoomIn, ZoomOut, Maximize, AlertCircle,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import Editor, { useMonaco } from '@monaco-editor/react'
import { Document, Page, pdfjs } from 'react-pdf'
import * as Resizable from 'react-resizable-panels'
import { registerLatexLsp } from '@/lib/monaco-latex-lsp'
import { useConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  useLatexFiles, useLatexFileContent, useSaveLatexFile,
  useCreateLatexFile, useDeleteLatexFile, useLatexCompile, useLatexOutputPdf,
} from '@/hooks/use-latex'
import type { PdfRangeSource } from '@/services/latex-api'
import { LATEX } from '@/services/api-routes'

function toPdfFile(source: PdfRangeSource) {
  return source as Parameters<typeof Document>[0]['file']
}
import { WritingFileTree } from './writing-file-tree'

pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`
import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'

// --- Constants ---

const AUTO_SAVE_DELAY_MS = 1500
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

function detectCompiler(content: string): string {
  return XELATEX_RE.test(content) ? 'xelatex' : 'pdflatex'
}

function getLanguageByExt(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  if (['tex', 'cls', 'sty', 'bst', 'def', 'lof', 'lot', 'toc'].includes(ext)) return 'latex'
  if (ext === 'bib') return 'bibtex'
  if (ext === 'md') return 'markdown'
  return 'plaintext'
}

type SidebarTab = 'files' | 'ai'

// --- Main Component ---

export function WritingPage() {
  const { projectId = '' } = useParams({ strict: false })

  // UI state
  const [activeTab, setActiveTab] = useState<SidebarTab>('files')
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null)
  const [compiler, setCompiler] = useState('pdflatex')
  const [numPages, setNumPages] = useState(0)
  const [displayScale, setDisplayScale] = useState(0.9)
  const [renderScale, setRenderScale] = useState(0.9)
  const [isMobile, setIsMobile] = useState(false)
  const [editorContent, setEditorContent] = useState('')
  const [isCreatingFile, setIsCreatingFile] = useState(false)
  const [newFileName, setNewFileName] = useState('')

  // Refs
  const previewContainerRef = useRef<HTMLDivElement>(null)
  const renderDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastSavedContentRef = useRef('')
  const localEditsRef = useRef<string | null>(null)

  const monaco = useMonaco()
  const { confirm, ConfirmDialog: DeleteConfirmDialog } = useConfirmDialog()

  // Data hooks
  const { data: files = [] } = useLatexFiles(projectId)
  const { data: fileContent } = useLatexFileContent(projectId, activeFilePath)
  const saveMutation = useSaveLatexFile(projectId)
  const createMutation = useCreateLatexFile(projectId)
  const deleteMutation = useDeleteLatexFile(projectId)
  const compileMutation = useLatexCompile()
  const outputPdfMutation = useLatexOutputPdf()

  // Derived state
  const [pdfFileSource, setPdfFileSource] = useState<PdfRangeSource | null>(null)
  const [pdfDownloadUrl, setPdfDownloadUrl] = useState<string | null>(null)
  const [compileError, setCompileError] = useState<string | null>(null)
  const [compileLogs, setCompileLogs] = useState<string | null>(null)
  const [showFullLogs, setShowFullLogs] = useState(false)
  const isCompiling = compileMutation.isPending || outputPdfMutation.isPending

  // Monaco LaTeX LSP
  useEffect(() => {
    if (monaco) registerLatexLsp()
  }, [monaco])

  // Mobile detection
  useEffect(() => {
    const check = () => setIsMobile(window.innerWidth < 1024)
    check()
    window.addEventListener('resize', check)
    return () => window.removeEventListener('resize', check)
  }, [])

  // Sync editor with loaded file content
  useEffect(() => {
    if (fileContent?.content !== undefined) {
      lastSavedContentRef.current = fileContent.content
      queueMicrotask(() => {
        setEditorContent(fileContent.content)
        if (activeFilePath === 'main.tex') {
          setCompiler(detectCompiler(fileContent.content))
        }
      })
    }
  }, [fileContent, activeFilePath])

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

  // Debounced auto-save
  const scheduleAutoSave = useCallback((content: string) => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => {
      if (activeFilePath && content !== lastSavedContentRef.current) {
        saveMutation.mutate({ filePath: activeFilePath, content })
        lastSavedContentRef.current = content
      }
    }, AUTO_SAVE_DELAY_MS)
  }, [activeFilePath, saveMutation])

  const handleEditorChange = (value: string | undefined) => {
    if (value === undefined) return
    localEditsRef.current = value
    scheduleAutoSave(value)
  }

  const handleSelectFile = (filePath: string) => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    setActiveFilePath(filePath)
  }

  const handleCompile = async () => {
    setCompileError(null)
    setCompileLogs(null)
    setShowFullLogs(false)
    try {
      // Auto-save current file before compiling
      if (activeFilePath && editorContent !== lastSavedContentRef.current) {
        await saveMutation.mutateAsync({ filePath: activeFilePath, content: editorContent })
        lastSavedContentRef.current = editorContent
      }

      const rootFile = files.find((f) => f.file_path === 'main.tex')?.file_path
        || activeFilePath
        || files[0]?.file_path

      if (!rootFile) return

      // Backend loads files from storage — no need to send file contents
      const result = await compileMutation.mutateAsync({
        projectId, files: [], rootFile, compiler,
      })

      if (result.status === 'error' || result.status === 'failure' || result.error) {
        setCompileError(result.error || 'Compilation failed')
        if (result.logs) setCompileLogs(result.logs)
        return
      }

      if (result.build_id) {
        const source = await outputPdfMutation.mutateAsync({ projectId, buildId: result.build_id })
        setPdfFileSource(source)
        setPdfDownloadUrl(LATEX.outputPdf(projectId, result.build_id))
      }
    } catch (err) {
      setCompileError(err instanceof Error ? err.message : '编译失败')
    }
  }

  const handleCreateFile = () => {
    const name = newFileName.trim()
    if (!name) { setIsCreatingFile(false); return }
    createMutation.mutate(
      { path: name, content: '' },
      {
        onSuccess: () => { setActiveFilePath(name); setNewFileName(''); setIsCreatingFile(false) },
        onError: () => setIsCreatingFile(false),
      },
    )
  }

  const handleDeleteFile = (filePath: string) => {
    confirm({
      title: '删除文件',
      description: `确定删除 "${filePath}" 吗？此操作不可撤销。`,
      confirmText: '删除',
      variant: 'destructive',
      onConfirm: () => {
        deleteMutation.mutate(filePath, {
          onSuccess: () => { if (activeFilePath === filePath) setActiveFilePath(null) },
        })
      },
    })
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
        <TabButton icon={<Files size={20} />} active={activeTab === 'files'} onClick={() => setActiveTab('files')} />
        <TabButton icon={<Sparkles size={20} />} active={activeTab === 'ai'} onClick={() => setActiveTab('ai')} />
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
                    <div className="flex-1 flex flex-col h-full overflow-hidden">
                      <div className="p-4 h-14 border-b border-border/40 flex items-center justify-between flex-none bg-card/50">
                        <h3 className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">Explorer</h3>
                        <div className="flex items-center gap-1">
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md" onClick={() => { setIsCreatingFile(true); setNewFileName('') }}><Plus size={14} /></Button>
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md"><Upload size={14} /></Button>
                        </div>
                      </div>
                      {isCreatingFile && (
                        <div className="px-2 py-2 border-b border-border/40">
                          <input
                            autoFocus
                            className="w-full px-2 py-1 text-[12px] bg-background border border-border rounded-md focus:outline-none focus:ring-1 focus:ring-primary"
                            placeholder="filename.tex"
                            value={newFileName}
                            onChange={(e) => setNewFileName(e.target.value)}
                            onKeyDown={(e) => { if (e.key === 'Enter') handleCreateFile(); if (e.key === 'Escape') setIsCreatingFile(false) }}
                            onBlur={() => { if (!newFileName.trim()) setIsCreatingFile(false) }}
                          />
                        </div>
                      )}
                      <WritingFileTree
                        files={files}
                        activeFilePath={activeFilePath}
                        onSelect={handleSelectFile}
                        onDeleteFile={handleDeleteFile}
                      />
                    </div>
                  ) : (
                    <AIPanel />
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
                <EditToolbar />
                <div className="flex items-center gap-1">
                  <span className="text-[10px] font-mono text-muted-foreground mr-1">{compilerLabel}</span>
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-green-600 hover:text-green-700 hover:bg-green-50 transition-all active:scale-90" onClick={handleCompile} disabled={isCompiling}>
                    {isCompiling ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} className="fill-current" />}
                  </Button>
                  {pdfDownloadUrl && (
                    <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" asChild>
                      <a href={pdfDownloadUrl} download="output.pdf"><Download size={14} /></a>
                    </Button>
                  )}
                </div>
              </div>
              <div className="flex-1 relative overflow-hidden">
                {activeFilePath ? (
                  <Editor
                    height="100%" language={getLanguageByExt(activeFilePath)} theme="vs-light"
                    value={editorContent} onChange={handleEditorChange}
                    options={{ minimap: { enabled: false }, fontSize: 14, lineHeight: 1.6, padding: { top: 20 }, wordWrap: 'on', automaticLayout: true, folding: true, lineNumbers: 'on', scrollbar: { vertical: 'visible', useShadows: false } }}
                  />
                ) : (
                  <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                    选择一个文件开始编辑
                  </div>
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
                </div>
              </div>

              <div ref={previewContainerRef} className="flex-1 overflow-y-auto p-8 bg-[#525659]/10 scrollbar-thin scrollbar-thumb-muted-foreground/20">
                {compileError ? (
                  <div className="max-w-lg mx-auto mt-16 rounded-xl bg-destructive/5 border border-destructive/20 p-6 select-text">
                    <div className="flex items-center gap-2 mb-3 text-destructive">
                      <AlertCircle size={16} />
                      <h4 className="text-sm font-bold">编译错误</h4>
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

function TabButton({ icon, active, onClick }: { icon: React.ReactNode; active: boolean; onClick: () => void }) {
  return (
    <button onClick={onClick} className={cn("p-2 rounded-xl transition-all hover:scale-110", active ? "bg-background text-primary shadow-sm ring-1 ring-border/50" : "text-muted-foreground hover:text-foreground")}>
      {icon}
    </button>
  )
}

function EditToolbar() {
  return (
    <div className="flex items-center gap-3 opacity-50">
      <span className="hover:text-primary cursor-pointer px-1 transition-colors font-bold text-[13px]">B</span>
      <span className="hover:text-primary cursor-pointer px-1 transition-colors italic font-serif text-[13px]">I</span>
      <span className="hover:text-primary cursor-pointer px-1 transition-colors underline underline-offset-4 text-[13px]">U</span>
      <div className="h-3 w-px bg-border" />
      <span className="hover:text-primary cursor-pointer px-1 transition-colors text-[9px] font-mono tracking-tighter">{'\\cite'}</span>
      <span className="hover:text-primary cursor-pointer px-1 transition-colors text-[9px] font-mono tracking-tighter">{'\\ref'}</span>
    </div>
  )
}

function AIPanel() {
  return (
    <div className="flex-1 flex flex-col h-full overflow-hidden">
      <div className="p-4 h-14 border-b border-border/40 flex items-center flex-none bg-card/50">
        <h3 className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest flex items-center gap-2">
          <Sparkles size={14} className="text-primary" /> AI Assistant
        </h3>
      </div>
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        <div className="bg-primary/5 rounded-2xl p-4 border border-primary/10 text-[12px] leading-relaxed relative">
          <div className="absolute -top-2 left-4 px-2 bg-background border border-primary/10 rounded-full text-[9px] font-black text-primary uppercase shadow-sm">AI</div>
          Ask me to refine your paragraphs.
        </div>
      </div>
      <div className="p-4 border-t border-border/40 bg-muted/5 flex-none">
        <div className="relative group">
          <textarea placeholder="Ask AI..." className="w-full rounded-2xl border border-border/60 bg-background p-4 text-[12px] focus:outline-none focus:ring-2 focus:ring-primary/10 resize-none h-28" />
          <Button size="icon-xs" className="absolute right-3 bottom-3 rounded-xl shadow-lg shadow-primary/20"><MessageSquareText size={14} /></Button>
        </div>
      </div>
    </div>
  )
}

function LoadingSpinner({ text }: { text: string }) {
  return (
    <div className="flex flex-col items-center justify-center p-20 gap-4">
      <Loader2 className="h-8 w-8 animate-spin text-primary/40" />
      <p className="text-sm text-muted-foreground font-medium uppercase tracking-tighter">{text}</p>
    </div>
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
