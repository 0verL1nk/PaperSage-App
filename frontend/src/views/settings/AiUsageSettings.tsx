/**
 * 设置 → AI 用量：用户查看自身近 30 天的调用与 Token 消耗。
 * 服务端强制按当前用户过滤；没有数据时明确说明，不显示编造的费用。
 */
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RefreshCw, TriangleAlert } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { getMyAiUsage, type AiUsageBucket } from '@/services/ai-usage-api'
import { friendlyErrorMessage } from '@/services/errors'

function fmtInt(value: number): string {
  return value.toLocaleString('zh-CN')
}

function fmtPercent(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

function fmtMs(value: number | null): string {
  return value === null ? '—' : `${fmtInt(value)} ms`
}

function DayTokens({ days }: { days: AiUsageBucket[] }) {
  const max = Math.max(1, ...days.map((day) => day.prompt_tokens + day.completion_tokens))
  return (
    <div className="space-y-2">
      {days.map((day) => {
        const total = day.prompt_tokens + day.completion_tokens
        const width = total > 0 ? Math.max(2, Math.round((total / max) * 100)) : 0
        return (
          <div key={day.key} className="flex items-center gap-2">
            <span className="w-14 shrink-0 text-[11px] tabular-nums text-muted-foreground">{day.key?.slice(5) ?? '—'}</span>
            <div className="h-2.5 flex-1 overflow-hidden rounded-full bg-muted">
              <div className="h-full rounded-full bg-primary" style={{ width: `${width}%` }} />
            </div>
            <span className="w-20 shrink-0 text-right text-[11px] tabular-nums">{fmtInt(total)}</span>
          </div>
        )
      })}
    </div>
  )
}

export function AiUsageSettings() {
  const { t } = useTranslation()
  const usageQuery = useQuery({
    queryKey: ['my-ai-usage'],
    queryFn: () => getMyAiUsage(),
    retry: false,
  })

  if (usageQuery.isLoading) {
    return (
      <Card>
        <CardContent className="space-y-3 p-4">
          <span className="sr-only">{t('settings.aiUsageLoading')}</span>
          {[0, 1, 2].map((row) => <Skeleton key={row} className="h-10 w-full" />)}
        </CardContent>
      </Card>
    )
  }

  if (usageQuery.isError) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 p-8 text-center">
          <TriangleAlert className="size-8 text-destructive" />
          <p className="text-sm font-medium">{t('settings.aiUsageLoadFailed')}</p>
          <p className="text-xs text-muted-foreground">{friendlyErrorMessage(usageQuery.error)}</p>
          <Button size="sm" variant="outline" onClick={() => void usageQuery.refetch()}>
            <RefreshCw className="mr-1 size-3.5" />{t('settings.aiUsageRetry')}
          </Button>
        </CardContent>
      </Card>
    )
  }

  const report = usageQuery.data
  const totals = report?.totals
  const hasCalls = (totals?.calls ?? 0) > 0

  if (!hasCalls) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-2 p-10 text-center">
          <p className="text-sm font-medium">{t('settings.aiUsageEmpty')}</p>
          <p className="max-w-md text-xs text-muted-foreground">{t('settings.aiUsageEmptyDesc')}</p>
        </CardContent>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t('settings.aiUsageCalls')}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">{fmtInt(totals?.calls ?? 0)}</p>
            <p className="mt-1 text-[11px] text-muted-foreground">
              {t('settings.aiUsageSuccessRate')} {fmtPercent(totals?.success_rate ?? 0)}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t('settings.aiUsageTokens')}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">
              {fmtInt((totals?.prompt_tokens ?? 0) + (totals?.completion_tokens ?? 0))}
            </p>
            <p className="mt-1 text-[11px] text-muted-foreground">
              {t('settings.aiUsageTokensValue', {
                prompt: fmtInt(totals?.prompt_tokens ?? 0),
                completion: fmtInt(totals?.completion_tokens ?? 0),
              })}
            </p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t('settings.aiUsageLatency')}</p>
            <p className="mt-1 text-lg font-semibold tabular-nums">{fmtMs(totals?.latency_p50_ms ?? null)}</p>
            <p className="mt-1 text-[11px] text-muted-foreground">P95 {fmtMs(totals?.latency_p95_ms ?? null)}</p>
          </CardContent>
        </Card>
        <Card>
          <CardContent className="p-4">
            <p className="text-xs text-muted-foreground">{t('settings.aiUsageMissingLabel')}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">{fmtInt(totals?.usage_missing_calls ?? 0)}</p>
            <p className="mt-1 text-[11px] text-muted-foreground">{t('settings.aiUsageMissingHint')}</p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t('settings.aiUsageByDay')}</CardTitle>
        </CardHeader>
        <CardContent>
          <DayTokens days={report?.by_day ?? []} />
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-sm">{t('settings.aiUsageByModel')}</CardTitle>
        </CardHeader>
        <CardContent className="overflow-x-auto p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>{t('settings.aiUsageModel')}</TableHead>
                <TableHead className="text-right">{t('settings.aiUsageTableCalls')}</TableHead>
                <TableHead className="text-right">{t('settings.aiUsageTableTokens')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(report?.by_model ?? []).map((row) => (
                <TableRow key={row.key ?? '—'}>
                  <TableCell className="max-w-[200px] truncate text-xs" title={row.key}>{row.key ?? '—'}</TableCell>
                  <TableCell className="text-right text-xs tabular-nums">{fmtInt(row.calls)}</TableCell>
                  <TableCell className="text-right text-xs tabular-nums">
                    {fmtInt(row.prompt_tokens)} / {fmtInt(row.completion_tokens)}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <p className="text-[11px] text-muted-foreground">{t('settings.aiUsageNote')}</p>
    </div>
  )
}
