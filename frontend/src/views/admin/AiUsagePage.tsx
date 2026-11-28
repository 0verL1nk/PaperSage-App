/**
 * 管理后台 — AI 调用与健康度（仅 superuser）。
 * 只读 ai_model_calls 的聚合结果：KPI + 按日趋势 + 按模型/功能/错误码明细。
 * 红线：不展示个人明细，也不提供按用户下钻（个人消耗仅本人可见）。
 */
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Download, RefreshCw, ShieldAlert, TriangleAlert } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import {
  downloadAdminAiUsage,
  getAdminAiUsage,
  type AiUsageBucket,
  type AiUsageQuery,
} from '@/services/ai-usage-api'
import { friendlyErrorMessage, FrontendApiError } from '@/services/errors'

const FEATURE_LABELS: Record<string, string> = {
  chat: 'AI 助手对话',
  cross_chapter_review: '跨章节检查',
}

const ERROR_LABELS: Record<string, string> = {
  auth: '鉴权失败',
  rate_limit: '服务商限流',
  bad_request: '请求无效',
  provider_5xx: '服务商 5xx',
  network: '网络错误',
  stream_interrupted: '流式中断',
  unknown: '未知错误',
}

function featureLabel(key?: string): string {
  if (!key) return '—'
  return FEATURE_LABELS[key] ?? key
}

function errorLabel(key?: string): string {
  if (!key) return '—'
  return ERROR_LABELS[key] ?? key
}

function fmtInt(value: number): string {
  return value.toLocaleString('zh-CN')
}

function fmtPercent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function fmtMs(value: number | null): string {
  return value === null ? '—' : `${fmtInt(value)} ms`
}

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  anchor.click()
  URL.revokeObjectURL(url)
}

function Kpi({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <Card>
      <CardContent className="p-4">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="mt-1 text-2xl font-semibold tabular-nums">{value}</p>
        {hint ? <p className="mt-1 text-[11px] text-muted-foreground">{hint}</p> : null}
      </CardContent>
    </Card>
  )
}

function TrendBar({ value, max, className }: { value: number; max: number; className: string }) {
  const width = max > 0 ? Math.max(value > 0 ? 2 : 0, Math.round((value / max) * 100)) : 0
  return (
    <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-muted">
      <div className={className} style={{ width: `${width}%` }} />
    </div>
  )
}

function DayTrend({ days }: { days: AiUsageBucket[] }) {
  const maxCalls = Math.max(1, ...days.map((day) => day.calls))
  const maxLatency = Math.max(1, ...days.map((day) => day.latency_p95_ms ?? 0))
  const maxTokens = Math.max(1, ...days.map((day) => day.prompt_tokens + day.completion_tokens))
  return (
    <div className="grid gap-3 lg:grid-cols-3">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">按日调用与失败</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {days.map((day) => (
            <div key={day.key} className="flex items-center gap-2">
              <span className="w-14 shrink-0 text-[11px] tabular-nums text-muted-foreground">{day.key?.slice(5) ?? '—'}</span>
              <div className="flex h-2.5 flex-1 overflow-hidden rounded-full bg-muted">
                <div className="h-full bg-destructive" style={{ width: `${day.calls > 0 ? ((day.error + day.timeout + day.cancelled) / maxCalls) * 100 : 0}%` }} />
                <div className="h-full bg-primary" style={{ width: `${day.calls > 0 ? (day.success / maxCalls) * 100 : 0}%` }} />
              </div>
              <span className="w-20 shrink-0 text-right text-[11px] tabular-nums">
                {fmtInt(day.calls)}<span className="text-destructive"> / {fmtInt(day.error + day.timeout + day.cancelled)}</span>
              </span>
            </div>
          ))}
          <p className="pt-1 text-[11px] text-muted-foreground">总计 / 其中失败（错误 + 超时 + 取消）</p>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">按日延迟（成功调用）</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {days.map((day) => (
            <div key={day.key} className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="w-14 shrink-0 text-[11px] tabular-nums text-muted-foreground">{day.key?.slice(5) ?? '—'}</span>
                <TrendBar value={day.latency_p50_ms ?? 0} max={maxLatency} className="h-full rounded-full bg-primary/60" />
                <span className="w-20 shrink-0 text-right text-[11px] tabular-nums">P50 {fmtMs(day.latency_p50_ms)}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-14 shrink-0" />
                <TrendBar value={day.latency_p95_ms ?? 0} max={maxLatency} className="h-full rounded-full bg-primary" />
                <span className="w-20 shrink-0 text-right text-[11px] tabular-nums">P95 {fmtMs(day.latency_p95_ms)}</span>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">按日 Token 消耗</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          {days.map((day) => (
            <div key={day.key} className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="w-14 shrink-0 text-[11px] tabular-nums text-muted-foreground">{day.key?.slice(5) ?? '—'}</span>
                <TrendBar value={day.prompt_tokens} max={maxTokens} className="h-full rounded-full bg-primary/60" />
                <span className="w-20 shrink-0 text-right text-[11px] tabular-nums">入 {fmtInt(day.prompt_tokens)}</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="w-14 shrink-0" />
                <TrendBar value={day.completion_tokens} max={maxTokens} className="h-full rounded-full bg-primary" />
                <span className="w-20 shrink-0 text-right text-[11px] tabular-nums">出 {fmtInt(day.completion_tokens)}</span>
              </div>
            </div>
          ))}
        </CardContent>
      </Card>
    </div>
  )
}

function BucketTable({ title, keyLabel, rows, label }: {
  title: string
  keyLabel: string
  rows: AiUsageBucket[]
  label: (key?: string) => string
}) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm">{title}</CardTitle>
      </CardHeader>
      <CardContent className="overflow-x-auto p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{keyLabel}</TableHead>
              <TableHead className="text-right">调用</TableHead>
              <TableHead className="text-right">成功率</TableHead>
              <TableHead className="text-right">P50</TableHead>
              <TableHead className="text-right">P95</TableHead>
              <TableHead className="text-right">Token（输入/输出）</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((row) => (
              <TableRow key={row.key ?? '—'}>
                <TableCell className="max-w-[180px] truncate text-xs" title={row.key}>{label(row.key)}</TableCell>
                <TableCell className="text-right text-xs tabular-nums">{fmtInt(row.calls)}</TableCell>
                <TableCell className="text-right text-xs tabular-nums">{fmtPercent(row.success_rate)}</TableCell>
                <TableCell className="text-right text-xs tabular-nums">{fmtMs(row.latency_p50_ms)}</TableCell>
                <TableCell className="text-right text-xs tabular-nums">{fmtMs(row.latency_p95_ms)}</TableCell>
                <TableCell className="text-right text-xs tabular-nums">
                  {fmtInt(row.prompt_tokens)} / {fmtInt(row.completion_tokens)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

export function AiUsagePage() {
  const [draft, setDraft] = useState<AiUsageQuery>({})
  const [query, setQuery] = useState<AiUsageQuery>({})
  const [exporting, setExporting] = useState<'csv' | 'json' | null>(null)

  const reportQuery = useQuery({
    queryKey: ['admin-ai-usage', query],
    queryFn: () => getAdminAiUsage(query),
    retry: false,
  })

  const forbidden = reportQuery.error instanceof FrontendApiError && reportQuery.error.status === 403
  const report = reportQuery.data
  const totals = report?.totals
  const hasCalls = (totals?.calls ?? 0) > 0

  const runExport = async (format: 'csv' | 'json') => {
    setExporting(format)
    try {
      const blob = await downloadAdminAiUsage(format, query)
      saveBlob(blob, `ai-usage.${format}`)
      toast.success(`正在下载 ${format.toUpperCase()} 报表`)
    } catch (error) {
      toast.error(friendlyErrorMessage(error))
    } finally {
      setExporting(null)
    }
  }

  return (
    <div className="space-y-4">
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(event) => {
          event.preventDefault()
          setQuery(draft)
        }}
      >
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          开始日期
          <Input
            type="date"
            className="h-9 w-[150px]"
            value={draft.from ?? ''}
            onChange={(event) => setDraft((current) => ({ ...current, from: event.target.value || undefined }))}
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          结束日期
          <Input
            type="date"
            className="h-9 w-[150px]"
            value={draft.to ?? ''}
            onChange={(event) => setDraft((current) => ({ ...current, to: event.target.value || undefined }))}
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          功能入口
          <Select
            value={draft.feature_kind ?? 'all'}
            onValueChange={(value) => setDraft((current) => ({ ...current, feature_kind: value === 'all' ? undefined : value }))}
          >
            <SelectTrigger className="h-9 w-[160px]">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部</SelectItem>
              <SelectItem value="chat">AI 助手对话</SelectItem>
              <SelectItem value="cross_chapter_review">跨章节检查</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          模型（精确匹配）
          <Input
            className="h-9 w-[170px]"
            placeholder="如 deepseek-chat"
            value={draft.model ?? ''}
            onChange={(event) => setDraft((current) => ({ ...current, model: event.target.value.trim() || undefined }))}
          />
        </label>
        <Button type="submit" size="sm" className="h-9">应用筛选</Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-9"
          onClick={() => {
            setDraft({})
            setQuery({})
          }}
        >
          重置
        </Button>
        <div className="ml-auto flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-9"
            disabled={exporting !== null}
            onClick={() => void runExport('csv')}
          >
            <Download className="mr-1 size-3.5" />{exporting === 'csv' ? '导出中…' : '导出 CSV'}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-9"
            disabled={exporting !== null}
            onClick={() => void runExport('json')}
          >
            <Download className="mr-1 size-3.5" />{exporting === 'json' ? '导出中…' : '导出 JSON'}
          </Button>
        </div>
      </form>

      {reportQuery.isLoading ? (
        <Card>
          <CardContent className="space-y-3 p-4">
            <span className="sr-only">正在加载 AI 调用统计</span>
            {[0, 1, 2].map((row) => <Skeleton key={row} className="h-12 w-full" />)}
          </CardContent>
        </Card>
      ) : forbidden ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
            <ShieldAlert className="size-8 text-muted-foreground" />
            <p className="text-sm font-medium">需要管理员权限</p>
            <p className="text-xs text-muted-foreground">当前账号没有查看全站 AI 调用统计的权限，请联系管理员。</p>
          </CardContent>
        </Card>
      ) : reportQuery.isError ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
            <TriangleAlert className="size-8 text-destructive" />
            <p className="text-sm font-medium">无法加载 AI 调用统计</p>
            <p className="text-xs text-muted-foreground">{friendlyErrorMessage(reportQuery.error)}</p>
            <Button size="sm" variant="outline" onClick={() => void reportQuery.refetch()}>
              <RefreshCw className="mr-1 size-3.5" />重试
            </Button>
          </CardContent>
        </Card>
      ) : !hasCalls ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-2 p-10 text-center">
            <p className="text-sm font-medium">还没有模型调用记录</p>
            <p className="max-w-md text-xs text-muted-foreground">
              配置模型并完成一次 AI 对话后，调用次数、耗时与 Token 消耗会自动出现在这里。
            </p>
          </CardContent>
        </Card>
      ) : (
        <>
          <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <Kpi label="调用次数" value={fmtInt(totals?.calls ?? 0)} hint={`成功 ${fmtInt(totals?.success ?? 0)} 次`} />
            <Kpi label="成功率" value={fmtPercent(totals?.success_rate ?? 0)} hint={`失败 ${fmtInt((totals?.error ?? 0) + (totals?.timeout ?? 0) + (totals?.cancelled ?? 0))} 次`} />
            <Kpi label="P50 / P95 延迟" value={fmtMs(totals?.latency_p50_ms ?? null)} hint={`P95 ${fmtMs(totals?.latency_p95_ms ?? null)}`} />
            <Kpi
              label="Token 消耗（输入/输出）"
              value={`${fmtInt(totals?.prompt_tokens ?? 0)} / ${fmtInt(totals?.completion_tokens ?? 0)}`}
              hint={(totals?.usage_missing_calls ?? 0) > 0 ? `${fmtInt(totals?.usage_missing_calls ?? 0)} 次调用未返回用量` : '全部调用均返回用量'}
            />
          </div>

          <DayTrend days={report?.by_day ?? []} />

          <div className="grid gap-3 lg:grid-cols-2">
            <BucketTable title="按模型" keyLabel="模型" rows={report?.by_model ?? []} label={(key) => key ?? '—'} />
            <BucketTable title="按功能入口" keyLabel="功能" rows={report?.by_feature ?? []} label={featureLabel} />
          </div>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm">错误码分布</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-wrap gap-2">
              {(report?.by_error ?? []).map((row) => (
                <Badge key={row.key ?? 'unknown'} variant="secondary" className="gap-1.5">
                  {errorLabel(row.key)}
                  <span className="tabular-nums">{fmtInt(row.calls)}</span>
                </Badge>
              ))}
            </CardContent>
          </Card>

          <p className="text-[11px] text-muted-foreground">
            统计口径：按{report?.timezone === 'Asia/Shanghai' ? '北京时间' : report?.timezone ?? '服务器时区'}自然日；P50/P95 只统计成功调用；
            首字延迟只统计流式且服务商返回首字时间的调用；未返回用量显示「—」。管理端不提供按用户下钻。
          </p>
        </>
      )}
    </div>
  )
}
