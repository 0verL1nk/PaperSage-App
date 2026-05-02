import { useCallback, useMemo, useRef } from 'react'

const AUTO_SAVE_DELAY_MS = 1500

interface SaveMutation {
  mutate: (args: { filePath: string; content: string }) => void
  mutateAsync: (args: { filePath: string; content: string }) => Promise<unknown>
  isPending: boolean
}

export interface AutoSaveHandle {
  /** Record an editor content change and schedule a debounced save. */
  onChange: (content: string) => void
  /** Immediately save current content (used by Ctrl+S and pre-compile). */
  saveNow: () => Promise<void>
  /** Switch active file: saves pending changes first, then resets state. */
  switchFile: (filePath: string) => void
  /** Apply server content to refs — returns true if applied. */
  applyServerContent: (content: string) => boolean
  /** Current editor content from ref. */
  getContent: () => string
  /** Set the active file path (called during init). */
  setActiveFile: (filePath: string) => void
}

/**
 * Auto-save coordination hook that prevents two race conditions:
 *
 * 1. File-switch race: switching files no longer lets a stale timer
 *    write old content to the new file path.
 * 2. Auto-save overwrite: the debounced callback reads from refs at
 *    fire time, so it always saves the latest edits.
 */
export function useAutoSave(saveMutation: SaveMutation): AutoSaveHandle {
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastSavedContentRef = useRef('')
  const localEditsRef = useRef<string | null>(null)
  const activeFilePathRef = useRef<string | null>(null)
  const editorContentRef = useRef('')

  const scheduleAutoSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => {
      const filePath = activeFilePathRef.current
      const content = localEditsRef.current ?? editorContentRef.current
      if (filePath && content !== lastSavedContentRef.current) {
        saveMutation.mutate({ filePath, content })
        lastSavedContentRef.current = content
      }
    }, AUTO_SAVE_DELAY_MS)
  }, [saveMutation])

  const onChange = useCallback((content: string) => {
    localEditsRef.current = content
    editorContentRef.current = content
    scheduleAutoSave()
  }, [scheduleAutoSave])

  const saveNow = useCallback(async () => {
    const filePath = activeFilePathRef.current
    const content = localEditsRef.current ?? editorContentRef.current
    if (!filePath || content === lastSavedContentRef.current) return
    if (saveTimerRef.current) {
      clearTimeout(saveTimerRef.current)
      saveTimerRef.current = null
    }
    await saveMutation.mutateAsync({ filePath, content })
    lastSavedContentRef.current = content
  }, [saveMutation])

  const switchFile = useCallback((filePath: string) => {
    if (filePath === activeFilePathRef.current) return

    // Save pending changes for current file
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = null
    const currentContent = localEditsRef.current ?? editorContentRef.current
    const currentPath = activeFilePathRef.current
    if (currentPath && currentContent !== lastSavedContentRef.current) {
      saveMutation.mutate({ filePath: currentPath, content: currentContent })
      lastSavedContentRef.current = currentContent
    }

    // Reset refs immediately — prevents stale timer from writing to new file
    localEditsRef.current = null
    editorContentRef.current = ''
    activeFilePathRef.current = filePath
  }, [saveMutation])

  const applyServerContent = useCallback((content: string): boolean => {
    lastSavedContentRef.current = content
    if (localEditsRef.current === null) {
      editorContentRef.current = content
      return true
    }
    return false
  }, [])

  const getContent = useCallback(() => editorContentRef.current, [])

  const setActiveFile = useCallback((filePath: string) => {
    activeFilePathRef.current = filePath
  }, [])

  return useMemo<AutoSaveHandle>(
    () => ({ onChange, saveNow, switchFile, applyServerContent, getContent, setActiveFile }),
    [onChange, saveNow, switchFile, applyServerContent, getContent, setActiveFile],
  )
}
