/**
 * 管理后台 — 编译错误日志（仅 superuser）。
 * 浏览器 WASM 编译失败时客户端自动上报脱敏诊断；本页支持筛选、分页与详情查看。
 * 只读 compile_error_reports，不读 compile_jobs.logs（用户编译历史）。
 */
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ClipboardCopy, Download, ShieldAlert } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { EmptyState, ErrorState, LoadingState } from '@/components/shared'
import {
  getCompileDiagnostic,
  listCompileDiagnostics,
  type CompileDiagnosticDetail,
  type CompileDiagnosticItem,
  type CompileDiagnosticQuery,
} from '@/services/admin-api'
import { friendlyErrorMessage, FrontendApiError } from '@/services/errors'

const PAGE_SIZE = 20

const FAILURE_KIND_LABELS: Record<string, string> = {
  resource: '资源加载',
  timeout: '编译超时',
  service: '服务不可用',
  latex: 'LaTeX 编译',
  unknown: '未知',
}

const FAILURE_STAGE_LABELS: Record<string, string> = {
  prepare: '准备文件',
  job_create: '创建任务',
  compile: '编译',
  upload: '上传结果',
}

const RESOURCE_KIND_LABELS: Record<string, string> = {
  package: '宏包',
  font: '字体',
}

interface Filters {
  failureKind: string
  engine: string
  identity: string
  from: string
  to: string
}

const EMPTY_FILTERS: Filters = { failureKind: 'all', engine: 'all', identity: 'all', from: '', to: '' }

function toQuery(filters: Filters): CompileDiagnosticQuery {
  return {
    failure_kind: filters.failureKind === 'all' ? undefined : filters.failureKind,
    engine: filters.engine === 'all' ? undefined : filters.engine,
    is_guest: filters.identity === 'all' ? undefined : filters.identity === 'guest',
    from: filters.from || undefined,
    to: filters.to || undefined,
  }
}

function formatTime(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN', { hour12: false })
}

function formatDuration(ms: number): string {
  return ms >= 1000 ? `${(ms / 1000).toFixed(1)} 秒` : `${ms} 毫秒`
}

function identityLabel(item: CompileDiagnosticItem): string {
  if (item.is_guest) return '游客'
  return item.username ?? '已注销用户'
}

function projectLabel(item: CompileDiagnosticItem): string {
  if (item.project_name) return item.project_name
  return item.project_id ? '已删除项目' : '—'
}

function failureKindVariant(kind: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  if (kind === 'latex') return 'destructive'
  if (kind === 'resource' || kind === 'service') return 'secondary'
  return 'outline'
}

export function CompileDiagnosticsPage() {
  const [filters, setFilters] = useState<Filters>(EMPTY_FILTERS)
  const [page, setPage] = useState(1)
  const [detailId, setDetailId] = useState<string | null>(null)

  const listQuery = useQuery({
    queryKey: ['admin-compile-diagnostics', filters, page],
    queryFn: () => listCompileDiagnostics({ ...toQuery(filters), page, page_size: PAGE_SIZE }),
    retry: false,
  })

  const updateFilter = <K extends keyof Filters>(key: K, value: Filters[K]) => {
    setFilters((current) => ({ ...current, [key]: value }))
    setPage(1)
  }

  const forbidden = listQuery.error instanceof FrontendApiError && listQuery.error.status === 403
  const items = listQuery.data?.items ?? []
  const total = listQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-end gap-2">
        <FilterSelect
          label="错误类型"
          value={filters.failureKind}
          onChange={(value) => updateFilter('failureKind', value)}
          options={[
            { value: 'all', label: '全部' },
            ...Object.entries(FAILURE_KIND_LABELS).map(([value, label]) => ({ value, label })),
          ]}
        />
        <FilterSelect
          label="引擎"
          value={filters.engine}
          onChange={(value) => updateFilter('engine', value)}
          options={[
            { value: 'all', label: '全部' },
            { value: 'pdflatex', label: 'pdfLaTeX' },
            { value: 'xelatex', label: 'XeLaTeX' },
          ]}
        />
        <FilterSelect
          label="身份"
          value={filters.identity}
          onChange={(value) => updateFilter('identity', value)}
          options={[
            { value: 'all', label: '全部' },
            { value: 'user', label: '登录用户' },
            { value: 'guest', label: '游客' },
          ]}
        />
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          开始日期
          <Input
            type="date"
            className="h-9 w-[150px]"
            value={filters.from}
            onChange={(event) => updateFilter('from', event.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1 text-xs text-muted-foreground">
          结束日期
          <Input
            type="date"
            className="h-9 w-[150px]"
            value={filters.to}
            onChange={(event) => updateFilter('to', event.target.value)}
          />
        </label>
        <Button
          variant="ghost"
          size="sm"
          className="h-9"
          onClick={() => {
            setFilters(EMPTY_FILTERS)
            setPage(1)
          }}
        >
          重置筛选
        </Button>
      </div>

      {listQuery.isLoading ? (
        <LoadingState title="正在加载编译错误日志" rows={3} />
      ) : forbidden ? (
        <EmptyState
          icon={ShieldAlert}
          title="需要管理员权限"
          description="当前账号没有查看编译错误日志的权限，请联系管理员。"
          action={
            <Button size="sm" variant="outline" asChild>
              <Link to="/app/projects">返回项目列表</Link>
            </Button>
          }
        />
      ) : listQuery.isError ? (
        <ErrorState
          title="无法加载编译错误日志"
          description={friendlyErrorMessage(listQuery.error)}
          onRetry={() => void listQuery.refetch()}
        />
      ) : items.length === 0 ? (
        <EmptyState
          title="暂无编译错误记录"
          description={
            Object.values(filters).every((value) => value === 'all' || value === '')
              ? '编译器目前没有报错记录。用户编译失败后，脱敏诊断会自动出现在这里。'
              : '当前筛选条件下没有记录，可尝试放宽筛选条件。'
          }
        />
      ) : (
        <>
          <div className="hidden md:block">
            <Card className="overflow-x-auto">
              <Table className="min-w-[1080px] table-fixed">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[150px]">时间</TableHead>
                    <TableHead className="w-[80px]">身份</TableHead>
                    <TableHead className="w-[150px]">项目</TableHead>
                    <TableHead className="w-[100px]">类型</TableHead>
                    <TableHead className="w-[90px]">引擎</TableHead>
                    <TableHead className="w-[90px]">耗时</TableHead>
                    <TableHead>摘要</TableHead>
                    <TableHead className="w-[100px] text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className="truncate text-xs">{formatTime(item.created_at)}</TableCell>
                      <TableCell className="truncate text-xs">{identityLabel(item)}</TableCell>
                      <TableCell className="truncate text-xs" title={projectLabel(item)}>{projectLabel(item)}</TableCell>
                      <TableCell>
                        <Badge variant={failureKindVariant(item.failure_kind)}>
                          {FAILURE_KIND_LABELS[item.failure_kind] ?? item.failure_kind}
                        </Badge>
                      </TableCell>
                      <TableCell className="truncate text-xs">{item.engine}</TableCell>
                      <TableCell className="truncate text-xs">{formatDuration(item.duration_ms)}</TableCell>
                      <TableCell className="truncate text-xs" title={item.error_message}>
                        {item.error_message}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button size="sm" variant="ghost" className="h-8" onClick={() => setDetailId(item.id)}>
                          查看详情
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Card>
          </div>

          <div className="space-y-3 md:hidden">
            {items.map((item) => (
              <Card key={item.id}>
                <CardContent className="space-y-2 p-3">
                  <div className="flex items-center justify-between gap-2">
                    <Badge variant={failureKindVariant(item.failure_kind)}>
                      {FAILURE_KIND_LABELS[item.failure_kind] ?? item.failure_kind}
                    </Badge>
                    <span className="text-[11px] text-muted-foreground">{formatTime(item.created_at)}</span>
                  </div>
                  <p className="line-clamp-2 text-xs">{item.error_message}</p>
                  <p className="text-[11px] text-muted-foreground">
                    {identityLabel(item)} · {projectLabel(item)} · {item.engine} · {formatDuration(item.duration_ms)}
                  </p>
                  <Button
                    size="sm"
                    variant="outline"
                    className="h-9 w-full"
                    onClick={() => setDetailId(item.id)}
                  >
                    查看详情
                  </Button>
                </CardContent>
              </Card>
            ))}
          </div>

          <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground">
            <span>共 {total} 条，第 {page} / {totalPages} 页</span>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                className="h-9"
                disabled={page <= 1 || listQuery.isFetching}
                onClick={() => setPage((current) => Math.max(1, current - 1))}
              >
                上一页
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="h-9"
                disabled={page >= totalPages || listQuery.isFetching}
                onClick={() => setPage((current) => Math.min(totalPages, current + 1))}
              >
                下一页
              </Button>
            </div>
          </div>
        </>
      )}

      <DiagnosticDetailSheet detailId={detailId} onClose={() => setDetailId(null)} />
    </div>
  )
}

function FilterSelect({ label, value, onChange, options }: {
  label: string
  value: string
  onChange: (value: string) => void
  options: Array<{ value: string; label: string }>
}) {
  return (
    <label className="flex flex-col gap-1 text-xs text-muted-foreground">
      {label}
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger className="h-9 w-[130px]" aria-label={label}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map((option) => (
            <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
          ))}
        </SelectContent>
      </Select>
    </label>
  )
}

function DiagnosticDetailSheet({ detailId, onClose }: { detailId: string | null; onClose: () => void }) {
  const detailQuery = useQuery({
    queryKey: ['admin-compile-diagnostic', detailId],
    queryFn: () => getCompileDiagnostic(detailId as string),
    enabled: detailId !== null,
    retry: false,
  })
  const detail = detailQuery.data ?? null

  const copyJson = async () => {
    if (!detail) return
    try {
      await navigator.clipboard.writeText(JSON.stringify(detail, null, 2))
      toast.success('已复制诊断 JSON')
    } catch {
      toast.error('复制失败，请手动选择文本复制')
    }
  }

  const exportJson = () => {
    if (!detail) return
    const blob = new Blob([JSON.stringify(detail, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `compile-diagnostic-${detail.id}.json`
    anchor.click()
    URL.revokeObjectURL(url)
  }

  return (
    <Sheet open={detailId !== null} onOpenChange={(open) => { if (!open) onClose() }}>
      <SheetContent side="right" className="w-full overflow-y-auto sm:max-w-2xl">
        <SheetHeader>
          <SheetTitle>编译错误详情</SheetTitle>
          <SheetDescription>
            {detail ? `${formatTime(detail.created_at)} · 诊断编号 ${detail.id}` : '正在加载诊断详情…'}
          </SheetDescription>
        </SheetHeader>

        {detailQuery.isLoading ? (
          <LoadingState title="正在加载诊断详情" rows={2} compact />
        ) : detailQuery.isError ? (
          <ErrorState
            title="无法加载诊断详情"
            description={friendlyErrorMessage(detailQuery.error)}
            onRetry={() => void detailQuery.refetch()}
            compact
          />
        ) : detail ? (
          <div className="space-y-4 px-4 pb-6">
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" className="h-9" onClick={() => void copyJson()}>
                <ClipboardCopy className="mr-1 size-3.5" />复制 JSON
              </Button>
              <Button size="sm" variant="outline" className="h-9" onClick={exportJson}>
                <Download className="mr-1 size-3.5" />导出 JSON
              </Button>
            </div>

            <DetailGrid detail={detail} />

            <section className="space-y-1">
              <h3 className="text-xs font-semibold text-muted-foreground">错误摘要</h3>
              <p className="select-text break-words rounded-lg bg-muted/40 p-3 text-xs">{detail.error_message}</p>
            </section>

            {detail.failed_resources && detail.failed_resources.length > 0 && (
              <section className="space-y-1">
                <h3 className="text-xs font-semibold text-muted-foreground">失败资源</h3>
                <div className="overflow-x-auto rounded-lg border">
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead>类别</TableHead>
                        <TableHead>名称</TableHead>
                        <TableHead>状态</TableHead>
                        <TableHead>HTTP</TableHead>
                        <TableHead>错误</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {detail.failed_resources.map((resource, index) => (
                        <TableRow key={`${resource.kind}:${resource.name}:${index}`}>
                          <TableCell className="text-xs">{RESOURCE_KIND_LABELS[resource.kind] ?? resource.kind}</TableCell>
                          <TableCell className="text-xs">{resource.name}</TableCell>
                          <TableCell className="text-xs">{resource.state}</TableCell>
                          <TableCell className="text-xs">{resource.status ?? '—'}</TableCell>
                          <TableCell className="max-w-[220px] truncate text-xs" title={resource.error}>
                            {resource.error ?? '—'}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              </section>
            )}

            <section className="space-y-1">
              <h3 className="text-xs font-semibold text-muted-foreground">脱敏日志片段</h3>
              {detail.log_excerpt ? (
                <pre className="max-h-72 select-text overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/40 p-3 font-mono text-[11px] leading-relaxed">
                  {detail.log_excerpt}
                </pre>
              ) : (
                <p className="rounded-lg bg-muted/40 p-3 text-xs text-muted-foreground">
                  本次失败没有可展示的日志片段（仅 LaTeX 编译错误会保留诊断行）。
                </p>
              )}
            </section>

            <section className="space-y-1">
              <h3 className="text-xs font-semibold text-muted-foreground">客户端环境</h3>
              <pre className="select-text overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/40 p-3 font-mono text-[11px] leading-relaxed">
                {JSON.stringify(detail.client_env ?? {}, null, 2)}
              </pre>
            </section>
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

function DetailGrid({ detail }: { detail: CompileDiagnosticDetail }) {
  const rows: Array<[string, string]> = [
    ['身份', identityLabel(detail)],
    ['项目', projectLabel(detail)],
    ['错误类型', FAILURE_KIND_LABELS[detail.failure_kind] ?? detail.failure_kind],
    ['失败阶段', FAILURE_STAGE_LABELS[detail.failure_stage] ?? detail.failure_stage],
    ['引擎', `${detail.engine}${detail.engine_version ? ` (${detail.engine_version})` : ''}`],
    ['资源目录版本', detail.resource_profile || '—'],
    ['耗时', formatDuration(detail.duration_ms)],
    ['HTTP 状态', detail.http_status === null ? '—' : String(detail.http_status)],
    ['客户端', detail.client_kind === 'desktop' ? '桌面端' : '网页'],
    ['关联编译任务', detail.compile_job_id ?? '—'],
  ]
  return (
    <dl className="grid grid-cols-1 gap-2 rounded-lg border p-3 text-xs sm:grid-cols-2">
      {rows.map(([label, value]) => (
        <div key={label} className="flex gap-2">
          <dt className="w-24 shrink-0 text-muted-foreground">{label}</dt>
          <dd className="min-w-0 break-words">{value}</dd>
        </div>
      ))}
    </dl>
  )
}
