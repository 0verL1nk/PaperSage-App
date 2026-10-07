/** Document preview panel — PDF, DOCX, and TXT rendering with zoom controls. */

import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { FileText, ZoomIn, ZoomOut, RotateCcw, X, AlertTriangle } from 'lucide-react'
import { Document, Page, pdfjs } from 'react-pdf'
import { Button } from '@/components/ui/button'
import { getOptionalApiBaseUrl } from '@/lib/env'
import { apiClient } from '@/services/client'
import { fetchDocumentPreviewBlob } from '@/services/files-api'
import { useSessionStore } from '@/store'
import { toast } from 'sonner'
import { LoadingSpinner } from '@/components/shared/loading-spinner'
import type { ProjectDocument } from '@/types/files'

import 'react-pdf/dist/Page/AnnotationLayer.css'
import 'react-pdf/dist/Page/TextLayer.css'

pdfjs.GlobalWorkerOptions.workerSrc = `https://unpkg.com/pdfjs-dist@${pdfjs.version}/build/pdf.worker.min.mjs`

const ZOOM_STEP = 0.25
const ZOOM_MIN = 0.5
const ZOOM_MAX = 3.0
const ZOOM_DEFAULT = 1.0
const DEBOUNCE_MS = 400

function buildPreviewUrl(docId: string): string {
  // 与 apiClient 一致：按当前选中的环境解析，切换后预览也跟着走
  return `${getOptionalApiBaseUrl()}/api/v1/documents/${docId}/preview`
}

function authHeaders(): Record<string, string> {
  const token = localStorage.getItem('papersage_access_token')
  return token ? { Authorization: `Bearer ${token}` } : {}
}

function showErrorToast(message: string) {
  toast.error('预览失败', { description: message, duration: 5000 })
}

interface DocumentPreviewPanelProps {
  document: ProjectDocument
  onClose: () => void
}

function ZoomControls({ scale, onZoomIn, onZoomOut, onReset }: {
  scale: number
  onZoomIn: () => void
  onZoomOut: () => void
  onReset: () => void
}) {
  return (
    <div className="flex items-center gap-0.5 bg-muted/60 rounded-full p-0.5">
      <Button variant="ghost" size="icon-xs" className="rounded-full" onClick={onZoomOut} disabled={scale <= ZOOM_MIN}>
        <ZoomOut size={13} />
      </Button>
      <span className="text-[10px] text-muted-foreground w-10 text-center tabular-nums select-none">
        {Math.round(scale * 100)}%
      </span>
      <Button variant="ghost" size="icon-xs" className="rounded-full" onClick={onZoomIn} disabled={scale >= ZOOM_MAX}>
        <ZoomIn size={13} />
      </Button>
      <Button variant="ghost" size="icon-xs" className="rounded-full" onClick={onReset} disabled={scale === ZOOM_DEFAULT}>
        <RotateCcw size={12} />
      </Button>
    </div>
  )
}

function LazyPage({ pageNumber, scale, width, className }: {
  pageNumber: number
  scale: number
  width?: number
  className?: string
}) {
  const [isVisible, setIsVisible] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setIsVisible(true)
          // Once it's visible, we can stop observing if we want to keep it rendered,
          // or keep observing to unmount when it leaves. Let's keep it rendered for better UX.
          observer.disconnect()
        }
      },
      {
        rootMargin: '400px', // Load pages 400px before they enter viewport
      }
    )

    if (containerRef.current) {
      observer.observe(containerRef.current)
    }

    return () => observer.disconnect()
  }, [])

  return (
    <div
      ref={containerRef}
      className={isVisible ? className : "mb-4"}
      style={{
        // Use a neutral minimum height for the placeholder, then let content dictate height
        minHeight: isVisible ? 'auto' : '200px',
      }}
    >
      {isVisible ? (
        <Page
          pageNumber={pageNumber}
          scale={scale}
          width={width}
          loading={<LoadingSpinner text="渲染中" className="p-8" iconClassName="h-6 w-6" />}
        />
      ) : (
        <div className="flex items-center justify-center h-full text-muted-foreground/30 text-[10px] bg-muted/5 rounded-sm border border-dashed border-border/20">
          第 {pageNumber} 页
        </div>
      )}
    </div>
  )
}

function PdfRenderer({ url }: { url: string }) {
  const [numPages, setNumPages] = useState(0)
  const [scale, setScale] = useState(ZOOM_DEFAULT)
  const [appliedScale, setAppliedScale] = useState(ZOOM_DEFAULT)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [loadProgress, setLoadProgress] = useState(0)
  const [containerWidth, setContainerWidth] = useState<number | undefined>()
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const element = containerRef.current
    if (!element) return

    const observer = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (entry) {
        // contentRect 已是 padding 内的可用宽度，不要再减（曾双重扣除
        // 导致画布偏窄、页面视觉上贴左）
        setContainerWidth(entry.contentRect.width)
      }
    })

    observer.observe(element)
    return () => observer.disconnect()
  }, [])

  const fileConfig = useMemo(() => ({
    url,
    httpHeaders: authHeaders(),
  }), [url])

  const handleZoomIn = useCallback(() => {
    setScale((s) => Math.min(s + ZOOM_STEP, ZOOM_MAX))
  }, [])

  const handleZoomOut = useCallback(() => {
    setScale((s) => Math.max(s - ZOOM_STEP, ZOOM_MIN))
  }, [])

  const handleReset = useCallback(() => {
    setScale(ZOOM_DEFAULT)
    setAppliedScale(ZOOM_DEFAULT)
  }, [])

  useEffect(() => {
    const timer = setTimeout(() => setAppliedScale(scale), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [scale])

  const handleLoadError = useCallback((error: Error) => {
    const msg = error.message || 'PDF 加载失败'
    setLoadError(msg)
    showErrorToast(msg)
  }, [])

  if (loadError) return <ErrorState message={loadError} />

  return (
    <div className="flex flex-col h-full">
      <div ref={containerRef} className="flex-1 overflow-auto p-4">
        <Document
          file={fileConfig}
          onLoadSuccess={({ numPages: n }) => setNumPages(n)}
          onLoadError={handleLoadError}
          onLoadProgress={({ loaded, total }) => {
            if (total > 0) setLoadProgress((loaded / total) * 100)
          }}
          loading={<LoadingSpinner text="加载 PDF 中" progress={loadProgress} className="p-20" />}
        >
          <div className="flex flex-col items-center">
            {Array.from({ length: numPages }, (_, i) => (
              <LazyPage
                key={i}
                pageNumber={i + 1}
                scale={appliedScale}
                width={containerWidth}
                className="mb-4 shadow-sm bg-white"
              />
            ))}
          </div>
        </Document>
      </div>
      <div className="flex-none px-4 py-2 border-t border-border/30 flex items-center justify-center">
        <ZoomControls scale={scale} onZoomIn={handleZoomIn} onZoomOut={handleZoomOut} onReset={handleReset} />
        {numPages > 0 && (
          <span className="text-[10px] text-muted-foreground ml-3">{numPages} 页</span>
        )}
      </div>
    </div>
  )
}

function ErrorState({ message }: { message: string }) {
  return (
    <div className="flex flex-col items-center justify-center h-full text-center p-8">
      <div className="w-14 h-14 rounded-2xl bg-destructive/10 flex items-center justify-center mb-3">
        <AlertTriangle size={24} className="text-destructive/70" />
      </div>
      <p className="text-sm text-muted-foreground">{message}</p>
    </div>
  )
}

function DocxRenderer({ url }: { url: string }) {
  const containerRef = useRef<HTMLDivElement>(null)
  const scrollRef = useRef<HTMLDivElement>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [scale, setScale] = useState(ZOOM_DEFAULT)
  // fit-to-width：面板宽 / 纸张自然宽（Letter 816px 塞进 ~485px 的
  // 预览面板曾溢出 300+px 出横向滚动条；用 CSS zoom 缩到面板宽，
  // zoom 参与布局故不会留白/溢出，现代浏览器均已支持）
  const [containerW, setContainerW] = useState(0)
  const [pageW, setPageW] = useState(0)

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const ro = new ResizeObserver((entries) => {
      const e = entries[0]
      if (e) setContainerW(e.contentRect.width)
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  // 渲染完成后测纸张自然宽（loading 变 false 时容器已填充）
  useEffect(() => {
    if (loading) return
    const sec = containerRef.current?.querySelector('section.docx') as HTMLElement | null
    if (sec) setPageW(sec.getBoundingClientRect().width / (zoomRef.current || 1))
  }, [loading])

  useEffect(() => {
    const container = containerRef.current
    if (!container) return
    let destroyed = false
    setLoading(true)

    async function render() {
      try {
        const { data: blob } = await apiClient.get<Blob>(url, { responseType: 'blob' })
        if (destroyed) return

        const { renderAsync } = await import('docx-preview')
        if (destroyed || !container) return

        container.innerHTML = ''
        // 标准用法：inWrapper 保留库的页面包装（按文档自身的页面尺寸与
        // 页边距分页排版）；灰色底由下方 CSS 覆盖为透明。
        // 之前 inWrapper:false + 一堆 !important 行高/段距/宽度覆盖，
        // 把文档自己的排版全部顶掉了——格式全乱的根源。
        await renderAsync(blob, container, undefined, {
          inWrapper: true,
          ignoreHeight: false,
          ignoreWidth: false,
          ignoreFonts: false,
          breakPages: true,
          useBase64URL: true,
        })
        if (!destroyed) setLoading(false)
      } catch (err) {
        if (destroyed) return
        const msg = err instanceof Error ? err.message : 'DOCX 预览失败'
        setError(msg)
        showErrorToast(msg)
        setLoading(false)
      }
    }

    render()
    return () => { destroyed = true }
  }, [url])

  const handleZoomIn = useCallback(() => setScale((s) => Math.min(s + ZOOM_STEP, ZOOM_MAX)), [])
  const handleZoomOut = useCallback(() => setScale((s) => Math.max(s - ZOOM_STEP, ZOOM_MIN)), [])
  const handleReset = useCallback(() => setScale(ZOOM_DEFAULT), [])

  const fitScale = pageW > 0 && containerW > 0 ? Math.min(1, containerW / pageW) : 1
  const zoom = fitScale * scale
  const zoomRef = useRef(1)
  zoomRef.current = zoom

  if (error) return <ErrorState message={error} />

  return (
    <div className="flex flex-col h-full bg-background overflow-x-hidden relative">
      {/* Clean styles strictly targeted at the output document */}
      <style>{`
        /* 只装饰页面底板；段落/字体/表格等文档自身排版一律由
           docx-preview 按原始 docx 样式生成，不做任何覆盖 */
        .docx-wrapper {
          background: transparent !important;
          padding: 0 !important;
          display: flex !important;
          flex-direction: column !important;
          align-items: center !important;
          gap: 1rem !important;
        }
        .docx-wrapper > section.docx {
          background: #fff !important;
          box-shadow: 0 8px 32px rgba(0, 0, 0, 0.10) !important;
          border-radius: 4px !important;
          margin: 0 !important;
          color: #1f2328 !important;
        }
      `}</style>
      
      <div ref={scrollRef} className="flex-1 overflow-y-auto overflow-x-hidden p-4 md:p-8">
        {loading && <LoadingSpinner text="解析文档中" className="mt-12" />}
        <div
          className="flex justify-center"
          style={{ display: loading ? 'none' : 'flex' }}
        >
          <div style={{
            // CSS zoom 参与布局（区别于 transform）：fit-to-width 缩放后
            // 盒子尺寸同步收缩，居中与纵向滚动都自然成立
            zoom,
          }}>
            <div ref={containerRef} />
          </div>
        </div>
      </div>

      {!loading && (
        <div className="flex-none px-4 py-2 border-t border-border/30 flex items-center justify-center bg-background/80 backdrop-blur-sm relative z-10">
          <ZoomControls scale={scale} onZoomIn={handleZoomIn} onZoomOut={handleZoomOut} onReset={handleReset} />
        </div>
      )}
    </div>
  )
}

function TxtRenderer({ url }: { url: string }) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false

    apiClient.get(url, { responseType: 'text' }).then((res) => {
      if (!cancelled) setText(res.data as string)
    }).catch((err) => {
      if (cancelled) return
      const msg = err instanceof Error ? err.message : 'TXT 预览失败'
      setError(msg)
      showErrorToast(msg)
    })

    return () => { cancelled = true }
  }, [url])

  if (error) return <ErrorState message={error} />
  if (text === null) return <div className="flex items-center justify-center h-full text-sm text-muted-foreground">加载中...</div>

  return (
    <pre className="flex-1 overflow-auto p-6 text-[13px] leading-relaxed font-mono whitespace-pre-wrap break-words text-foreground/80">
      {text}
    </pre>
  )
}

function UnsupportedRenderer({ type }: { type: string }) {
  const ext = type.includes('/') ? type.split('/').pop() ?? type : type
  return (
    <div className="flex flex-col items-center justify-center h-full text-center p-8">
      <div className="w-16 h-16 rounded-2xl bg-accent/10 flex items-center justify-center mb-4">
        <FileText size={28} className="text-accent-foreground/60" />
      </div>
      <p className="text-sm text-muted-foreground">.{ext} 文件暂不支持预览</p>
    </div>
  )
}

function PreviewContent({ doc, url }: { doc: ProjectDocument; url: string }) {
  const type = doc.type.toLowerCase()

  if (type === 'application/pdf') return <PdfRenderer url={url} />
  if (type === 'application/vnd.openxmlformats-officedocument.wordprocessingml.document') return <DocxRenderer url={url} />
  if (type === 'text/plain') return <TxtRenderer url={url} />
  return <UnsupportedRenderer type={type} />
}

export function DocumentPreviewPanel({ document: doc, onClose }: DocumentPreviewPanelProps) {
  const isGuest = useSessionStore((state) => state.isGuest)
  const [guestPreview, setGuestPreview] = useState<{ documentId: string; url: string; error?: string } | null>(null)

  useEffect(() => {
    let disposed = false
    let objectUrl = ''
    if (!isGuest) return
    void fetchDocumentPreviewBlob(doc.id).then((blob) => {
      if (disposed) return
      objectUrl = URL.createObjectURL(blob)
      setGuestPreview({ documentId: doc.id, url: objectUrl })
    }).catch((error: unknown) => {
      if (disposed) return
      const message = error instanceof Error ? error.message : '本地文件读取失败'
      setGuestPreview({ documentId: doc.id, url: '', error: message })
      showErrorToast(message)
    })
    return () => {
      disposed = true
      if (objectUrl) URL.revokeObjectURL(objectUrl)
    }
  }, [doc.id, isGuest])

  const guestResult = guestPreview?.documentId === doc.id ? guestPreview : null
  const previewUrl = isGuest ? guestResult?.url ?? '' : buildPreviewUrl(doc.id)
  const previewError = guestResult?.error ?? ''

  return (
    <div className="flex flex-col h-full">
      <div className="h-10 border-b border-border/40 flex-none flex items-center justify-between px-4 bg-card/50 backdrop-blur-sm">
        <div className="flex items-center gap-2 min-w-0">
          <FileText size={13} className="text-accent-foreground/60 flex-none" />
          <span className="text-[11px] font-medium text-muted-foreground truncate">{doc.name}</span>
        </div>
        <Button variant="ghost" size="icon-xs" className="rounded-full" onClick={onClose}>
          <X size={14} />
        </Button>
      </div>
      <div className="flex-1 min-h-0 overflow-y-auto">
        {previewError ? <ErrorState message={previewError} /> : previewUrl ? <PreviewContent doc={doc} url={previewUrl} /> : <LoadingSpinner text="读取文件中" className="p-8" />}
      </div>
    </div>
  )
}
