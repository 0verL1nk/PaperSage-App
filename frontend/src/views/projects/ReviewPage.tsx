import { useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import {
  AlertTriangle, ArrowRight, CheckCircle2, CircleAlert, FileCheck2, History, LoaderCircle,
  Play, RefreshCw, ShieldAlert, Sparkles, Square,
} from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useLatexFileContent, useLatexFiles } from '@/hooks/use-latex'
import { friendlyErrorMessage } from '@/services/errors'
import { useResearchArtifacts } from '@/services/research-api'
import { useChatStore } from '@/store/chat-store'
import { useSessionStore } from '@/store/session-store'
import { useSettingsStore } from '@/store/settings-store'
import type { ChapterReviewPayload, ResearchArtifact } from '@/types/research'
import { evaluateReviewChecks } from './review-checks'
import { CROSS_CHAPTER_REVIEW_PROMPT } from './review-prompt'
import {
  REVIEW_BASIS_LABEL,
  REVIEW_DIMENSION_HINT,
  REVIEW_DIMENSION_LABEL,
  REVIEW_DIMENSION_ORDER,
  REVIEW_SEVERITY_LABEL,
  REVIEW_SOURCE_LABEL,
  REVIEW_STATUS_LABEL,
  compareReviewReports,
  dimensionOf,
  displayBasisRef,
  findingsByDimension,
  formatReviewTime,
  humanizeReviewText,
  planSectionTitles,
  reviewCoverage,
  reviewLocation,
  reviewReportProblems,
  sortReportsByUpdatedAt,
} from './cross-chapter-review'
import { AiPanel } from './ai-panel'

function reviewPayload(artifact: ResearchArtifact | undefined): ChapterReviewPayload | null {
  const payload = artifact?.current?.payload as unknown
  if (!payload || typeof payload !== 'object') return null
  const candidate = payload as Partial<ChapterReviewPayload>
  return Array.isArray(candidate.dimensions) ? (candidate as ChapterReviewPayload) : null
}

function findingCount(artifact: ResearchArtifact): number {
  return reviewPayload(artifact)?.findings?.length ?? 0
}

const STATUS_TONE: Record<string, string> = {
  checked: 'text-emerald-700 dark:text-emerald-400',
  findings: 'text-amber-700 dark:text-amber-400',
  insufficient: 'text-amber-700 dark:text-amber-400',
  not_checked: 'text-muted-foreground',
  failed: 'text-destructive',
}

export function ReviewPage() {
  const { projectId = '' } = useParams({ strict: false })
  const { data: files = [], isPending: filesLoading, error: filesError, refetch } = useLatexFiles(projectId)
  const mainFile = files.find((file) => file.file_path === 'main.tex')
  const { data: mainContent, isPending: contentLoading, error: contentError } = useLatexFileContent(projectId, mainFile?.file_path ?? null)
  const checks = evaluateReviewChecks(files, mainContent?.content)
  const pending = filesLoading || (!!mainFile && contentLoading)
  const error = filesError || contentError
  const actionCount = checks.filter((check) => check.status === 'action').length
  const warningCount = checks.filter((check) => check.status === 'warning').length
  const passCount = checks.filter((check) => check.status === 'pass').length

  const isGuest = useSessionStore((state) => state.isGuest)
  const chat = useChatStore()
  const agentSettings = useSettingsStore((state) => state.settings?.agent)
  const fetchSettings = useSettingsStore((state) => state.fetchSettings)
  useEffect(() => {
    if (!agentSettings && !isGuest) void fetchSettings().catch(() => {})
  }, [agentSettings, fetchSettings, isGuest])
  const modelReady = Boolean(agentSettings?.providers?.length || agentSettings?.server_fallback?.configured)
  const settingsPending = !isGuest && !agentSettings

  const reportsQuery = useResearchArtifacts(isGuest ? undefined : projectId, 'chapter_review')
  const refetchReports = reportsQuery.refetch
  const parsedReports = useMemo(
    () => sortReportsByUpdatedAt(reportsQuery.data ?? [])
      .map((report) => {
        const payload = reviewPayload(report)
        // 修复前的历史报告可能是「维度标 findings 却没有问题」；隔离而不是改写用户数据。
        const problems = payload ? reviewReportProblems(payload) : ['报告内容无法读取']
        return { report, problems }
      }),
    [reportsQuery.data],
  )
  const reports = useMemo(
    () => parsedReports.filter((item) => item.problems.length === 0).map((item) => item.report),
    [parsedReports],
  )
  const isolatedReports = useMemo(
    () => parsedReports.filter((item) => item.problems.length > 0),
    [parsedReports],
  )
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selectedIndex = Math.max(0, reports.findIndex((report) => report.id === selectedId))
  const selected = reports[selectedIndex]
  const previous = reports[selectedIndex + 1]
  const payload = reviewPayload(selected)
  const previousPayload = reviewPayload(previous)
  const coverage = payload ? reviewCoverage(payload) : null
  const comparison = payload && previousPayload ? compareReviewReports(previousPayload, payload) : null
  // 小规模映射，直接计算；手动 memo 会被 React Compiler 判定为无法保留。
  const sectionTitles = payload ? planSectionTitles(payload) : new Map<string, string>()

  const wasStreaming = useRef(false)
  useEffect(() => {
    if (wasStreaming.current && !chat.streaming) void refetchReports()
    wasStreaming.current = chat.streaming
  }, [chat.streaming, refetchReports])

  const runReview = () => {
    if (isGuest || !modelReady || chat.streaming || chat.cancelling || chat.restoring || chat.restoreFailed) return
    // 新一轮检查完成后默认展示最新报告，不把用户留在旧的历史选择上。
    setSelectedId(null)
    // 失败原因由 chat store 写入 chat.error，下方与 AI 面板统一呈现。
    void chat.send(projectId, CROSS_CHAPTER_REVIEW_PROMPT).catch(() => {})
  }

  const runDisabled = isGuest || !modelReady || settingsPending || chat.streaming || chat.cancelling || chat.restoring || chat.restoreFailed

  return (
    <div className="h-full overflow-y-auto overscroll-contain">
      <div className="mx-auto max-w-5xl space-y-6 p-4 sm:p-7 lg:p-9">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0">
            <Badge variant="outline" className="mb-3 gap-1.5 rounded-full border-primary/20 bg-primary/5 px-3 py-1 text-primary">
              <FileCheck2 className="size-3.5" /> 项目检查
            </Badge>
            <h1 className="text-balance text-2xl font-semibold tracking-tight sm:text-3xl">提交前，把容易漏的地方过一遍</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              先用本地规则检查文件结构，再由 AI 结合项目背景、章节计划、各章正文与资料做跨章节论证审阅。
            </p>
          </div>
          <Button asChild className="min-h-[44px] w-full shrink-0 sm:w-auto">
            <Link to="/project/$projectId/writing" params={{ projectId }}>
              打开论文撰写 <ArrowRight className="ml-2 size-4" />
            </Link>
          </Button>
        </header>

        <Card className="rounded-2xl border-border/50 shadow-sm">
          <CardHeader className="pb-3">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="text-base">基础结构检查</CardTitle>
              <Badge variant="secondary" className="font-normal">本地确定性检查</Badge>
            </div>
            <CardDescription>
              {pending ? '正在读取项目文件并检查…' : error ? '文件读取未完成，请重试后再查看结果。' : `已检查 ${checks.length} 项，发现 ${actionCount} 项待修复、${warningCount} 项建议确认。以下结果由本地规则计算，不含 AI 语义判断。`}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {error ? (
              <div role="alert" className="flex flex-col gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-2 text-sm text-destructive">
                  <CircleAlert className="mt-0.5 size-4 shrink-0" />
                  <div className="min-w-0">
                    <span className="break-words">{friendlyErrorMessage(error)}</span>
                    <details className="mt-1 text-xs text-muted-foreground">
                      <summary className="flex min-h-[44px] cursor-pointer items-center">技术细节</summary>
                      <p className="break-all">{error.message}</p>
                    </details>
                  </div>
                </div>
                <Button type="button" variant="outline" className="min-h-[44px] shrink-0" onClick={() => void refetch()}>
                  <RefreshCw className="mr-2 size-4" />重试读取
                </Button>
              </div>
            ) : pending ? (
              <div className="flex min-h-32 items-center justify-center gap-2 text-sm text-muted-foreground">
                <LoaderCircle className="size-4 animate-spin" />正在检查文件…
              </div>
            ) : (
              <div className="divide-y divide-border/40">
              {checks.map((check) => {
                const Icon = check.status === 'pass' ? CheckCircle2 : check.status === 'warning' ? AlertTriangle : CircleAlert
                const tone = check.status === 'pass'
                  ? 'text-emerald-700 dark:text-emerald-400'
                  : check.status === 'warning'
                    ? 'text-amber-700 dark:text-amber-400'
                    : 'text-destructive'
                const label = check.status === 'pass' ? '已通过' : check.status === 'warning' ? '建议确认' : '需要处理'
                return (
                  <div key={check.id} className="flex min-w-0 flex-col gap-3 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-start">
                    <Icon className={`mt-0.5 size-4 shrink-0 ${tone}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h2 className="text-sm font-semibold">{check.title}</h2>
                        <span className={`text-[11px] font-medium ${tone}`}>{label}</span>
                      </div>
                      <p className="mt-1 break-words text-xs leading-5 text-muted-foreground">{check.detail}</p>
                    </div>
                    {check.status !== 'pass' && (
                      <Button asChild variant="ghost" className="min-h-[44px] shrink-0 justify-start px-2 sm:justify-center">
                        <Link to="/project/$projectId/writing" params={{ projectId }}>去修改 <ArrowRight className="ml-1 size-3.5" /></Link>
                      </Button>
                    )}
                  </div>
                )
              })}
              </div>
            )}
          </CardContent>
        </Card>

        {!pending && !error && (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl bg-muted/35 px-4 py-3 text-xs text-muted-foreground" aria-live="polite">
            <span className="font-medium text-foreground">{passCount} 项结构检查通过</span>
            <span>{warningCount} 项建议确认</span>
            <span>{actionCount} 项需要处理</span>
          </div>
        )}

        <Card className="rounded-2xl border-border/50 shadow-sm">
          <CardHeader className="pb-3">
            <div className="flex flex-wrap items-center gap-2">
              <CardTitle className="flex items-center gap-2 text-base"><Sparkles className="size-4 text-primary" />跨章节检查</CardTitle>
              <Badge variant="outline" className="font-normal">AI 模型审阅</Badge>
            </div>
            <CardDescription>
              由 AI 结合项目背景、已采用章节计划、各章正文与已读资料，检查目标与结论、证据支撑、章节间重复/矛盾、学术规范符合度。语义结论可能出错，请逐条核对依据。
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-4">
            {isGuest ? (
              <div className="rounded-xl border border-border/50 bg-muted/30 p-4 text-sm text-muted-foreground">
                登录后可使用跨章节检查；报告会保存在项目里，刷新后仍可查看。
              </div>
            ) : (
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <Button type="button" className="min-h-[44px]" onClick={runReview} disabled={runDisabled}>
                    {chat.streaming ? <LoaderCircle className="mr-2 size-4 animate-spin" /> : <Play className="mr-2 size-4" />}
                    {chat.streaming ? '正在检查…' : '运行跨章节检查'}
                  </Button>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {settingsPending ? '正在确认模型配置…' : modelReady ? '检查需要几分钟；过程中可在下方 AI 助手查看读取与推理进度。' : '尚未配置 AI 模型，跨章节检查不可用。'}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {!settingsPending && !modelReady ? (
                    <Button asChild variant="outline" className="min-h-[44px]">
                      <Link to="/app/settings">去设置模型</Link>
                    </Button>
                  ) : null}
                  {chat.streaming ? (
                    <Button type="button" variant="outline" className="min-h-[44px]" onClick={() => void chat.cancel()} disabled={chat.cancelling}>
                      <Square className="mr-2 size-4" />{chat.cancelling ? '正在取消…' : '取消检查'}
                    </Button>
                  ) : null}
                </div>
              </div>
            )}

            {chat.streaming ? (
              <div role="status" className="flex items-start gap-2 rounded-xl border border-primary/20 bg-primary/5 p-4 text-sm">
                <LoaderCircle className="mt-0.5 size-4 shrink-0 animate-spin text-primary" />
                <div>
                  <p className="font-medium">正在读取材料并生成检查报告</p>
                  <p className="mt-1 text-xs text-muted-foreground">进度与工具调用见下方 AI 助手。完成后报告会自动刷新到上方。</p>
                </div>
              </div>
            ) : null}

            {chat.error ? (
              <div role="alert" className="flex flex-col gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-2">
                  <CircleAlert className="mt-0.5 size-4 shrink-0" />
                  <span className="break-words">检查未完成：{friendlyErrorMessage(chat.error)}</span>
                </div>
                <Button type="button" variant="outline" className="min-h-[44px] shrink-0" onClick={runReview} disabled={runDisabled}>
                  <RefreshCw className="mr-2 size-4" />重新运行检查
                </Button>
              </div>
            ) : null}

            {reportsQuery.isPending && !isGuest ? (
              <div className="flex min-h-24 items-center justify-center gap-2 text-sm text-muted-foreground">
                <LoaderCircle className="size-4 animate-spin" />正在读取历史检查报告…
              </div>
            ) : reportsQuery.error && !isGuest ? (
              <div role="alert" className="flex flex-col gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-2">
                  <CircleAlert className="mt-0.5 size-4 shrink-0" />
                  <div className="min-w-0">
                    <span className="break-words">历史报告加载失败：{friendlyErrorMessage(reportsQuery.error)}</span>
                    <details className="mt-1 text-xs text-muted-foreground">
                      <summary className="flex min-h-[44px] cursor-pointer items-center">技术细节</summary>
                      <p className="break-all">{reportsQuery.error.message}</p>
                    </details>
                  </div>
                </div>
                <Button type="button" variant="outline" className="min-h-[44px] shrink-0" onClick={() => void refetchReports()}>
                  <RefreshCw className="mr-2 size-4" />重试读取
                </Button>
              </div>
            ) : !isGuest && reports.length === 0 && !chat.streaming ? (
              <div className="rounded-xl border border-dashed border-border/60 bg-muted/20 p-5 text-sm text-muted-foreground">
                <p className="font-medium text-foreground">{isolatedReports.length > 0 ? '还没有有效的跨章节检查报告' : '还没有跨章节检查报告'}</p>
                <p className="mt-1">{modelReady ? '先准备好已采用的章节计划与各章正文，再点上方「运行跨章节检查」。报告会保存下来，方便对比和回看。' : '先点上方「去设置模型」完成配置，再回来运行跨章节检查。'}</p>
              </div>
            ) : null}

            {!isGuest && isolatedReports.length > 0 ? (
              <div role="status" className="rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm">
                <p className="flex items-center gap-2 font-medium text-amber-800 dark:text-amber-300">
                  <ShieldAlert className="size-4" />已隔离 {isolatedReports.length} 份修复前的无效报告
                </p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  这些记录的维度状态与问题列表不一致，不会作为审查结果展示，也不能在历史中选择。修复后的新报告不受影响。
                </p>
                <details className="mt-2 text-xs text-muted-foreground">
                  <summary className="flex min-h-[44px] cursor-pointer items-center">查看被隔离记录</summary>
                  <ul className="mt-1 list-disc space-y-1 pl-5">
                    {isolatedReports.map(({ report, problems }) => (
                      <li key={report.id}>{formatReviewTime(report.updatedAt)}：{problems[0]}</li>
                    ))}
                  </ul>
                </details>
              </div>
            ) : null}

            {!isGuest && selected ? (
              <div className="space-y-4 border-t border-border/40 pt-4">
                {reports.length > 1 ? (
                  <div className="flex flex-wrap items-center gap-2 text-xs">
                    <History className="size-4 text-muted-foreground" />
                    <label htmlFor="review-history" className="font-medium">查看历史报告</label>
                    <select
                      id="review-history"
                      className="min-h-[44px] rounded-lg border border-border/60 bg-background px-2 text-xs"
                      value={selected.id}
                      onChange={(event) => setSelectedId(event.target.value)}
                    >
                      {reports.map((report, index) => (
                        <option key={report.id} value={report.id}>
                          {formatReviewTime(report.updatedAt)}{index === 0 ? '（最新）' : ''} · {findingCount(report)} 条问题
                        </option>
                      ))}
                    </select>
                  </div>
                ) : null}

                {!payload ? (
                  <div role="alert" className="rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive">
                    这份报告内容无法读取，请重新运行一次检查。
                  </div>
                ) : (
                  <>
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                      <span className="font-medium text-foreground">{formatReviewTime(selected.updatedAt)} 的检查</span>
                      <span>{payload.findings.length} 条问题</span>
                      <span>
                        章节计划：{payload.input?.planStatus === 'adopted' ? '已采用' : payload.input?.planStatus === 'candidate' ? '候选（尚未采用）' : '缺失'}
                      </span>
                      <span>
                        学术规范：{payload.input?.normsAvailable ? `已采用 ${payload.input.norms?.length ?? 0} 条` : humanizeReviewText(payload.input?.normsNote || '尚未接入', sectionTitles)}
                      </span>
                    </div>

                    <div className="grid gap-2 sm:grid-cols-2">
                      {REVIEW_DIMENSION_ORDER.map((key) => {
                        const dimension = dimensionOf(payload, key)
                        return (
                          <div key={key} className="rounded-xl border border-border/50 bg-card p-3">
                            <div className="flex items-center justify-between gap-2">
                              <span className="text-xs font-medium">{REVIEW_DIMENSION_LABEL[key]}</span>
                              <span className={`text-[11px] font-medium ${STATUS_TONE[dimension?.status ?? 'not_checked']}`}>
                                {dimension ? REVIEW_STATUS_LABEL[dimension.status] : '未返回'}
                              </span>
                            </div>
                            {dimension?.note ? <p className="mt-1 text-[11px] leading-5 text-muted-foreground">{humanizeReviewText(dimension.note, sectionTitles)}</p> : null}
                            <p className="mt-1 text-[11px] leading-5 text-muted-foreground">{REVIEW_DIMENSION_HINT[key]}</p>
                          </div>
                        )
                      })}
                    </div>

                    {coverage && !coverage.complete ? (
                      <div role="status" className="rounded-xl border border-amber-500/25 bg-amber-500/5 p-4 text-sm">
                        <p className="flex items-center gap-2 font-medium text-amber-800 dark:text-amber-300">
                          <ShieldAlert className="size-4" />本次覆盖不完整（{coverage.covered}/{REVIEW_DIMENSION_ORDER.length} 个维度完成）
                        </p>
                        <ul className="mt-2 list-disc space-y-1 pl-5 text-xs text-muted-foreground">
                          {coverage.reasons.map((reason) => <li key={reason}>{humanizeReviewText(reason, sectionTitles)}</li>)}
                        </ul>
                        <p className="mt-2 text-xs text-muted-foreground">未覆盖的维度不能当作「没有问题」；补齐材料后可重新运行。</p>
                      </div>
                    ) : coverage && coverage.complete && payload.findings.length === 0 ? (
                      <div role="status" className="rounded-xl border border-emerald-500/25 bg-emerald-500/5 p-4 text-sm text-emerald-800 dark:text-emerald-300">
                        <p className="flex items-center gap-2 font-medium"><CheckCircle2 className="size-4" />本次四个维度均已完成，未发现问题</p>
                        <p className="mt-1 text-xs text-muted-foreground">模型结论仍需结合导师意见与原始文献人工核验。</p>
                      </div>
                    ) : null}

                    {comparison && (comparison.added.length > 0 || comparison.resolved.length > 0 || comparison.persisting.length > 0) ? (
                      <div className="rounded-xl bg-muted/30 p-3 text-xs text-muted-foreground">
                        <p className="font-medium text-foreground">与上一次报告对比</p>
                        <p className="mt-1">新增 {comparison.added.length} 条，已消失 {comparison.resolved.length} 条，仍存在 {comparison.persisting.length} 条。</p>
                        {comparison.added.length > 0 ? (
                          <p className="mt-1">新增：{comparison.added.map((finding) => humanizeReviewText(finding.problem, sectionTitles)).join('；')}</p>
                        ) : null}
                        {comparison.resolved.length > 0 ? (
                          <p className="mt-1">已消失：{comparison.resolved.map((finding) => humanizeReviewText(finding.problem, sectionTitles)).join('；')}</p>
                        ) : null}
                      </div>
                    ) : null}

                    <div className="space-y-4">
                      {findingsByDimension(payload).map((group) => (
                        <section key={group.key} className="space-y-2">
                          <h2 className="text-sm font-semibold">{group.label}（{group.findings.length}）</h2>
                          {group.findings.map((finding) => (
                            <article key={finding.id} className="rounded-xl border border-border/50 bg-card p-4">
                              <div className="flex flex-wrap items-center gap-2">
                                <Badge variant={finding.severity === 'high' ? 'destructive' : 'secondary'} className="font-normal">
                                  {REVIEW_SEVERITY_LABEL[finding.severity] ?? finding.severity}
                                </Badge>
                                <span className="text-xs text-muted-foreground">{reviewLocation(finding, sectionTitles)}</span>
                                {finding.anchorVerified ? <Badge variant="outline" className="font-normal">引文已核对</Badge> : null}
                                {finding.confidence === 'low' ? <Badge variant="outline" className="font-normal">置信度低</Badge> : null}
                              </div>
                              {finding.quote ? (
                                <blockquote className="mt-2 whitespace-pre-wrap rounded-lg border-l-2 border-primary/40 bg-muted/30 px-3 py-2 text-xs leading-5 text-muted-foreground">
                                  {finding.quote}
                                </blockquote>
                              ) : null}
                              <p className="mt-2 break-words text-sm leading-6">{humanizeReviewText(finding.problem, sectionTitles)}</p>
                              <details className="mt-2">
                                <summary className="flex min-h-[44px] cursor-pointer items-center text-xs font-medium text-primary">查看依据（{finding.basis.length}）</summary>
                                <ul className="mt-1 space-y-1">
                                  {finding.basis.map((basis, index) => (
                                    <li key={`${finding.id}-basis-${index}`} className="rounded-lg bg-muted/30 p-2 text-xs">
                                      <span className="font-medium">{REVIEW_BASIS_LABEL[basis.kind] ?? basis.kind}</span>
                                      <span className="text-muted-foreground">
                                        {' · '}{displayBasisRef(basis, sectionTitles)}
                                        {basis.lineStart ? `（第 ${basis.lineStart}${basis.lineEnd && basis.lineEnd !== basis.lineStart ? `–${basis.lineEnd}` : ''} 行）` : ''}
                                      </span>
                                      {basis.quote ? <p className="mt-1 whitespace-pre-wrap leading-5 text-muted-foreground">{basis.quote}</p> : null}
                                    </li>
                                  ))}
                                </ul>
                              </details>
                              <p className="mt-2 break-words text-xs leading-5"><span className="font-medium">下一步：</span>{humanizeReviewText(finding.nextStep, sectionTitles)}</p>
                            </article>
                          ))}
                        </section>
                      ))}
                    </div>

                    {payload.sourcesRead?.length ? (
                      <details className="rounded-xl border border-border/50 bg-muted/20 p-3 text-xs">
                        <summary className="flex min-h-[44px] cursor-pointer items-center font-medium">本次读取的材料（{payload.sourcesRead.length}）</summary>
                        <ul className="mt-2 space-y-1 text-muted-foreground">
                          {payload.sourcesRead.map((source, index) => (
                            <li key={`${source.kind}-${source.name}-${index}`}>{REVIEW_SOURCE_LABEL[source.kind] ?? source.kind}：{source.name}</li>
                          ))}
                        </ul>
                      </details>
                    ) : null}
                  </>
                )}
              </div>
            ) : null}
          </CardContent>
        </Card>

        <Card className="overflow-hidden rounded-2xl border-border/50 shadow-sm">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">AI 助手</CardTitle>
            <CardDescription>查看跨章节检查的读取与推理过程，或继续追问某条问题；不会直接修改正文。</CardDescription>
          </CardHeader>
          <div className="h-[520px] border-t border-border/40">
            <AiPanel projectId={projectId} mode="review" />
          </div>
        </Card>

        <p className="flex items-start gap-2 px-1 text-xs leading-5 text-muted-foreground">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-700" />
          基础结构检查是本地确定性规则；跨章节检查由 AI 模型生成，可能出错。请逐条核对引文与依据，并结合导师意见与原始文献人工确认。
        </p>
      </div>
    </div>
  )
}
