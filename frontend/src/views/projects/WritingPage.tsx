import { cn } from '@/lib/utils'
import { useState, useEffect, useRef } from 'react'
import { 
  MessageSquareText, 
  History, 
  Library, 
  Search, 
  ChevronLeft, 
  ChevronRight, 
  Play, 
  Download, 
  Maximize2,
  Sparkles,
  Loader2,
  FileText,
  FolderOpen,
  Image as ImageIcon,
  FileCode,
  Plus,
  Upload,
  Files,
  Monitor,
  ZoomIn,
  ZoomOut,
  Maximize
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import Editor from '@monaco-editor/react'
import { Document, Page, pdfjs } from 'react-pdf'
import { 
  Panel, 
  PanelGroup, 
  PanelResizeHandle 
} from 'react-resizable-panels'

// Set up worker for react-pdf
pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`

import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'

type SidebarTab = 'files' | 'ai'

export function WritingPage() {
  const [activeTab, setActiveTab] = useState<SidebarTab>('files')
  const [numPages, setNumPages] = useState<number>(0)
  const [scale, setScale] = useState<number>(0.9)
  const [isCompiling, setIsCompiling] = useState(false)
  const [isMobile, setIsMobile] = useState(false)
  const previewContainerRef = useRef<HTMLDivElement>(null)
  
  const [latexContent, setLatexContent] = useState(`\\documentclass{article}
\\usepackage[utf8]{inputenc}

\\title{Professional LaTeX Environment}
\\author{PaperSage Developer}
\\date{\\today}

\\begin{document}

\\maketitle

\\section{Introduction}
This is a professional LaTeX editing environment. 
You can resize panels by dragging the handles between them.

\\section{Zoom Controls}
Try the + and - buttons in the preview header to adjust the scale.

\\end{document}`)

  const [pdfFile, setPdfFile] = useState<string | null>(null)

  useEffect(() => {
    const checkMobile = () => setIsMobile(window.innerWidth < 1024)
    checkMobile()
    window.addEventListener('resize', checkMobile)
    return () => window.removeEventListener('resize', checkMobile)
  }, [])

  const handleEditorChange = (value: string | undefined) => {
    if (value !== undefined) setLatexContent(value)
  }

  const handleCompile = () => {
    setIsCompiling(true)
    setTimeout(() => {
      setPdfFile('https://raw.githubusercontent.com/mozilla/pdf.js/ba2edeae/web/compressed.tracemonkey-pldi-09.pdf')
      setIsCompiling(false)
    }, 1000)
  }

  const zoomIn = () => setScale(prev => Math.min(prev + 0.1, 2.5))
  const zoomOut = () => setScale(prev => Math.max(prev - 0.1, 0.4))
  const resetZoom = () => {
    if (previewContainerRef.current) {
      const containerWidth = previewContainerRef.current.clientWidth
      setScale((containerWidth - 80) / 600)
    }
  }

  return (
    <div className="flex h-full w-full bg-background overflow-hidden select-none font-sans">
      {/* 1. Activity Bar */}
      <aside className="flex-none w-12 border-r border-border/40 bg-muted/20 flex flex-col items-center py-4 gap-4 z-30">
        <button 
          onClick={() => setActiveTab('files')}
          className={cn(
            "p-2 rounded-xl transition-all hover:scale-110",
            activeTab === 'files' ? "bg-background text-primary shadow-sm ring-1 ring-border/50" : "text-muted-foreground hover:text-foreground"
          )}
        >
          <Files size={20} />
        </button>
        <button 
          onClick={() => setActiveTab('ai')}
          className={cn(
            "p-2 rounded-xl transition-all hover:scale-110",
            activeTab === 'ai' ? "bg-background text-primary shadow-sm ring-1 ring-border/50" : "text-muted-foreground hover:text-foreground"
          )}
        >
          <Sparkles size={20} />
        </button>
        <div className="mt-auto flex flex-col gap-4">
          <button className="p-2 text-muted-foreground hover:text-foreground transition-all hover:scale-110"><Search size={20} /></button>
          <button className="p-2 text-muted-foreground hover:text-foreground transition-all hover:scale-110"><Monitor size={20} /></button>
        </div>
      </aside>

      <div className="flex-1 min-w-0">
        <PanelGroup direction="horizontal" autoSaveId="writing-layout-v4">
          {/* 2. Side Pane */}
          {!isMobile && (
            <>
              <Panel defaultSize={18} minSize={12} collapsible>
                <aside className="h-full bg-card flex flex-col overflow-hidden border-r">
                  {activeTab === 'files' ? (
                    <div className="flex-1 flex flex-col h-full overflow-hidden">
                      <div className="p-4 h-14 border-b border-border/40 flex items-center justify-between flex-none bg-card/50">
                        <h3 className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">Explorer</h3>
                        <div className="flex items-center gap-1">
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md"><Plus size={14} /></Button>
                          <Button variant="ghost" size="icon-xs" className="h-7 w-7 rounded-md"><Upload size={14} /></Button>
                        </div>
                      </div>
                      <div className="flex-1 overflow-y-auto py-3 scrollbar-none">
                        <FileTreeItem icon={<FileText size={14} className="text-primary" />} label="main.tex" active />
                        <FileTreeItem icon={<FileCode size={14} className="text-orange-500" />} label="references.bib" />
                        <FileTreeFolder label="sections">
                          <FileTreeItem icon={<FileText size={14} />} label="intro.tex" indent />
                        </FileTreeFolder>
                        <FileTreeFolder label="images">
                          <FileTreeItem icon={<ImageIcon size={14} className="text-blue-500" />} label="arch.pdf" indent />
                        </FileTreeFolder>
                      </div>
                    </div>
                  ) : (
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
                  )}
                </aside>
              </Panel>
              <ResizeHandle />
            </>
          )}

          {/* 3. Middle: Editor */}
          <Panel defaultSize={isMobile ? 100 : 42} minSize={25}>
            <section className="h-full flex flex-col min-w-0 bg-background relative select-text border-r">
              <div className="h-10 border-b border-border/40 flex-none flex items-center justify-between px-4 bg-muted/10">
                <EditToolbar />
                <div className="flex items-center gap-1">
                  <Button 
                    variant="ghost" 
                    size="icon-xs" 
                    className="rounded-md text-green-600 hover:text-green-700 hover:bg-green-50 transition-all active:scale-90"
                    onClick={handleCompile}
                    disabled={isCompiling}
                  >
                    {isCompiling ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} className="fill-current" />}
                  </Button>
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground"><Download size={14} /></Button>
                </div>
              </div>
              <div className="flex-1 relative overflow-hidden">
                <Editor
                  height="100%"
                  defaultLanguage="latex"
                  theme="vs-light"
                  value={latexContent}
                  onChange={handleEditorChange}
                  options={{
                    minimap: { enabled: false },
                    fontSize: 14,
                    lineHeight: 1.6,
                    padding: { top: 20 },
                    wordWrap: 'on',
                    automaticLayout: true,
                    folding: true,
                    lineNumbers: 'on',
                    scrollbar: { vertical: 'visible', useShadows: false }
                  }}
                />
              </div>
            </section>
          </Panel>

          {/* 4. Right: Continuous Preview Renderer with Lazy Loading */}
          {!isMobile && (
            <>
              <ResizeHandle />
              <Panel defaultSize={40} minSize={20}>
                <aside className="h-full border-l border-border/40 bg-muted/20 flex flex-col relative min-w-0">
                  <div className="h-10 border-b border-border/40 flex-none flex items-center justify-between px-4 bg-card/50">
                    <span className="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">Preview</span>
                    <div className="flex items-center gap-1">
                      <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={zoomOut}><ZoomOut size={14} /></Button>
                      <span className="text-[10px] font-bold text-muted-foreground w-10 text-center">{Math.round(scale * 100)}%</span>
                      <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={zoomIn}><ZoomIn size={14} /></Button>
                      <div className="w-px h-3 bg-border mx-1" />
                      <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground" onClick={resetZoom}><Maximize size={14} /></Button>
                    </div>
                  </div>
                  
                  <div 
                    ref={previewContainerRef}
                    className="flex-1 overflow-y-auto p-8 flex flex-col items-center bg-[#525659]/10 scrollbar-thin scrollbar-thumb-muted-foreground/20"
                  >
                    {pdfFile ? (
                      <Document
                        file={pdfFile}
                        onLoadSuccess={({ numPages }) => setNumPages(numPages)}
                        loading={<div className="flex flex-col items-center justify-center p-20 gap-4"><Loader2 className="h-8 w-8 animate-spin text-primary/40" /><p className="text-sm text-muted-foreground font-medium uppercase tracking-tighter">Rendering...</p></div>}
                        className="flex flex-col gap-6"
                      >
                        {Array.from(new Array(numPages), (el, index) => (
                          <div key={`p_${index + 1}`} className="shadow-2xl rounded-sm bg-white overflow-hidden transition-transform duration-300">
                            <Page 
                              pageNumber={index + 1} 
                              scale={scale} 
                              renderTextLayer={true} 
                              renderAnnotationLayer={true}
                              loading={<div className="w-[500px] h-[700px] bg-white flex items-center justify-center text-xs text-muted-foreground italic">Page {index + 1} Loading...</div>}
                            />
                          </div>
                        ))}
                      </Document>
                    ) : (
                      <div className="flex flex-col items-center justify-center text-center p-12 max-w-xs mt-32">
                        <div className="w-16 h-16 rounded-3xl bg-background shadow-md flex items-center justify-center mb-6"><Play className="h-6 w-6 text-primary fill-primary/10" /></div>
                        <h4 className="text-sm font-bold mb-2">Ready to Compile</h4>
                        <p className="text-[12px] text-muted-foreground leading-relaxed">Click the Compile button to preview your PDF.</p>
                      </div>
                    )}
                  </div>
                </aside>
              </Panel>
            </>
          )}
        </PanelGroup>
      </div>
    </div>
  )
}

function ResizeHandle() {
  return (
    <PanelResizeHandle className="w-1 bg-transparent hover:bg-primary/20 active:bg-primary/40 transition-colors relative z-50">
      <div className="absolute inset-y-0 left-1/2 -translate-x-1/2 w-px bg-border/40" />
    </PanelResizeHandle>
  )
}

function FileTreeItem({ icon, label, active = false, indent = false }: { icon: React.ReactNode, label: string, active?: boolean, indent?: boolean }) {
  return (
    <div className="px-2 w-full mb-0.5">
      <div className={cn(
        "flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer transition-all duration-200 rounded-lg group relative",
        indent ? "ml-4 w-[calc(100%-1rem)]" : "w-full",
        active ? "bg-primary/10 text-primary font-bold shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground"
      )}>
        <span className="shrink-0 transition-transform group-hover:scale-110">{icon}</span>
        <span className="truncate">{label}</span>
        {active && <div className="absolute left-0 top-1.5 bottom-1.5 w-0.5 bg-primary rounded-full" />}
      </div>
    </div>
  )
}

function FileTreeFolder({ label, children }: { label: string, children: React.ReactNode }) {
  const [open, setOpen] = useState(true)
  return (
    <div className="mb-0.5">
      <div className="px-2">
        <div onClick={() => setOpen(!open)} className="flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer text-foreground/80 hover:bg-muted transition-all rounded-lg font-bold group">
          <FolderOpen size={14} className={cn("text-yellow-500/80 transition-transform duration-300", !open && "-rotate-90", "group-hover:scale-110")} />
          <span>{label}</span>
        </div>
      </div>
      {open && children}
    </div>
  )
}

function EditToolbar() {
  return (
    <div className="flex items-center gap-3 opacity-50">
      <span className="hover:text-primary cursor-pointer px-1 transition-colors font-bold text-[13px]">B</span>
      <span className="hover:text-primary cursor-pointer px-1 transition-colors italic font-serif text-[13px]">I</span>
      <span className="hover:text-primary cursor-pointer px-1 transition-colors underline underline-offset-4 text-[13px]">U</span>
      <div className="h-3 w-px bg-border" />
      <span className="hover:text-primary cursor-pointer px-1 transition-colors text-[9px] font-mono tracking-tighter">\\cite</span>
      <span className="hover:text-primary cursor-pointer px-1 transition-colors text-[9px] font-mono tracking-tighter">\\ref</span>
    </div>
  )
}
