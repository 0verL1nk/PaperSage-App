import { useCallback, useEffect, useRef, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'

import * as agentApi from '@/services/agent-api'
import { describeAgentError } from '@/services/agent-errors'
import type { ParagraphFocus } from '@/lib/latex/paragraph'

/**
 * 动态写作建议的触发层。
 *
 * 默认「专注写作」：不自动建 Run，只有用户点「检查本段」才发请求。
 * Opt-In 自动检查默认关闭；开启后：
 * - 段落完成（光标离开刚编辑的段落 / 空行 / 标题）→ 3s 后检查该段；
 * - 仍停在本段继续写 → 停止输入 8s 后才检查。
 * 不在连续输入中弹窗、不抢焦点、不 toast，结果只进侧栏徽标。
 */

const AUTO_STORAGE_KEY = 'papersage.writing-suggestion.auto'
export const AUTO_CHECK_IDLE_MS = 8000
export const AUTO_PARAGRAPH_DONE_MS = 3000

export interface SuggestionFocusTarget {
  filePath: string
  focus: ParagraphFocus
}

export type SuggestionNotice = { kind: 'ok' | 'skip' | 'error'; text: string }

const SKIP_TEXT: Record<agentApi.SuggestionSkipReason, string> = {
  pending: '本段已有待处理建议，先在「建议」里处理完再检查。',
  duplicate: '本段刚检查过，先处理已有建议或修改后再试。',
  running: '正在检查上一段，稍等片刻再试。',
  rate_limited: '检查太频繁了，休息一下再试。',
  too_short: '这一段还太短，写完一两句再检查。',
}

export function useWritingSuggestion(
  projectId: string,
  getFocus: () => SuggestionFocusTarget | null,
  enabled: boolean,
) {
  const queryClient = useQueryClient()
  const proposalsQuery = useQuery({
    queryKey: ['agent-proposals', projectId, 'visible'],
    // 不带 status：待处理之外还要保留刚应用的卡片，用户才能一键撤销。
    queryFn: () => agentApi.listProposals(projectId),
    enabled: enabled && !!projectId,
  })
  const [autoCheck, setAutoCheckState] = useState(() => {
    if (typeof window === 'undefined') return false
    return window.localStorage.getItem(AUTO_STORAGE_KEY) === '1'
  })
  const [checking, setChecking] = useState(false)
  const [notice, setNotice] = useState<SuggestionNotice | null>(null)
  const [lastCheckedText, setLastCheckedText] = useState('')
  const lastEdited = useRef<SuggestionFocusTarget | null>(null)
  const autoChecked = useRef(new Set<string>())
  const idleTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const doneTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const checkingRef = useRef(false)
  const streamRef = useRef<AbortController | null>(null)

  // 列表只展示待处理与最近 24 小时内已应用/已撤销的建议；其余状态留在审计里。
  const visible = (proposalsQuery.data ?? []).filter((p) => {
    if (p.status === 'pending') return true
    if (p.status !== 'applied' && p.status !== 'reverted') return false
    if (!p.createdAt) return true
    return Date.now() - new Date(p.createdAt).getTime() < 24 * 60 * 60 * 1000
  })
  const pending = visible.filter((p) => p.status === 'pending')

  const clearTimers = useCallback(() => {
    if (idleTimer.current) clearTimeout(idleTimer.current)
    if (doneTimer.current) clearTimeout(doneTimer.current)
    idleTimer.current = null
    doneTimer.current = null
  }, [])

  const setAutoCheck = useCallback((next: boolean) => {
    setAutoCheckState(next)
    if (!next) clearTimers()
    if (typeof window !== 'undefined') {
      window.localStorage.setItem(AUTO_STORAGE_KEY, next ? '1' : '0')
    }
  }, [clearTimers])

  const invalidate = useCallback(
    () => queryClient.invalidateQueries({ queryKey: ['agent-proposals', projectId] }),
    [projectId, queryClient],
  )

  const runCheck = useCallback(
    async (trigger: agentApi.SuggestionTrigger, override?: SuggestionFocusTarget) => {
      if (!enabled || checkingRef.current) return
      const target = override ?? getFocus()
      if (!target) {
        setNotice({ kind: 'skip', text: '把光标放进正文段落再检查；标题与公式不能作为写作建议的目标。' })
        return
      }
      const text = target.focus.paragraphText
      if ([...text.trim()].length < 20) {
        setNotice({ kind: 'skip', text: '这一段还太短，写完一两句再检查。' })
        return
      }
      // 同一段文本只自动检查一次；手动检查也登记，避免随后自动重复。
      autoChecked.current.add(`${target.filePath}\u0000${text}`)
      checkingRef.current = true
      setChecking(true)
      setNotice(null)
      const toolsSeen = new Set<string>()
      let sawProposal = false
      try {
        const result = await agentApi.createSuggestion(projectId, {
          filePath: target.filePath,
          paragraphText: text,
          sectionPath: target.focus.sectionPath,
          trigger,
          clientRequestId: crypto.randomUUID(),
        })
        if ('skipped' in result) {
          setNotice({ kind: 'skip', text: SKIP_TEXT[result.skipped] ?? '本次没有检查：稍后再试。' })
          return
        }
        const controller = new AbortController()
        streamRef.current = controller
        await agentApi.streamRun(result.runId, {
          signal: controller.signal,
          onEvent: (event) => {
            if (event.type === 'tool-input-available') {
              const name = typeof event.toolName === 'string' ? event.toolName : ''
              if (name) toolsSeen.add(name)
            }
            if (event.type === 'data-proposal') sawProposal = true
          },
        })
        setLastCheckedText(text)
        await invalidate()
        if (sawProposal) {
          setNotice({ kind: 'ok', text: '已给出建议，请在「建议」里查看依据并确认。' })
        } else if (toolsSeen.has('skip_suggestion')) {
          setNotice({ kind: 'ok', text: '本段暂无需要修改的问题。' })
        } else {
          setNotice({ kind: 'error', text: '本次没有产出可应用的结构化建议，可稍后重试。' })
        }
      } catch (error) {
        setNotice({ kind: 'error', text: describeAgentError(error) })
      } finally {
        streamRef.current = null
        checkingRef.current = false
        setChecking(false)
      }
    },
    [enabled, getFocus, invalidate, projectId],
  )

  const checkNow = useCallback(() => runCheck('manual'), [runCheck])

  const autoCheckParagraph = useCallback(
    (target: SuggestionFocusTarget) => {
      if (!enabled || !autoCheck || checkingRef.current) return
      if ([...target.focus.paragraphText.trim()].length < 20) return
      if (autoChecked.current.has(`${target.filePath}\u0000${target.focus.paragraphText}`)) return
      void runCheck('auto', target)
    },
    [autoCheck, enabled, runCheck],
  )

  /**
   * 编辑器每次内容变化调用：记住最后编辑的段落，并排一个 8s 段内空闲检查。
   * 用户继续在段内输入会重置计时；光标离开该段则由 notifyCursorMove 走 3s。
   */
  const notifyActivity = useCallback(() => {
    if (!enabled || !autoCheck) return
    const target = getFocus()
    if (!target) return
    lastEdited.current = target
    if (idleTimer.current) clearTimeout(idleTimer.current)
    idleTimer.current = setTimeout(() => {
      idleTimer.current = null
      const current = lastEdited.current
      const now = getFocus()
      if (!current || !now) return
      if (now.filePath !== current.filePath || now.focus.paragraphText !== current.focus.paragraphText) return
      autoCheckParagraph(current)
    }, AUTO_CHECK_IDLE_MS)
  }, [autoCheck, enabled, autoCheckParagraph, getFocus])

  /**
   * 光标移动时调用：离开刚编辑的段落即视为段落完成，3s 后检查该段。
   * 仍在同一段内移动不会触发。
   */
  const notifyCursorMove = useCallback(() => {
    if (!enabled || !autoCheck) return
    const edited = lastEdited.current
    if (!edited) return
    const now = getFocus()
    if (now && now.filePath === edited.filePath && now.focus.paragraphText === edited.focus.paragraphText) return
    if (idleTimer.current) {
      clearTimeout(idleTimer.current)
      idleTimer.current = null
    }
    if (doneTimer.current) clearTimeout(doneTimer.current)
    doneTimer.current = setTimeout(() => {
      doneTimer.current = null
      // 3s 内用户又回到该段继续写：撤销「段落完成」，交给 8s 段内计时。
      const current = lastEdited.current ?? edited
      const now = getFocus()
      if (now && now.filePath === current.filePath && now.focus.paragraphText === current.focus.paragraphText) return
      autoCheckParagraph(current)
    }, AUTO_PARAGRAPH_DONE_MS)
  }, [autoCheck, enabled, autoCheckParagraph, getFocus])

  useEffect(
    () => () => {
      if (idleTimer.current) clearTimeout(idleTimer.current)
      if (doneTimer.current) clearTimeout(doneTimer.current)
      streamRef.current?.abort()
    },
    [],
  )

  return {
    pending,
    visible,
    pendingCount: pending.length,
    checking,
    notice,
    lastCheckedText,
    autoCheck,
    setAutoCheck,
    checkNow,
    notifyActivity,
    notifyCursorMove,
    refetch: proposalsQuery.refetch,
    isLoading: proposalsQuery.isLoading,
    isError: proposalsQuery.isError,
  }
}
