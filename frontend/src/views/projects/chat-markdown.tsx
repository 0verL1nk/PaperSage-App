import { Component, isValidElement, memo, useEffect, useRef, useState, type ReactNode } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Check, Copy } from 'lucide-react'

import { cn } from '@/lib/utils'

/**
 * Assistant Markdown 渲染。
 *
 * 只做 GFM 语义化展示：不启用 raw HTML（`skipHtml`），不引入高亮/KaTeX。
 * 解析或渲染异常时退化为纯文本，避免消息区白屏。
 */
export const ChatMarkdown = memo(function ChatMarkdown({
  text,
  streaming = false,
  className,
}: {
  text: string
  streaming?: boolean
  className?: string
}) {
  return (
    <div
      className={cn('min-w-0 text-[14px] leading-6 text-foreground', className)}
      aria-live={streaming ? 'polite' : undefined}
      aria-atomic={streaming ? false : undefined}
    >
      <MarkdownBoundary fallback={<p className="whitespace-pre-wrap break-words">{text}</p>}>
        <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml components={components}>
          {text}
        </ReactMarkdown>
      </MarkdownBoundary>
      {streaming && (
        <span
          aria-hidden="true"
          data-streaming-cursor=""
          className="mt-1 inline-block h-4 w-[2px] animate-pulse rounded-full bg-foreground/60 align-middle"
        />
      )}
    </div>
  )
})

class MarkdownBoundary extends Component<{ fallback: ReactNode; children: ReactNode }, { failed: boolean }> {
  state = { failed: false }

  static getDerivedStateFromError() {
    return { failed: true }
  }

  render() {
    return this.state.failed ? this.props.fallback : this.props.children
  }
}

function textContent(node: ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textContent).join('')
  if (isValidElement<{ children?: ReactNode }>(node)) return textContent(node.props.children)
  return ''
}

const components: Components = {
  h1: ({ children }) => <h1 className="mb-2 mt-5 text-[18px] font-semibold leading-7 first:mt-0">{children}</h1>,
  h2: ({ children }) => <h2 className="mb-2 mt-5 text-[16px] font-semibold leading-6 first:mt-0">{children}</h2>,
  h3: ({ children }) => <h3 className="mb-1.5 mt-4 text-[15px] font-semibold leading-6 first:mt-0">{children}</h3>,
  h4: ({ children }) => <h4 className="mb-1.5 mt-4 text-[14px] font-semibold leading-6 first:mt-0">{children}</h4>,
  p: ({ children }) => (
    <p className="my-3 break-words [overflow-wrap:anywhere] first:mt-0 last:mb-0">{children}</p>
  ),
  ul: ({ children }) => <ul className="my-3 list-disc space-y-1 pl-5 first:mt-0 last:mb-0">{children}</ul>,
  ol: ({ children }) => <ol className="my-3 list-decimal space-y-1 pl-5 first:mt-0 last:mb-0">{children}</ol>,
  li: ({ children }) => <li className="break-words pl-0.5 [&>p]:my-0">{children}</li>,
  blockquote: ({ children }) => (
    <blockquote className="my-3 border-l-2 border-primary/30 pl-3 text-muted-foreground [&>p]:my-0">
      {children}
    </blockquote>
  ),
  a: ({ href, children }) => (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="break-words text-primary underline underline-offset-2 [overflow-wrap:anywhere] hover:text-primary/80"
    >
      {children}
    </a>
  ),
  hr: () => <hr className="my-4 border-border" />,
  code: ({ children }) => (
    <code className="break-words rounded bg-muted px-1.5 py-0.5 font-mono text-[0.875em] [overflow-wrap:anywhere]">
      {children}
    </code>
  ),
  pre: ({ children }) => {
    const code = isValidElement<{ className?: string }>(children) ? children : null
    const language = /language-([\w+#-]+)/.exec(code?.props.className ?? '')?.[1] ?? ''
    return <CodeBlock code={textContent(children).replace(/\n$/, '')} language={language} />
  },
  table: ({ children }) => (
    <div className="my-3 w-full max-w-full overflow-x-auto rounded-lg border border-border/60">
      <table className="w-full min-w-max border-collapse text-[13px] leading-5">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="bg-muted/60">{children}</thead>,
  tr: ({ children }) => <tr className="border-b border-border/60 last:border-0">{children}</tr>,
  th: ({ children }) => (
    <th className="border-r border-border/60 px-2.5 py-1.5 text-left font-medium last:border-r-0">{children}</th>
  ),
  td: ({ children }) => (
    <td className="border-r border-border/60 px-2.5 py-1.5 align-top last:border-r-0">{children}</td>
  ),
}

function CodeBlock({ code, language }: { code: string; language: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')
  const timer = useRef<number | undefined>(undefined)

  useEffect(() => () => window.clearTimeout(timer.current), [])

  const copy = async () => {
    window.clearTimeout(timer.current)
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
      await navigator.clipboard.writeText(code)
      setState('copied')
    } catch {
      setState('failed')
    }
    timer.current = window.setTimeout(() => setState('idle'), 1600)
  }

  return (
    <div className="my-3 overflow-hidden rounded-xl border border-border/60 bg-muted/30">
      <div className="flex items-center justify-between gap-2 border-b border-border/60 bg-muted/50 px-3 py-1.5">
        <span className="font-mono text-[11px] uppercase tracking-wide text-muted-foreground">
          {language || 'text'}
        </span>
        <button
          type="button"
          onClick={() => void copy()}
          className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-muted-foreground transition-colors hover:bg-background/60 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
          aria-label={state === 'copied' ? '已复制代码' : state === 'failed' ? '复制失败' : '复制代码'}
        >
          {state === 'copied' ? <Check size={12} /> : <Copy size={12} />}
          <span aria-live="polite">{state === 'copied' ? '已复制' : state === 'failed' ? '复制失败' : '复制'}</span>
        </button>
      </div>
      <pre className="overflow-x-auto p-3 text-[12.5px] leading-5">
        <code className="font-mono">{code}</code>
      </pre>
    </div>
  )
}
