import { useState } from 'react'
import { BookMarked, ExternalLink, LoaderCircle, RefreshCw, TriangleAlert } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { friendlyErrorMessage } from '@/services/errors'
import {
  adoptionIndex,
  adoptionSummary,
  useAdoptNorms,
  useNormEntries,
  useNormPacks,
  useProjectNorms,
  useUnadoptNorm,
  useUpdateAdoption,
} from '@/services/norms-api'
import { bindingLabel, scenarioLabel, type NormEntry, type NormScenario } from '@/types/norms'

const SCENARIO_FILTERS: NormScenario[] = ['undergraduate_thesis', 'intro_paper', 'all']

/** 项目准备页的「学术规范」卡片：显示已采用摘要，点「管理」在抽屉里挑选。 */
export function NormLibraryPanel({ projectId }: { projectId: string }) {
  const [open, setOpen] = useState(false)
  const { data: adoptions = [], isLoading, error, refetch } = useProjectNorms(projectId)
  const summary = adoptionSummary(adoptions)

  return (
    <Card className="rounded-2xl border-border/50 shadow-sm">
      <CardHeader className="px-4 pb-2 pt-4">
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2 text-sm">
            <BookMarked className="size-4 text-primary" />学术规范
          </CardTitle>
          <Button size="sm" variant="outline" className="min-h-11" onClick={() => setOpen(true)}>
            管理
          </Button>
        </div>
      </CardHeader>
      <CardContent className="space-y-2 px-4 pb-4">
        {isLoading ? (
          <p role="status" className="flex items-center gap-2 text-xs text-muted-foreground">
            <LoaderCircle className="size-3.5 animate-spin" />正在读取已采用的规范…
          </p>
        ) : error ? (
          <div role="alert" className="space-y-2 text-xs text-muted-foreground">
            <p>没读到已采用的规范：{friendlyErrorMessage(error)}</p>
            <Button size="sm" variant="outline" className="min-h-11" onClick={() => void refetch()}>重试</Button>
          </div>
        ) : summary.total === 0 ? (
          <p className="text-xs leading-5 text-muted-foreground">
            还没有采用任何学术规范。采用后，写作建议与跨章节检查会引用这些条款并标出来源。
          </p>
        ) : (
          <>
            <p className="text-sm font-semibold">
              已采用 {summary.total} 条 · 已启用 {summary.enabled} 条
            </p>
            <div className="flex flex-wrap gap-1.5">
              {summary.categories.map((category) => (
                <Badge key={category} variant="secondary" className="font-normal">{category}</Badge>
              ))}
            </div>
            {summary.outdated > 0 && (
              <p className="text-xs text-amber-700">{summary.outdated} 条规范有新版本；已采用的论文不受影响，想换到新版本请先取消采用再重新采用。</p>
            )}
          </>
        )}
      </CardContent>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="right" className="flex w-full flex-col gap-3 overflow-hidden p-4 sm:max-w-lg">
          <SheetHeader className="p-0">
            <SheetTitle>选择本项目的学术规范</SheetTitle>
            <SheetDescription>
              采用后会固定当前版本；规范更新不会自动改变已采用的论文。
            </SheetDescription>
          </SheetHeader>
          {open && <NormPicker projectId={projectId} />}
        </SheetContent>
      </Sheet>
    </Card>
  )
}

function NormPicker({ projectId }: { projectId: string }) {
  const [keyword, setKeyword] = useState('')
  const [packId, setPackId] = useState('')
  const [scenario, setScenario] = useState<NormScenario | ''>('')

  const packs = useNormPacks()
  const adoptions = useProjectNorms(projectId)
  const entries = useNormEntries({
    q: keyword.trim() || undefined,
    packId: packId || undefined,
    scenario: scenario || undefined,
    pageSize: 50,
  })
  const adopt = useAdoptNorms(projectId)
  const unadopt = useUnadoptNorm(projectId)
  const update = useUpdateAdoption(projectId)
  const adopted = adoptionIndex(adoptions.data ?? [])
  const busy = adopt.isPending || unadopt.isPending || update.isPending
  const mutationError = adopt.error ?? unadopt.error ?? update.error

  const selectedPack = (packs.data ?? []).find((pack) => pack.id === packId)

  return (
    <div className="flex min-h-0 flex-1 flex-col gap-3">
      <Input
        value={keyword}
        onChange={(event) => setKeyword(event.target.value)}
        placeholder="搜标题、编号或出处，例如 GB/T 7714"
        aria-label="搜索规范条目"
        className="min-h-11"
      />

      <div className="flex flex-wrap gap-1.5" role="group" aria-label="按场景筛选">
        <Button
          size="sm"
          variant={scenario === '' ? 'default' : 'outline'}
          className="min-h-11"
          onClick={() => setScenario('')}
        >
          全部场景
        </Button>
        {SCENARIO_FILTERS.map((item) => (
          <Button
            key={item}
            size="sm"
            variant={scenario === item ? 'default' : 'outline'}
            className="min-h-11"
            onClick={() => setScenario(item)}
          >
            {scenarioLabel(item)}
          </Button>
        ))}
      </div>

      <div className="flex flex-wrap gap-1.5" role="group" aria-label="按规范包筛选">
        <Button
          size="sm"
          variant={packId === '' ? 'default' : 'outline'}
          className="min-h-11"
          onClick={() => setPackId('')}
        >
          全部规范包
        </Button>
        {(packs.data ?? []).map((pack) => (
          <Button
            key={pack.id}
            size="sm"
            variant={packId === pack.id ? 'default' : 'outline'}
            className="min-h-11"
            onClick={() => setPackId(pack.id)}
          >
            {pack.name}
          </Button>
        ))}
      </div>

      {selectedPack && (
        <Button className="min-h-11" disabled={busy} onClick={() => adopt.mutate({ packId: selectedPack.id })}>
          采用「{selectedPack.name}」的全部 {selectedPack.entryCount} 条
        </Button>
      )}

      {mutationError && (
        <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-xs text-destructive">
          {friendlyErrorMessage(mutationError)}
        </p>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto pr-1">
        {entries.isLoading ? (
          <p role="status" className="flex items-center gap-2 p-4 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" />正在读取规范条目…
          </p>
        ) : entries.error ? (
          <div role="alert" className="space-y-2 p-4 text-sm text-muted-foreground">
            <p className="flex items-center gap-2"><TriangleAlert className="size-4" />读取失败：{friendlyErrorMessage(entries.error)}</p>
            <Button size="sm" variant="outline" className="min-h-11" onClick={() => void entries.refetch()}>
              <RefreshCw className="size-4" />重试
            </Button>
          </div>
        ) : (entries.data?.items.length ?? 0) === 0 ? (
          <p className="p-4 text-sm leading-6 text-muted-foreground">
            没有匹配的规范条目。换个关键词，或清空筛选再看看。
          </p>
        ) : (
          <ul className="space-y-2">
            {(entries.data?.items ?? []).map((entry) => (
              <EntryRow
                key={entry.id}
                entry={entry}
                adoptedState={adopted.get(entry.id)?.state}
                hasNewerVersion={adopted.get(entry.id)?.hasNewerVersion ?? false}
                busy={busy}
                onAdopt={() => adopt.mutate({ entryIds: [entry.id] })}
                onToggle={() => {
                  const current = adopted.get(entry.id)
                  if (!current) return
                  update.mutate({
                    entryId: entry.id,
                    state: current.state === 'enabled' ? 'disabled' : 'enabled',
                  })
                }}
                onRemove={() => unadopt.mutate(entry.id)}
              />
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}

interface EntryRowProps {
  entry: NormEntry
  adoptedState?: 'enabled' | 'disabled'
  hasNewerVersion: boolean
  busy: boolean
  onAdopt: () => void
  onToggle: () => void
  onRemove: () => void
}

function EntryRow({
  entry,
  adoptedState,
  hasNewerVersion,
  busy,
  onAdopt,
  onToggle,
  onRemove,
}: EntryRowProps) {
  return (
    <li className="rounded-xl border border-border/60 p-3">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium leading-5">{entry.title}</p>
          <p className="mt-1 text-xs text-muted-foreground">
            {entry.packName} · {entry.code}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge variant={entry.binding === 'must' ? 'default' : 'outline'} className="font-normal">
            {bindingLabel(entry.binding)}
          </Badge>
          {adoptedState === 'enabled' && (
            <Badge variant="secondary" className="font-normal">已启用</Badge>
          )}
          {adoptedState === 'disabled' && (
            <Badge variant="outline" className="font-normal">已停用</Badge>
          )}
        </div>
      </div>

      <p className="mt-2 whitespace-pre-wrap text-xs leading-5 text-muted-foreground">{entry.ruleText}</p>

      <details className="mt-2">
        <summary className="min-h-11 cursor-pointer list-none py-2 text-xs font-medium text-primary">
          查看出处与适用章节
        </summary>
        <div className="space-y-1 text-xs leading-5 text-muted-foreground">
          <p>出处：{entry.sourceRef}</p>
          {entry.sourceQuote && <p>原文：{entry.sourceQuote}</p>}
          {entry.applicableSections.length > 0 && <p>适用：{entry.applicableSections.join('、')}</p>}
          {entry.sourceUrl && (
            <a
              href={entry.sourceUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-primary underline"
            >
              打开来源<ExternalLink className="size-3" />
            </a>
          )}
        </div>
      </details>

      <div className="mt-1 flex flex-wrap items-center gap-2">
        {adoptedState ? (
          <>
            <Button size="sm" variant="outline" className="min-h-11" disabled={busy} onClick={onToggle}>
              {adoptedState === 'enabled' ? '本次停用' : '重新启用'}
            </Button>
            <Button size="sm" variant="ghost" className="min-h-11" disabled={busy} onClick={onRemove}>
              取消采用
            </Button>
            {hasNewerVersion && (
              <span className="text-xs text-amber-700">
                目录里已有更新版本；本项目仍按采用时的版本生效。想换到新版本，请先取消采用，再重新采用。
              </span>
            )}
          </>
        ) : (
          <Button size="sm" className="min-h-11" disabled={busy} onClick={onAdopt}>
            采用这条
          </Button>
        )}
      </div>
    </li>
  )
}
