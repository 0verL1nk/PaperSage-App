import { useState } from 'react'
import { BookMarked, LoaderCircle, Pencil, Plus, RefreshCw, Trash2, TriangleAlert } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { Textarea } from '@/components/ui/textarea'
import { friendlyErrorMessage } from '@/services/errors'
import {
  useCreateEntry,
  useCreatePack,
  useDeleteEntry,
  useDeletePack,
  useNormEntries,
  useNormEntry,
  useNormPacks,
  useUpdateEntry,
} from '@/services/norms-api'
import {
  bindingLabel,
  scenarioLabel,
  type NormBinding,
  type NormEntryDetail,
  type NormEntryInput,
  type NormPack,
  type NormScenario,
} from '@/types/norms'
import { cn } from '@/lib/utils'

const SCENARIOS: NormScenario[] = ['undergraduate_thesis', 'intro_paper', 'all']
const BINDINGS: NormBinding[] = ['must', 'should', 'recommended']

const emptyDraft = (packId: string, scenario: NormScenario): NormEntryInput => ({
  packId,
  code: '',
  category: '格式排版',
  ruleKind: 'user',
  binding: 'should',
  scenario,
  tags: [],
  title: '',
  ruleText: '',
  applicableSections: [],
  sourceRef: '',
  sourceQuote: '',
})

export function NormsPage() {
  const packs = useNormPacks()
  const [packId, setPackId] = useState('')
  const [keyword, setKeyword] = useState('')
  const [scenario, setScenario] = useState<NormScenario | ''>('')
  const [detailId, setDetailId] = useState<string | null>(null)
  const [draft, setDraft] = useState<{ entryId?: string; value: NormEntryInput; expectedVersion?: number } | null>(null)
  const [packDraft, setPackDraft] = useState<{ key: string; name: string; scenario: NormScenario } | null>(null)

  const entries = useNormEntries({
    packId: packId || undefined,
    q: keyword.trim() || undefined,
    scenario: scenario || undefined,
    pageSize: 50,
  })
  const createEntry = useCreateEntry()
  const updateEntry = useUpdateEntry()
  const deleteEntry = useDeleteEntry()
  const createPack = useCreatePack()
  const deletePack = useDeletePack()

  const selectedPack: NormPack | undefined = (packs.data ?? []).find((pack) => pack.id === packId)
  const editablePack = Boolean(selectedPack && !selectedPack.isBuiltin)
  const mutationError = createEntry.error ?? updateEntry.error ?? deleteEntry.error ?? createPack.error ?? deletePack.error

  const startCreate = () => {
    if (!selectedPack || selectedPack.isBuiltin) return
    setDraft({ value: emptyDraft(selectedPack.id, selectedPack.scenario) })
  }

  const startEdit = (entry: NormEntryDetail) => {
    setDetailId(null)
    setDraft({
      entryId: entry.id,
      expectedVersion: entry.version,
      value: {
        packId: entry.packId,
        code: entry.code,
        category: entry.category,
        ruleKind: entry.ruleKind,
        binding: entry.binding,
        scenario: entry.scenario,
        sortOrder: entry.sortOrder,
        tags: entry.tags,
        title: entry.title,
        ruleText: entry.ruleText,
        rationale: entry.rationale,
        applicableSections: entry.applicableSections,
        sourceRef: entry.sourceRef,
        sourceQuote: entry.sourceQuote,
        sourceUrl: entry.sourceUrl,
      },
    })
  }

  const submitDraft = () => {
    if (!draft) return
    const done = () => setDraft(null)
    if (draft.entryId) {
      updateEntry.mutate(
        { entryId: draft.entryId, expectedVersion: draft.expectedVersion ?? 1, ...draft.value },
        { onSuccess: done },
      )
    } else {
      createEntry.mutate(draft.value, { onSuccess: done })
    }
  }

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 overflow-y-auto p-3 sm:p-5 lg:h-full lg:min-h-0 lg:overflow-hidden">
      <header className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-primary">学术规范</p>
          <h1 className="mt-1 text-2xl font-semibold tracking-tight">规范库</h1>
          <p className="mt-1 text-sm text-muted-foreground">内置规范开箱可用；自建包里的条目可以自己增删改。</p>
        </div>
        <Button className="min-h-11" onClick={() => setPackDraft({ key: '', name: '', scenario: 'undergraduate_thesis' })}>
          <Plus className="size-4" />新建规范包
        </Button>
      </header>

      {mutationError && (
        <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {friendlyErrorMessage(mutationError)}
        </p>
      )}

      <div className="grid gap-4 lg:min-h-0 lg:flex-1 lg:grid-cols-[260px_minmax(0,1fr)]">
        <Card className="max-h-64 overflow-y-auto rounded-2xl border-border/50 shadow-sm lg:max-h-none lg:min-h-0">
          <CardHeader className="px-4 pb-2 pt-4">
            <CardTitle className="flex items-center gap-2 text-sm"><BookMarked className="size-4 text-primary" />规范包</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 px-2 pb-4">
            {packs.isLoading ? (
              <p role="status" className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground">
                <LoaderCircle className="size-3.5 animate-spin" />正在读取…
              </p>
            ) : packs.error ? (
              <div role="alert" className="space-y-2 px-2 py-3 text-xs text-muted-foreground">
                <p>读取失败：{friendlyErrorMessage(packs.error)}</p>
                <Button size="sm" variant="outline" className="min-h-11" onClick={() => void packs.refetch()}>重试</Button>
              </div>
            ) : (
              <>
                <PackButton active={packId === ''} onClick={() => setPackId('')} name="全部规范包" meta="内置与自建" />
                {(packs.data ?? []).map((pack) => (
                  <PackButton
                    key={pack.id}
                    active={packId === pack.id}
                    onClick={() => setPackId(pack.id)}
                    name={pack.name}
                    meta={`${pack.isBuiltin ? '内置' : '自建'} · ${pack.entryCount} 条 · ${scenarioLabel(pack.scenario)}`}
                  />
                ))}
              </>
            )}
          </CardContent>
        </Card>

        <Card className="flex flex-col rounded-2xl border-border/50 shadow-sm lg:min-h-0">
          <CardHeader className="gap-3 px-4 pb-2 pt-4">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <CardTitle className="text-sm">{selectedPack ? selectedPack.name : '全部条目'}</CardTitle>
              <div className="flex flex-wrap gap-2">
                <Button
                  size="sm"
                  className="min-h-11"
                  disabled={!editablePack}
                  title={editablePack ? undefined : '内置规范只读；先新建自己的规范包再添加条目'}
                  onClick={startCreate}
                >
                  <Plus className="size-4" />新增条目
                </Button>
                {selectedPack && !selectedPack.isBuiltin && (
                  <Button
                    size="sm"
                    variant="outline"
                    className="min-h-11"
                    disabled={deletePack.isPending}
                    onClick={() => {
                      if (window.confirm(`删除规范包「${selectedPack.name}」？包内条目会一并删除；已被项目采用的包不能删除。`)) {
                        deletePack.mutate(selectedPack.id, { onSuccess: () => setPackId('') })
                      }
                    }}
                  >
                    <Trash2 className="size-4" />删除规范包
                  </Button>
                )}
              </div>
            </div>
            <CardDescription>
              {editablePack ? '这是你自建的规范包，条目可以新增、编辑与删除。' : '内置规范包只读；点条目可查看出处与版本历史。'}
            </CardDescription>
            <div className="flex flex-col gap-2">
              <Input
                value={keyword}
                onChange={(event) => setKeyword(event.target.value)}
                placeholder="搜标题、编号或出处"
                aria-label="搜索规范条目"
                className="min-h-11"
              />
              <div className="flex flex-wrap gap-1.5" role="group" aria-label="按场景筛选">
                <Button size="sm" variant={scenario === '' ? 'default' : 'outline'} className="min-h-11" onClick={() => setScenario('')}>全部场景</Button>
                {SCENARIOS.map((item) => (
                  <Button key={item} size="sm" variant={scenario === item ? 'default' : 'outline'} className="min-h-11" onClick={() => setScenario(item)}>
                    {scenarioLabel(item)}
                  </Button>
                ))}
              </div>
            </div>
          </CardHeader>

          <CardContent className="px-4 pb-4 lg:min-h-0 lg:flex-1 lg:overflow-y-auto">
            {entries.isLoading ? (
              <p role="status" className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
                <LoaderCircle className="size-4 animate-spin" />正在读取规范条目…
              </p>
            ) : entries.error ? (
              <div role="alert" className="space-y-2 py-6 text-sm text-muted-foreground">
                <p className="flex items-center gap-2"><TriangleAlert className="size-4" />读取失败：{friendlyErrorMessage(entries.error)}</p>
                <Button size="sm" variant="outline" className="min-h-11" onClick={() => void entries.refetch()}>
                  <RefreshCw className="size-4" />重试
                </Button>
              </div>
            ) : (entries.data?.items.length ?? 0) === 0 ? (
              <p className="py-8 text-center text-sm leading-6 text-muted-foreground">
                {keyword.trim() || scenario
                  ? '没有匹配的规范条目。换个关键词或筛选条件试试。'
                  : selectedPack
                    ? '这个包还没有条目。点「新增条目」开始添加。'
                    : '这个库里还没有条目。'}
              </p>
            ) : (
              <ul className="space-y-2">
                {(entries.data?.items ?? []).map((entry) => (
                  <li key={entry.id}>
                    <button
                      type="button"
                      onClick={() => setDetailId(entry.id)}
                      className="min-h-11 w-full rounded-xl border border-border/60 p-3 text-left transition-colors hover:bg-muted/40"
                    >
                      <div className="flex flex-wrap items-start justify-between gap-2">
                        <div className="min-w-0">
                          <p className="text-sm font-medium leading-5">{entry.title}</p>
                          <p className="mt-1 text-xs text-muted-foreground">
                            {entry.packName} · {entry.code} · v{entry.version} · {entry.category}
                          </p>
                        </div>
                        <Badge variant={entry.binding === 'must' ? 'default' : 'outline'} className="font-normal">
                          {bindingLabel(entry.binding)}
                        </Badge>
                      </div>
                      <p className="mt-2 line-clamp-2 text-xs leading-5 text-muted-foreground">{entry.ruleText}</p>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      </div>

      <EntryDetailSheet
        entryId={detailId}
        onClose={() => setDetailId(null)}
        onEdit={startEdit}
        onDelete={(entryId) => {
          if (window.confirm('删除这条规范？删除后无法恢复。')) {
            deleteEntry.mutate(entryId, { onSuccess: () => setDetailId(null) })
          }
        }}
        busy={deleteEntry.isPending}
      />

      {draft && (
        <EntryEditor
          draft={draft.value}
          editing={Boolean(draft.entryId)}
          busy={createEntry.isPending || updateEntry.isPending}
          error={createEntry.error ?? updateEntry.error}
          onChange={(value) => setDraft({ ...draft, value })}
          onCancel={() => setDraft(null)}
          onSubmit={submitDraft}
        />
      )}

      {packDraft && (
        <PackEditor
          draft={packDraft}
          busy={createPack.isPending}
          error={createPack.error}
          onChange={setPackDraft}
          onCancel={() => setPackDraft(null)}
          onSubmit={() =>
            createPack.mutate(
              { key: packDraft.key.trim(), name: packDraft.name.trim(), scenario: packDraft.scenario },
              {
                onSuccess: (created) => {
                  setPackDraft(null)
                  // 建完立刻切到新包：否则停在「全部规范包」，「新增条目」仍是禁用的，
                  // 用户会以为刚建的包不能加条目。
                  setPackId(created.id)
                },
              },
            )
          }
        />
      )}
    </div>
  )
}

function PackButton({ active, name, meta, onClick }: { active: boolean; name: string; meta: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'min-h-11 w-full rounded-lg px-3 py-2 text-left transition-colors',
        active ? 'bg-primary/10 text-primary' : 'hover:bg-muted/60',
      )}
    >
      <span className="block truncate text-sm font-medium">{name}</span>
      <span className="mt-0.5 block truncate text-[11px] text-muted-foreground">{meta}</span>
    </button>
  )
}

function EntryDetailSheet({
  entryId,
  onClose,
  onEdit,
  onDelete,
  busy,
}: {
  entryId: string | null
  onClose: () => void
  onEdit: (entry: NormEntryDetail) => void
  onDelete: (entryId: string) => void
  busy: boolean
}) {
  const detail = useNormEntry(entryId ?? undefined)
  const entry = detail.data

  return (
    <Sheet open={Boolean(entryId)} onOpenChange={(open) => !open && onClose()}>
      <SheetContent side="right" className="flex w-full flex-col gap-3 overflow-y-auto p-4 sm:max-w-lg">
        <SheetHeader className="p-0">
          <SheetTitle>{entry?.title ?? '规范条目'}</SheetTitle>
        </SheetHeader>
        {detail.isLoading ? (
          <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
            <LoaderCircle className="size-4 animate-spin" />正在读取…
          </p>
        ) : detail.error ? (
          <div role="alert" className="space-y-2 text-sm text-muted-foreground">
            <p>读取失败：{friendlyErrorMessage(detail.error)}</p>
            <Button size="sm" variant="outline" className="min-h-11" onClick={() => void detail.refetch()}>重试</Button>
          </div>
        ) : entry ? (
          <>
            <p className="text-xs text-muted-foreground">
              {entry.packName} · {entry.code} · 当前 v{entry.version} · {entry.category} · {bindingLabel(entry.binding)}
            </p>
            <p className="whitespace-pre-wrap text-sm leading-6">{entry.ruleText}</p>
            {entry.rationale && <p className="text-xs leading-5 text-muted-foreground">为什么：{entry.rationale}</p>}
            <div className="rounded-xl bg-muted/40 p-3 text-xs leading-5">
              <p className="font-semibold">出处</p>
              <p className="mt-1">{entry.sourceRef}</p>
              {entry.sourceQuote && <p className="mt-1 text-muted-foreground">原文：{entry.sourceQuote}</p>}
              {entry.applicableSections.length > 0 && (
                <p className="mt-1 text-muted-foreground">适用：{entry.applicableSections.join('、')}</p>
              )}
            </div>
            <div>
              <p className="text-xs font-semibold">版本历史</p>
              <ul className="mt-1 space-y-1">
                {entry.versions.map((version) => (
                  <li key={version.version} className="text-xs text-muted-foreground">
                    v{version.version} · {version.title}
                  </li>
                ))}
              </ul>
            </div>
            {!entry.isBuiltin && (
              <div className="flex flex-wrap gap-2 pt-2">
                <Button size="sm" className="min-h-11" onClick={() => onEdit(entry)}>
                  <Pencil className="size-4" />编辑
                </Button>
                <Button size="sm" variant="outline" className="min-h-11" disabled={busy} onClick={() => onDelete(entry.id)}>
                  <Trash2 className="size-4" />删除
                </Button>
              </div>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground">这条规范不存在或已被删除。</p>
        )}
      </SheetContent>
    </Sheet>
  )
}

interface FieldProps {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
  textarea?: boolean
  required?: boolean
}

function Field({ label, value, onChange, placeholder, textarea, required }: FieldProps) {
  return (
    <label className="block text-sm">
      <span className="text-xs font-medium text-muted-foreground">{label}{required ? '（必填）' : ''}</span>
      {textarea ? (
        <Textarea className="mt-1 min-h-24" value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
      ) : (
        <Input className="mt-1 min-h-11" value={value} placeholder={placeholder} onChange={(event) => onChange(event.target.value)} />
      )}
    </label>
  )
}

function EntryEditor({
  draft,
  editing,
  busy,
  error,
  onChange,
  onCancel,
  onSubmit,
}: {
  draft: NormEntryInput
  editing: boolean
  busy: boolean
  error: unknown
  onChange: (value: NormEntryInput) => void
  onCancel: () => void
  onSubmit: () => void
}) {
  const set = <K extends keyof NormEntryInput>(key: K, value: NormEntryInput[K]) => onChange({ ...draft, [key]: value })
  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{editing ? '编辑规范条目' : '新增规范条目'}</DialogTitle>
          <DialogDescription>修改会追加一个新版本，已采用旧版本的项目不会自动变化。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Field label="标题" required value={draft.title} onChange={(value) => set('title', value)} />
          <Field label="条目编号" required value={draft.code} placeholder="例如 MY-LAB-001" onChange={(value) => set('code', value)} />
          <Field label="分类" required value={draft.category} placeholder="格式排版 / 参考文献 / 图表公式…" onChange={(value) => set('category', value)} />
          <div>
            <span className="text-xs font-medium text-muted-foreground">强制程度</span>
            <div className="mt-1 flex flex-wrap gap-1.5">
              {BINDINGS.map((binding) => (
                <Button
                  key={binding}
                  size="sm"
                  variant={draft.binding === binding ? 'default' : 'outline'}
                  className="min-h-11"
                  onClick={() => set('binding', binding)}
                >
                  {bindingLabel(binding)}
                </Button>
              ))}
            </div>
          </div>
          <Field label="规则说明" required textarea value={draft.ruleText} onChange={(value) => set('ruleText', value)} />
          <Field label="来源出处" required value={draft.sourceRef} placeholder="例如 GB/T 7714—2015 第 8 章" onChange={(value) => set('sourceRef', value)} />
          <Field label="原文摘录" textarea value={draft.sourceQuote ?? ''} onChange={(value) => set('sourceQuote', value)} />
          <Field
            label="标签"
            value={draft.tags.join('、')}
            placeholder="用顿号或逗号分隔"
            onChange={(value) => set('tags', value.split(/[、,，]/).map((item) => item.trim()).filter(Boolean))}
          />
          <Field
            label="适用章节"
            value={draft.applicableSections.join('、')}
            placeholder="例如 摘要、参考文献"
            onChange={(value) => set('applicableSections', value.split(/[、,，]/).map((item) => item.trim()).filter(Boolean))}
          />
          {Boolean(error) && (
            <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {friendlyErrorMessage(error)}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" className="min-h-11" onClick={onCancel}>取消</Button>
          <Button className="min-h-11" disabled={busy} onClick={onSubmit}>{busy ? '保存中…' : '保存'}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function PackEditor({
  draft,
  busy,
  error,
  onChange,
  onCancel,
  onSubmit,
}: {
  draft: { key: string; name: string; scenario: NormScenario }
  busy: boolean
  error: unknown
  onChange: (value: { key: string; name: string; scenario: NormScenario }) => void
  onCancel: () => void
  onSubmit: () => void
}) {
  return (
    <Dialog open onOpenChange={(open) => !open && onCancel()}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>新建规范包</DialogTitle>
          <DialogDescription>自建包只对你自己可见，可以放学校或实验室的补充要求。</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <Field label="名称" required value={draft.name} onChange={(value) => onChange({ ...draft, name: value })} />
          <Field
            label="标识"
            required
            value={draft.key}
            placeholder="小写字母、数字与连字符，例如 my-lab-rules"
            onChange={(value) => onChange({ ...draft, key: value })}
          />
          <div>
            <span className="text-xs font-medium text-muted-foreground">适用场景</span>
            <div className="mt-1 flex flex-wrap gap-1.5">
              {SCENARIOS.map((item) => (
                <Button
                  key={item}
                  size="sm"
                  variant={draft.scenario === item ? 'default' : 'outline'}
                  className="min-h-11"
                  onClick={() => onChange({ ...draft, scenario: item })}
                >
                  {scenarioLabel(item)}
                </Button>
              ))}
            </div>
          </div>
          {Boolean(error) && (
            <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {friendlyErrorMessage(error)}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button variant="outline" className="min-h-11" onClick={onCancel}>取消</Button>
          <Button className="min-h-11" disabled={busy || !draft.name.trim() || !draft.key.trim()} onClick={onSubmit}>
            {busy ? '创建中…' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
