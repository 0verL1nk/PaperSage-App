import { useCallback, useEffect, useMemo, useRef } from 'react'
import type { editor } from 'monaco-editor'

import type { CompileIssue } from '@/lib/latex/compile-log'

/**
 * Monaco 编辑器公共原语：报错行内标记 + 跳行。HELE-47 的 SyncTeX 定位复用
 * 同一实例与 reveal；不要在页面里另存一份 editor ref。
 */

const DECORATION_LIMIT = 200
const CONTENT_REFRESH_DEBOUNCE_MS = 250

export interface UseLatexEditorOptions {
  activeFilePath: string | null
  issues: CompileIssue[]
}

export interface LatexEditorHandle {
  onMount: (instance: editor.IStandaloneCodeEditor) => void
  /** 跳转并聚焦到指定行；编辑器未就绪时返回 false。 */
  reveal: (line: number) => boolean
  refresh: () => void
}

function issueDecoration(issue: CompileIssue, endColumn: number): editor.IModelDeltaDecoration {
  const isError = issue.severity === 'error'
  return {
    range: {
      startLineNumber: issue.line!,
      startColumn: 1,
      endLineNumber: issue.line!,
      endColumn,
    },
    options: {
      isWholeLine: true,
      className: isError ? 'latex-issue-line-error' : 'latex-issue-line-warning',
      inlineClassName: isError ? 'latex-issue-inline-error' : 'latex-issue-inline-warning',
      glyphMarginClassName: isError ? 'latex-issue-glyph-error' : 'latex-issue-glyph-warning',
      hoverMessage: { value: `${isError ? '编译错误' : '编译警告'}：${issue.message}` },
      overviewRuler: {
        color: isError ? '#dc2626' : '#d97706',
        position: 4, // Monaco OverviewRulerLane.Right
      },
    },
  }
}

export function useLatexEditor({ activeFilePath, issues }: UseLatexEditorOptions): LatexEditorHandle {
  const instanceRef = useRef<editor.IStandaloneCodeEditor | null>(null)
  const collectionRef = useRef<editor.IEditorDecorationsCollection | null>(null)
  const contentListenerRef = useRef<{ dispose: () => void } | null>(null)
  const refreshTimerRef = useRef<number | null>(null)
  const issuesRef = useRef(issues)
  const activeFileRef = useRef(activeFilePath)

  const refresh = useCallback(() => {
    const instance = instanceRef.current
    const collection = collectionRef.current
    const model = instance?.getModel()
    if (!instance || !collection || !model) return
    const file = activeFileRef.current
    const decorations: editor.IModelDeltaDecoration[] = []
    for (const issue of issuesRef.current) {
      if (file === null || issue.filePath !== file || issue.line === null) continue
      if (issue.line > model.getLineCount()) continue
      decorations.push(issueDecoration(issue, model.getLineMaxColumn(issue.line)))
      if (decorations.length >= DECORATION_LIMIT) break
    }
    collection.set(decorations)
  }, [])

  const reveal = useCallback((line: number): boolean => {
    const instance = instanceRef.current
    if (!instance) return false
    instance.revealLineInCenter(line)
    instance.setPosition({ lineNumber: line, column: 1 })
    instance.focus()
    return true
  }, [])

  const onMount = useCallback((instance: editor.IStandaloneCodeEditor) => {
    instanceRef.current = instance
    collectionRef.current = instance.createDecorationsCollection([])
    // `setValue`（外部内容同步）会清空 decoration；监听内容变化后重建。
    contentListenerRef.current?.dispose()
    contentListenerRef.current = instance.onDidChangeModelContent(() => {
      if (refreshTimerRef.current !== null) window.clearTimeout(refreshTimerRef.current)
      refreshTimerRef.current = window.setTimeout(() => {
        refreshTimerRef.current = null
        refresh()
      }, CONTENT_REFRESH_DEBOUNCE_MS)
    })
    refresh()
  }, [refresh])

  useEffect(() => {
    issuesRef.current = issues
    activeFileRef.current = activeFilePath
    refresh()
  }, [issues, activeFilePath, refresh])

  useEffect(() => () => {
    if (refreshTimerRef.current !== null) window.clearTimeout(refreshTimerRef.current)
    contentListenerRef.current?.dispose()
    collectionRef.current?.clear()
  }, [])

  return useMemo(() => ({ onMount, reveal, refresh }), [onMount, reveal, refresh])
}
