import { useState } from 'react'
import { AlertCircle, AlertTriangle, Copy, HardDrive, Loader2, Sparkles } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { compileFailureMessage, compileRawLogText, type CompileFailureKind } from '@/lib/latex/compile-error'
import { logExcerptForIssue, type CompileIssue } from '@/lib/latex/compile-log'
import { cn } from '@/lib/utils'
import {
  applyProposal,
  diagnoseCompileError,
  rejectProposal,
  type CompileDiagnosis,
} from '@/services/agent-api'
import type { DiagnosticStatus } from '@/store/compile-store'

export interface CompileFailurePanelProps {
  projectId: string
  error: string
  logs: string | null
  issues: CompileIssue[]
  rootFile: string | null
  compiler: string | null
  failureKind: CompileFailureKind
  diagnosticStatus: DiagnosticStatus | null
  diagnosticId: string | null
  isMobile: boolean
  isCompiling: boolean
  isGuest: boolean
  onRetryCompile: () => void
  onReinitResources: () => void
  onSelectIssue: (issue: CompileIssue) => void
  /** 应用修复前先落盘，保证服务端基准与用户所见一致。 */
  onBeforeApply: () => Promise<void>
  /** 修复应用成功后刷新文件树与内容。 */
  onApplied: (filePath: string) => void
}

interface DiagnosisState {
  status: 'idle' | 'loading' | 'ready' | 'error'
  result: CompileDiagnosis | null
  message: string
  decision: 'applied' | 'rejected' | null
}

const IDLE_DIAGNOSIS: DiagnosisState = { status: 'idle', result: null, message: '', decision: null }

function isNavigable(issue: CompileIssue): boolean {
  return issue.filePath !== null && issue.line !== null
}

function issueLocation(issue: CompileIssue): string {
  if (issue.filePath && issue.line !== null) return `${issue.filePath}:${issue.line}`
  if (issue.filePath) return issue.filePath
  if (issue.line !== null) return `第 ${issue.line} 行`
  return '位置未知'
}

function IssueRow({ issue, onSelectIssue, action }: {
  issue: CompileIssue
  onSelectIssue: (issue: CompileIssue) => void
  action?: React.ReactNode
}) {
  const isError = issue.severity === 'error'
  const icon = isError
    ? <AlertCircle size={14} className="mt-0.5 shrink-0 text-destructive" />
    : <AlertTriangle size={14} className="mt-0.5 shrink-0 text-amber-600" />
  const body = (
    <>
      {icon}
      <span className="min-w-0">
        <span className="block truncate font-mono text-[11px] text-muted-foreground">{issueLocation(issue)}</span>
        <span className={cn('mt-0.5 block break-words text-xs', isError ? 'text-destructive/90' : 'text-muted-foreground')}>
          {issue.message}
        </span>
        {!isNavigable(issue) && (
          <span className="mt-1 block text-[10px] text-muted-foreground">
            {issue.source === 'bibtex'
              ? 'BibTeX 日志只给出辅助文件位置，无法在稿件中跳转'
              : '日志未给出可定位的文件与行号，无法跳转'}
          </span>
        )}
      </span>
    </>
  )
  return (
    <li className="flex items-start gap-2 rounded-lg border border-border/40 bg-background/60 p-2">
      {isNavigable(issue) ? (
        <button
          type="button"
          onClick={() => onSelectIssue(issue)}
          title="跳转到报错位置"
          className="flex min-h-11 flex-1 items-start gap-2 rounded-md px-1 text-left transition-colors hover:bg-muted/50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {body}
        </button>
      ) : (
        <div className="flex min-h-11 flex-1 items-start gap-2 px-1">{body}</div>
      )}
      {action}
    </li>
  )
}

/** 成功但有警告：预览顶部一条可折叠摘要。 */
export function CompileWarningSummary({ issues, onSelectIssue }: {
  issues: CompileIssue[]
  onSelectIssue: (issue: CompileIssue) => void
}) {
  const warnings = issues.filter((issue) => issue.severity === 'warning')
  if (warnings.length === 0) return null
  return (
    <details className="mx-auto max-w-2xl rounded-lg border border-amber-500/25 bg-background/95 p-3 text-sm shadow-sm backdrop-blur">
      <summary className="flex min-h-11 cursor-pointer select-none items-center gap-2 font-medium">
        <AlertTriangle size={15} className="shrink-0 text-amber-600" />
        本次编译有 {warnings.length} 条警告
      </summary>
      <ul className="mt-2 space-y-1.5">
        {warnings.map((issue, index) => (
          <IssueRow key={`${issueLocation(issue)}-${index}`} issue={issue} onSelectIssue={onSelectIssue} />
        ))}
      </ul>
      <p className="mt-2 text-[11px] text-muted-foreground">警告通常不影响生成 PDF，但可能意味着引用、字体或排版需要检查。</p>
    </details>
  )
}

export function CompileFailurePanel({
  projectId,
  error,
  logs,
  issues,
  rootFile,
  compiler,
  failureKind,
  diagnosticStatus,
  diagnosticId,
  isMobile,
  isCompiling,
  isGuest,
  onRetryCompile,
  onReinitResources,
  onSelectIssue,
  onBeforeApply,
  onApplied,
}: CompileFailurePanelProps) {
  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [diagnosis, setDiagnosis] = useState<DiagnosisState>(IDLE_DIAGNOSIS)
  const [deciding, setDeciding] = useState(false)

  const rawLog = compileRawLogText(error, logs)
  const errors = issues.filter((issue) => issue.severity === 'error')
  const warnings = issues.filter((issue) => issue.severity === 'warning')
  const canDiagnose = !isGuest && compiler !== null && rootFile !== null

  const runDiagnosis = async (issue: CompileIssue, key: string) => {
    setActiveKey(key)
    setDiagnosis({ ...IDLE_DIAGNOSIS, status: 'loading' })
    try {
      const result = await diagnoseCompileError(projectId, {
        filePath: issue.filePath ?? '',
        line: issue.line ?? 0,
        errorMessage: issue.message,
        logExcerpt: logExcerptForIssue(logs ?? error, issue),
        compiler: compiler ?? '',
        rootFile: rootFile ?? '',
      })
      setDiagnosis({ status: 'ready', result, message: '', decision: null })
    } catch (cause) {
      setDiagnosis({
        status: 'error',
        result: null,
        message: cause instanceof Error && cause.message
          ? cause.message
          : 'AI 诊断失败，请稍后重试。',
        decision: null,
      })
    }
  }

  const decide = async (approve: boolean) => {
    const patch = diagnosis.result?.patch
    if (!patch) return
    setDeciding(true)
    try {
      if (approve) {
        // 用户所见内容先落盘，服务端才能用同一基准校验锚点。
        await onBeforeApply()
        await applyProposal(patch.proposalId)
        onApplied(patch.filePath)
        setDiagnosis((current) => ({ ...current, decision: 'applied' }))
        toast.success('修复已应用，请重新编译验证')
      } else {
        await rejectProposal(patch.proposalId)
        setDiagnosis((current) => ({ ...current, decision: 'rejected' }))
        toast.success('已忽略本次修复建议')
      }
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : '操作失败，请稍后重试。'
      // 409 是「稿件在诊断后已变化」，必须让用户重新诊断而不是拿旧锚点覆盖。
      const hint = /建议已失效|稿件已变化|stale/i.test(message) ? '稿件在诊断后已变化，请重新诊断后再应用。' : message
      toast.error(hint)
      setDiagnosis((current) => ({ ...current, status: 'error', message: hint }))
    } finally {
      setDeciding(false)
    }
  }

  return (
    <div className="mx-auto mt-16 max-w-lg select-text rounded-xl border border-destructive/20 bg-destructive/5 p-6">
      <div className="mb-3 flex items-center justify-between text-destructive">
        <div className="flex items-center gap-2">
          <AlertCircle size={16} />
          <h4 className="text-sm font-bold">编译错误</h4>
        </div>
        <button
          type="button"
          className={cn('inline-flex items-center gap-1 rounded text-[10px] text-muted-foreground transition-colors hover:bg-muted/50 hover:text-foreground', isMobile ? 'min-h-11 px-3' : 'px-2 py-1')}
          onClick={() => {
            void navigator.clipboard.writeText(rawLog)
            toast.success('已复制到剪贴板')
          }}
        >
          <Copy size={10} />
          复制
        </button>
      </div>
      <p className="text-sm leading-6 text-destructive/90">{compileFailureMessage(failureKind)}</p>

      {errors.length > 0 && (
        <div className="mt-3">
          <p className="mb-1.5 text-[11px] font-medium text-muted-foreground">定位到 {errors.length} 处错误</p>
          <ul className="max-h-72 space-y-1.5 overflow-y-auto pr-1">
            {errors.map((issue, index) => {
              const key = `error-${index}`
              return (
                <IssueRow
                  key={key}
                  issue={issue}
                  onSelectIssue={onSelectIssue}
                  action={(
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className={cn('shrink-0 gap-1', isMobile ? 'h-11' : 'h-7 px-2 text-[11px]')}
                      disabled={!canDiagnose || deciding}
                      title={isGuest ? '登录后可使用 AI 诊断' : undefined}
                      onClick={() => void runDiagnosis(issue, key)}
                    >
                      {diagnosis.status === 'loading' && activeKey === key
                        ? <Loader2 size={12} className="animate-spin" />
                        : <Sparkles size={12} />}
                      AI 诊断
                    </Button>
                  )}
                />
              )
            })}
          </ul>
        </div>
      )}
      {warnings.length > 0 && (
        <p className="mt-2 text-[11px] text-muted-foreground">另有 {warnings.length} 条警告，见预览顶部的警告摘要。</p>
      )}
      {issues.length === 0 && failureKind === 'latex' && (
        <p className="mt-3 rounded-lg border border-border/40 bg-background/60 p-2 text-xs text-muted-foreground">
          未能从日志中解析出具体的文件与行号。可在下方原始日志中搜索以「!」开头的行定位问题。
        </p>
      )}
      {isGuest && errors.length > 0 && (
        <p className="mt-2 text-[11px] text-muted-foreground">登录后可使用 AI 诊断与修复建议。</p>
      )}

      {diagnosis.status !== 'idle' && (
        <div className="mt-3 rounded-lg border border-border/50 bg-background/80 p-3">
          {diagnosis.status === 'loading' && (
            <p className="flex items-center gap-2 text-xs text-muted-foreground">
              <Loader2 size={13} className="animate-spin" />
              正在让 AI 结合报错日志与源码分析原因…
            </p>
          )}
          {diagnosis.status === 'error' && (
            <div role="alert" className="space-y-2">
              <p className="text-xs text-destructive">{diagnosis.message}</p>
              <p className="text-[11px] text-muted-foreground">
                AI 诊断不会自动改写稿件；如果反复失败，可先根据原始日志手动定位。
              </p>
              {/尚未配置 AI 模型|未配置/.test(diagnosis.message) && (
                <a href="/settings" className="inline-block text-[11px] text-primary underline underline-offset-2">
                  去设置中配置模型
                </a>
              )}
            </div>
          )}
          {diagnosis.status === 'ready' && diagnosis.result && (
            <div className="space-y-2">
              <p className="text-[11px] font-bold uppercase tracking-widest text-muted-foreground">AI 诊断</p>
              <p className="whitespace-pre-wrap text-xs leading-5">{diagnosis.result.rootCause}</p>
              {diagnosis.result.patch ? (
                <>
                  <p className="text-[11px] text-muted-foreground">修复文件：<span className="font-mono">{diagnosis.result.patch.filePath}</span></p>
                  <pre className="max-h-40 overflow-x-auto whitespace-pre-wrap rounded-lg bg-destructive/5 p-2 text-[11px] text-destructive/90">
                    {diagnosis.result.patch.oldText}
                  </pre>
                  <pre className="max-h-40 overflow-x-auto whitespace-pre-wrap rounded-lg bg-green-500/5 p-2 text-[11px] text-green-700">
                    {diagnosis.result.patch.newText}
                  </pre>
                  {diagnosis.result.patch.note && (
                    <p className="text-[11px] text-muted-foreground">修改说明：{diagnosis.result.patch.note}</p>
                  )}
                  {diagnosis.decision === 'applied' ? (
                    <p role="status" className="text-xs text-green-700">修复已应用，请重新编译验证效果。</p>
                  ) : diagnosis.decision === 'rejected' ? (
                    <p role="status" className="text-xs text-muted-foreground">已忽略本次修复建议，稿件未被修改。</p>
                  ) : (
                    <div className="flex flex-wrap gap-2 pt-1">
                      <Button type="button" size="sm" className={cn('gap-1', isMobile ? 'h-11' : 'h-8')} disabled={deciding} onClick={() => void decide(true)}>
                        {deciding && <Loader2 size={12} className="animate-spin" />}
                        应用修复
                      </Button>
                      <Button type="button" variant="outline" size="sm" className={cn(isMobile ? 'h-11' : 'h-8')} disabled={deciding} onClick={() => void decide(false)}>
                        忽略
                      </Button>
                    </div>
                  )}
                  <p className="text-[10px] text-muted-foreground">应用前请确认修改内容；应用后会写入稿件，不会自动重新编译。</p>
                </>
              ) : (
                <p className="text-xs text-muted-foreground">
                  {diagnosis.result.notice ?? '模型未给出可应用的修改建议，请参考根因说明手动修改。'}
                </p>
              )}
            </div>
          )}
        </div>
      )}

      {diagnosticStatus === 'pending' && (
        <p className="mt-2 text-[10px] text-muted-foreground">正在自动上报诊断信息…</p>
      )}
      {diagnosticStatus === 'sent' && (
        <p className="mt-2 text-[10px] text-muted-foreground">
          已自动上报诊断信息；如需反馈，请提供编号 {diagnosticId ? diagnosticId.slice(0, 8) : '—'}。
        </p>
      )}
      {diagnosticStatus === 'failed' && (
        <p className="mt-2 text-[10px] text-muted-foreground">
          诊断信息上报失败，不影响本次编译；可复制日志反馈给管理员。
        </p>
      )}

      <details className="mt-3">
        <summary className={cn('w-fit cursor-pointer select-none text-[10px] text-muted-foreground transition-colors hover:text-foreground', isMobile && 'inline-flex min-h-11 items-center')}>
          查看原始日志
        </summary>
        <pre className="mt-2 max-h-80 overflow-y-auto whitespace-pre-wrap break-words rounded-lg bg-muted/30 p-3 font-mono text-[10px] leading-relaxed text-muted-foreground/70">
          {rawLog}
        </pre>
      </details>
      {failureKind === 'resource' ? (
        <Button
          variant="outline"
          size="sm"
          data-testid="reinit-latex-resources"
          className={cn('mt-3 gap-2', isMobile ? 'h-11' : 'h-8')}
          onClick={onReinitResources}
          disabled={isCompiling}
        >
          <HardDrive size={13} />
          重新初始化资源
        </Button>
      ) : (
        <Button
          variant="outline"
          size="sm"
          data-testid="retry-browser-compile"
          className={cn('mt-3 gap-2', isMobile ? 'h-11' : 'h-8')}
          onClick={onRetryCompile}
          disabled={isCompiling}
        >
          {isCompiling && <Loader2 size={13} className="animate-spin" />}
          {isCompiling ? '正在重试编译' : '重试编译'}
        </Button>
      )}
    </div>
  )
}
