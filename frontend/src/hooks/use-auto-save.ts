import { useCallback, useMemo, useRef } from 'react'

const AUTO_SAVE_DELAY_MS = 1500

interface SaveMutation {
  mutateAsync: (args: { filePath: string; content: string }) => Promise<unknown>
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

interface SaveFailure {
  content: string
  error: Error
  reported: boolean
}

export function useAutoSave(saveMutation: SaveMutation): AutoSaveHandle {
  const saveTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const savedContentRef = useRef(new Map<string, string>())
  const draftsRef = useRef(new Map<string, string>())
  const failuresRef = useRef(new Map<string, SaveFailure>())
  const queuedContentRef = useRef(new Map<string, { content: string }>())
  const queueRef = useRef<Promise<void> | null>(null)
  const saveNowRef = useRef<Promise<void> | null>(null)
  const flushFailureRef = useRef<Error | null>(null)
  const localEditsRef = useRef<string | null>(null)
  const activeFilePathRef = useRef<string | null>(null)
  const editorContentRef = useRef('')

  const queueSave = useCallback((filePath: string, content: string, force = false) => {
    const queued = queuedContentRef.current.get(filePath)
    if (queued?.content === content) return
    if (!force && !queued && !failuresRef.current.has(filePath) && content === (savedContentRef.current.get(filePath) ?? '')) {
      if (draftsRef.current.get(filePath) === content) draftsRef.current.delete(filePath)
      return
    }

    const entry = { content }
    queuedContentRef.current.set(filePath, entry)
    const write = () => saveMutation.mutateAsync({ filePath, content })
    // Serialize writes so an older request cannot overwrite a newer one.
    let request: Promise<unknown>
    try {
      request = queueRef.current ? queueRef.current.then(write) : write()
    } catch (error) {
      request = Promise.reject(error)
    }
    const operation = request.then(
      () => {
        savedContentRef.current.set(filePath, content)
        failuresRef.current.delete(filePath)
        if (queuedContentRef.current.get(filePath) === entry && draftsRef.current.get(filePath) === content) {
          draftsRef.current.delete(filePath)
        }
      },
      (reason: unknown) => {
        const message = reason instanceof Error ? reason.message : String(reason)
        const error = new Error(`Could not save ${filePath}: ${message}. Reopen the file and save it before continuing.`)
        failuresRef.current.set(filePath, { content, error, reported: false })
        if (saveNowRef.current && !flushFailureRef.current) flushFailureRef.current = error
      },
    )
    queueRef.current = operation
    void operation.then(() => {
      if (queueRef.current === operation) queueRef.current = null
      if (queuedContentRef.current.get(filePath) === entry) queuedContentRef.current.delete(filePath)
    })
  }, [saveMutation])

  const scheduleAutoSave = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = setTimeout(() => {
      saveTimerRef.current = null
      const filePath = activeFilePathRef.current
      const content = localEditsRef.current ?? editorContentRef.current
      if (filePath) queueSave(filePath, content)
    }, AUTO_SAVE_DELAY_MS)
  }, [queueSave])

  const onChange = useCallback((content: string) => {
    localEditsRef.current = content
    editorContentRef.current = content
    if (activeFilePathRef.current) draftsRef.current.set(activeFilePathRef.current, content)
    scheduleAutoSave()
  }, [scheduleAutoSave])

  const saveNow = useCallback((): Promise<void> => {
    if (saveNowRef.current) return saveNowRef.current
    // Report an unseen failure before allowing an explicit retry of that draft.
    const retryable = new Map([...failuresRef.current].filter(([, failure]) => failure.reported))
    flushFailureRef.current = [...failuresRef.current.values()].find((failure) => !failure.reported)?.error ?? null

    const flush = async () => {
      while (true) {
        if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
        saveTimerRef.current = null
        const pending = queueRef.current
        if (pending) await pending
        if (queueRef.current && queueRef.current !== pending) continue

        const filePath = activeFilePathRef.current
        const draft = filePath ? draftsRef.current.get(filePath) : undefined
        const failure = filePath ? failuresRef.current.get(filePath) : undefined
        if (filePath && draft !== undefined) {
          if (failure && (retryable.get(filePath) === failure || draft !== failure.content)) {
            retryable.delete(filePath)
            queueSave(filePath, draft, true)
            continue
          }
          if (!failure && draft !== (savedContentRef.current.get(filePath) ?? '')) {
            queueSave(filePath, draft)
            continue
          }
          if (!failure) draftsRef.current.delete(filePath)
        }

        const failures = [...failuresRef.current.values()]
        failures.forEach((failure) => { failure.reported = true })
        const error = flushFailureRef.current ?? failures[0]?.error
        if (error) throw error
        return
      }
    }

    const operation = flush().finally(() => { saveNowRef.current = null })
    saveNowRef.current = operation
    return operation
  }, [queueSave])

  const switchFile = useCallback((filePath: string) => {
    if (filePath === activeFilePathRef.current) return
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current)
    saveTimerRef.current = null
    const currentPath = activeFilePathRef.current
    const draft = currentPath ? draftsRef.current.get(currentPath) : undefined
    if (currentPath && draft !== undefined) queueSave(currentPath, draft)

    const nextDraft = draftsRef.current.get(filePath)
    localEditsRef.current = nextDraft ?? null
    editorContentRef.current = nextDraft ?? ''
    activeFilePathRef.current = filePath
  }, [queueSave])

  const applyServerContent = useCallback((content: string): boolean => {
    if (activeFilePathRef.current) savedContentRef.current.set(activeFilePathRef.current, content)
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
