import { cn } from '@/lib/utils'
import { useState, useEffect, useRef } from 'react'
import { 
  MessageSquareText, 
  Search, 
  Play, 
  Download, 
  Sparkles,
  Loader2,
  FileText,
  FolderOpen,
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
import * as Resizable from 'react-resizable-panels'

// Set up worker for react-pdf
pdfjs.GlobalWorkerOptions.workerSrc = `//unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`

import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'

type SidebarTab = 'files' | 'ai'

export function WritingPage() {
  const [activeTab, setActiveTab] = useState<SidebarTab>('files')
  const [numPages, setNumPages] = useState<number>(0)
  
  // Anti-flicker scale state
  const [displayScale, setDisplayScale] = useState<number>(0.9) // Fast CSS scale
  const [renderScale, setRenderScale] = useState<number>(0.9)   // Debounced PDF render scale
  
  const [isCompiling, setIsCompiling] = useState(false)
  const [isMobile, setIsMobile] = useState(false)
  const previewContainerRef = useRef<HTMLDivElement>(null)
  const debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  
  const [latexContent, setLatexContent] = useState(`\\documentclass{article}
\\usepackage[utf8]{inputenc}

\\title{Anti-Flicker LaTeX Preview}
\\author{PaperSage Pro}
\\date{\\today}

\\begin{document}

\\maketitle

\\section{Flicker-Free Zoom}
We implemented a dual-scale strategy:
1. CSS Transform for immediate feedback (No flickering).
2. Debounced High-res rendering for crisp text.

\\section{Instructions}
Hold CTRL and scroll fast. You'll see the scale changes 
instantly without any white flashes.

\\end{document}`)

  const [pdfFile, setPdfFile] = useState<string | null>(null)

  useEffect(() => {
    const checkMobile = () => setIsMobile(window.innerWidth < 1024)
    checkMobile()
    window.addEventListener('resize', checkMobile)
    return () => window.removeEventListener('resize', checkMobile)
  }, [])

  // Sync renderScale with displayScale after a short delay
  useEffect(() => {
    if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current)
    
    debounceTimerRef.current = setTimeout(() => {
      setRenderScale(displayScale)
    }, 400) // Render high-res only after 400ms of inactivity

    return () => {
      if (debounceTimerRef.current) clearTimeout(debounceTimerRef.current)
    }
  }, [displayScale])

  // Handle Ctrl + Wheel Zoom
  useEffect(() => {
    const container = previewContainerRef.current
    if (!container) return

    const handleWheel = (e: WheelEvent) => {
      if (e.ctrlKey || e.metaKey) {
        e.preventDefault()
        const delta = -e.deltaY
        const factor = delta > 0 ? 1.08 : 0.92
        setDisplayScale(prev => {
          const nextScale = prev * factor
          return Math.max(0.4, Math.min(nextScale, 3.0))
        })
      }
    }

    container.addEventListener('wheel', handleWheel, { passive: false })
    return () => container.removeEventListener('wheel', handleWheel)
  }, [])

  const handleEditorChange = (value: string | undefined) => {
    if (value !== undefined) setLatexContent(value)
  }

  const handleCompile = () => {
    setIsCompiling(true)
    setTimeout(() => {
      setPdfFile('https://raw.githubusercontent.com/mozilla/pdf.js/ba2edeae/web/compressed.tracemonkey-pldi-09.pdf')
      setIsCompiling(false)
    }, 800)
  }

  const zoomIn = () => setDisplayScale(prev => Math.min(prev + 0.15, 3.0))
  const zoomOut = () => setDisplayScale(prev => Math.max(prev - 0.15, 0.4))
  const resetZoom = () => {
    if (previewContainerRef.current) {
      const containerWidth = previewContainerRef.current.clientWidth
      setDisplayScale((containerWidth - 100) / 600)
    }
  }

  return (
    <div className="flex h-full w-full bg-background overflow-hidden select-none font-sans">
      {/* 1. Activity Bar */}
      <aside className="flex-none w-12 border-r border-border/40 bg-muted/20 flex flex-col items-center py-4 gap-4 z-30">
        <button onClick={() => setActiveTab('files')} className={cn("p-2 rounded-xl transition-all hover:scale-110", activeTab === 'files' ? "bg-background text-primary shadow-sm ring-1 ring-border/50" : "text-muted-foreground hover:text-foreground")}>
          <Files size={20} />
        </button>
        <button onClick={() => setActiveTab('ai')} className={cn("p-2 rounded-xl transition-all hover:scale-110", activeTab === 'ai' ? "bg-background text-primary shadow-sm ring-1 ring-border/50" : "text-muted-foreground hover:text-foreground")}>
          <Sparkles size={20} />
        </button>
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
              </Resizable.Panel>
              <ResizeHandle />
            </>
          )}

          <Resizable.Panel defaultSize={isMobile ? 100 : 42} minSize={25}>
            <section className="h-full flex flex-col min-w-0 bg-background relative select-text border-r">
              <div className="h-10 border-b border-border/40 flex-none flex items-center justify-between px-4 bg-muted/10">
                <EditToolbar />
                <div className="flex items-center gap-1">
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-green-600 hover:text-green-700 hover:bg-green-50 transition-all active:scale-90" onClick={handleCompile} disabled={isCompiling}>
                    {isCompiling ? <Loader2 size={14} className="animate-spin" /> : <Play size={14} className="fill-current" />}
                  </Button>
                  <Button variant="ghost" size="icon-xs" className="rounded-md text-muted-foreground"><Download size={14} /></Button>
                </div>
              </div>
              <div className="flex-1 relative overflow-hidden">
                <Editor height="100%" defaultLanguage="latex" theme="vs-light" value={latexContent} onChange={handleEditorChange} options={{ minimap: { enabled: false }, fontSize: 14, lineHeight: 1.6, padding: { top: 20 }, wordWrap: 'on', automaticLayout: true, folding: true, lineNumbers: 'on', scrollbar: { vertical: 'visible', useShadows: false } }} />
              </div>
            </section>
          </Resizable.Panel>

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
              
              <div 
                ref={previewContainerRef}
                className="flex-1 overflow-y-auto p-8 bg-[#525659]/10 scrollbar-thin scrollbar-thumb-muted-foreground/20"
              >
                {pdfFile ? (
                  <div 
                    className="flex flex-col items-center gap-6 origin-top transition-transform duration-75 ease-out"
                    style={{ transform: `scale(${displayScale / renderScale})` }}
                  >
                    <Document
                      file={pdfFile}
                      onLoadSuccess={({ numPages }) => setNumPages(numPages)}
                      loading={<div className="flex flex-col items-center justify-center p-20 gap-4"><Loader2 className="h-8 w-8 animate-spin text-primary/40" /><p className="text-sm text-muted-foreground font-medium uppercase tracking-tighter">Rendering...</p></div>}
                      className="flex flex-col gap-6"
                    >
                      {Array.from(new Array(numPages), (_, index) => (
                        <div key={`p_${index + 1}`} className="shadow-2xl rounded-sm bg-white overflow-hidden">
                          <Page 
                            pageNumber={index + 1} 
                            scale={renderScale} 
                            renderTextLayer={true} 
                            renderAnnotationLayer={true}
                            loading={null}
                          />
                        </div>
                      ))}
                    </Document>
                  </div>
                ) : (
                  <div className="flex flex-col items-center justify-center text-center p-12 max-w-xs mt-32 mx-auto">
                    <div className="w-16 h-16 rounded-3xl bg-background shadow-md flex items-center justify-center mb-6"><Play className="h-6 w-6 text-primary fill-primary/10" /></div>
                    <h4 className="text-sm font-bold mb-2 text-muted-foreground">Ready to Compile</h4>
                  </div>
                )}
              </div>
            </aside>
          </Resizable.Panel>
        </Resizable.Group>
      </div>
    </div>
  )
}

function ResizeHandle() {
  return (
    <Resizable.Separator className="w-1 bg-transparent hover:bg-primary/20 active:bg-primary/40 transition-colors relative z-50">
      <div className="absolute inset-y-0 left-1/2 -translate-x-1/2 w-px bg-border/40" />
    </Resizable.Separator>
  )
}

function FileTreeItem({ icon, label, active = false, indent = false }: { icon: React.ReactNode, label: string, active?: boolean, indent?: boolean }) {
  return (
    <div className="px-2 w-full mb-0.5">
      <div className={cn("flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer transition-all duration-200 rounded-lg group relative", indent ? "ml-4 w-[calc(100%-1rem)]" : "w-full", active ? "bg-primary/10 text-primary font-bold shadow-sm" : "text-muted-foreground hover:bg-muted hover:text-foreground")}>
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
      <div className="px-2"><div onClick={() => setOpen(!open)} className="flex items-center gap-2 px-2 py-1.5 text-[12px] cursor-pointer text-foreground/80 hover:bg-muted transition-all rounded-lg font-bold group"><FolderOpen size={14} className={cn("text-yellow-500/80 transition-transform duration-300", !open && "-rotate-90", "group-hover:scale-110")} /><span>{label}</span></div></div>
      {open && children}
    </div>
  )
}

function EditToolbar() {
  return (
    <div className="flex items-center gap-3 opacity-50"><span className="hover:text-primary cursor-pointer px-1 transition-colors font-bold text-[13px]">B</span><span className="hover:text-primary cursor-pointer px-1 transition-colors italic font-serif text-[13px]">I</span><span className="hover:text-primary cursor-pointer px-1 transition-colors underline underline-offset-4 text-[13px]">U</span><div className="h-3 w-px bg-border" /><span className="hover:text-primary cursor-pointer px-1 transition-colors text-[9px] font-mono tracking-tighter">\\cite</span><span className="hover:text-primary cursor-pointer px-1 transition-colors text-[9px] font-mono tracking-tighter">\\ref</span></div>
  )
}
