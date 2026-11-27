import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, FileText, Loader2, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useLatexFiles } from '@/hooks/use-latex'
import { friendlyErrorMessage } from '@/services/errors'
import {
  useAddResearchRevision,
  useAdoptResearchRevision,
  useCreateResearchArtifact,
  useDeleteResearchArtifact,
  useRejectResearchRevision,
  useResearchArtifacts,
} from '@/services/research-api'
import type {
  ChapterPlanPayload,
  ChapterSection,
  ResearchArtifact,
  ResearchArtifactKind,
  ResearchField,
  ResearchPayload,
} from '@/types/research'
import { RESEARCH_KIND_LABEL, RESEARCH_STATUS_LABEL } from '@/types/research'

// chapter_review 是检查报告，归「检查完善」页所有；不混进资料组织列表。
const KINDS: Array<ResearchArtifactKind | 'all'> = [
  'all', 'literature_note', 'literature_comparison', 'chapter_plan', 'chapter_digest',
]

type ActionKind = 'adopt' | 'reject' | 'delete' | 'save'

/** 同一操作重复失败时按「标题 + 文案」合并，避免连续点击把 toast 堆满（HELE-22 S6）。 */
function showActionError(title: string, error: unknown) {
  const description = friendlyErrorMessage(error)
  toast.error(title, { id: `${title}:${description}`, description })
}

function artifactTitle(artifact: ResearchArtifact): string {
  const payload = artifact.current?.payload as unknown as Record<string, unknown> | undefined
  if (!payload) return RESEARCH_KIND_LABEL[artifact.kind]
  if (artifact.kind === 'literature_note') return String(payload.title || '阅读记录')
  if (artifact.kind === 'literature_comparison') return String(payload.question || '文献比较')
  if (artifact.kind === 'chapter_plan') {
    const sections = (payload as unknown as ChapterPlanPayload).sections
    return sections?.length ? `章节计划（${sections.length} 节）` : '章节计划'
  }
  if (artifact.kind === 'chapter_digest') return String(payload.sectionId || '章节摘要')
  return RESEARCH_KIND_LABEL[artifact.kind]
}

function FieldView({ label, field }: { label: string; field: ResearchField }) {
  const tone = field.status === 'reported' ? 'text-foreground' : 'text-amber-700'
  return (
    <div className="grid grid-cols-[5.5rem_1fr] gap-2 text-xs leading-5">
      <span className="text-muted-foreground">{label}</span>
      <div className={tone}>
        <span>{field.text || (field.status === 'not_read' ? '未读' : field.status === 'insufficient' ? '材料不足' : '未报告')}</span>
        {field.status !== 'reported' && <Badge variant="outline" className="ml-2 font-normal">{field.status === 'not_read' ? '未读' : field.status === 'insufficient' ? '材料不足' : '未报告'}</Badge>}
        {field.evidence?.length ? (
          <div className="mt-1 space-y-1 text-[11px] text-muted-foreground">
            {field.evidence.map((ref, index) => (
              <div key={index}>依据：{ref.quote || ref.sourceVersionId || ref.documentId || '未提供定位'}{ref.startByte != null ? `（${ref.startByte}-${ref.endByte ?? ''}）` : ''}</div>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}

function PayloadView({ payload, kind }: { payload: ResearchPayload; kind: ResearchArtifactKind }) {
  if (kind === 'literature_note') {
    const note = payload as unknown as Record<string, ResearchField>
    return (
      <div className="space-y-1.5">
        {([['question', '问题'], ['method', '方法'], ['objects', '对象/数据'], ['conditions', '评价条件'], ['results', '结果'], ['limitations', '限制']] as const).map(([key, label]) => (
          <FieldView key={key} label={label} field={note[key] ?? { text: '', status: 'not_reported' }} />
        ))}
      </div>
    )
  }
  if (kind === 'literature_comparison') {
    const p = payload as unknown as { question: string; axes: string[]; groups: Array<{ label?: string; values: ResearchField[] }>; differences?: ResearchField[]; conflicts?: ResearchField[]; missing?: ResearchField[] }
    return (
      <div className="space-y-2 text-xs">
        <p className="font-medium">{p.question}</p>
        <p className="text-muted-foreground">比较轴：{p.axes.join('；') || '未提供'}</p>
        {p.groups.map((group, index) => (
          <div key={index} className="rounded-lg border border-border/50 p-2">
            <p className="mb-1 font-medium">{group.label || `来源 ${index + 1}`}</p>
            {group.values.map((field, i) => <FieldView key={i} label={`轴 ${i + 1}`} field={field} />)}
          </div>
        ))}
        {[...(p.differences ?? []), ...(p.conflicts ?? []), ...(p.missing ?? [])].map((field, index) => (
          <FieldView key={`diff-${index}`} label="差异/冲突" field={field} />
        ))}
      </div>
    )
  }
  if (kind === 'chapter_plan') {
    const plan = payload as unknown as ChapterPlanPayload
    return (
      <div className="space-y-1.5">
        {plan.sections.map((section) => (
          <div key={section.id} className="rounded-lg border border-border/50 p-2 text-xs">
            <p className="font-medium">{section.order}. {section.title} <span className="text-muted-foreground">#{section.id}</span></p>
            <p className="mt-1 text-muted-foreground">{section.responsibility}</p>
            {section.dependencies?.length ? <p className="mt-1 text-[11px] text-muted-foreground">依赖：{section.dependencies.join('、')}</p> : null}
            {section.questions?.length ? <p className="mt-1 text-[11px] text-muted-foreground">问题：{section.questions.join('；')}</p> : null}
          </div>
        ))}
      </div>
    )
  }
  const digest = payload as unknown as { sectionId: string; content: string; coverage: string[]; terms?: string[] }
  return (
    <div className="space-y-1 text-xs">
      <p><span className="text-muted-foreground">章节：</span>{digest.sectionId}</p>
      <p className="leading-5">{digest.content}</p>
      <p className="text-muted-foreground">覆盖：{digest.coverage.join('；')}</p>
      {digest.terms?.length ? <p className="text-muted-foreground">术语：{digest.terms.join('、')}</p> : null}
    </div>
  )
}

function PlanEditor({
  artifact, projectId, busy, saving, onSave,
}: {
  artifact: ResearchArtifact
  projectId: string
  busy: boolean
  saving: boolean
  onSave: (payload: ChapterPlanPayload, expectedRevision: number) => void
}) {
  const current = artifact.current?.payload as unknown as ChapterPlanPayload | undefined
  const [sections, setSections] = useState<ChapterSection[]>(current?.sections ?? [])
  const { data: files = [] } = useLatexFiles(projectId)
  const texFiles = files.filter((file) => file.file_path.toLowerCase().endsWith('.tex'))
  const update = (index: number, patch: Partial<ChapterSection>) =>
    setSections((prev) => prev.map((section, i) => (i === index ? { ...section, ...patch } : section)))
  const add = () => setSections((prev) => [...prev, {
    id: `s${prev.length + 1}`, title: '', responsibility: '', order: prev.length + 1,
  }])
  return (
    <div className="mt-2 space-y-2 rounded-lg border border-dashed border-border/60 p-2">
      {sections.map((section, index) => (
        <div key={index} className="space-y-1">
          <div className="grid gap-1 sm:grid-cols-[4rem_1fr_1fr]">
            <input aria-label={`章节 ${index + 1} ID`} className="rounded border border-border/60 bg-background px-2 py-1 text-xs" value={section.id} onChange={(e) => update(index, { id: e.target.value })} />
            <input aria-label={`章节 ${index + 1} 标题`} className="rounded border border-border/60 bg-background px-2 py-1 text-xs" placeholder="标题" value={section.title} onChange={(e) => update(index, { title: e.target.value })} />
            <input aria-label={`章节 ${index + 1} 职责`} className="rounded border border-border/60 bg-background px-2 py-1 text-xs" placeholder="职责" value={section.responsibility} onChange={(e) => update(index, { responsibility: e.target.value })} />
          </div>
          <select
            aria-label={`章节 ${index + 1} 对应稿件文件`}
            className="w-full rounded border border-border/60 bg-background px-2 py-1 text-xs"
            value={section.filePath ?? ''}
            onChange={(e) => update(index, { filePath: e.target.value || undefined })}
          >
            <option value="">未关联稿件文件</option>
            {section.filePath && !texFiles.some((file) => file.file_path === section.filePath) ? (
              <option value={section.filePath}>{section.filePath}（当前项目中找不到）</option>
            ) : null}
            {texFiles.map((file) => <option key={file.file_path} value={file.file_path}>{file.file_path}</option>)}
          </select>
        </div>
      ))}
      <div className="flex justify-end gap-2">
        <Button size="sm" variant="outline" onClick={add} disabled={busy}><Plus className="mr-1 size-3" />添加章节</Button>
        <Button size="sm" disabled={busy || sections.length === 0} onClick={() => onSave({ sections }, artifact.currentRevision)}>
          {saving ? <><Loader2 className="mr-1 size-3 animate-spin" />保存中…</> : '保存新修订'}
        </Button>
      </div>
    </div>
  )
}

export function ResearchArtifactsPanel({ projectId }: { projectId: string }) {
  const [kind, setKind] = useState<ResearchArtifactKind | 'all'>('all')
  const [expanded, setExpanded] = useState<string | null>(null)
  const [editing, setEditing] = useState<string | null>(null)
  const [pending, setPending] = useState<{ id: string; action: ActionKind } | null>(null)
  const { data: artifacts = [], isLoading, error, refetch } = useResearchArtifacts(projectId)
  const create = useCreateResearchArtifact(projectId)
  const addRevision = useAddResearchRevision(projectId)
  const adopt = useAdoptResearchRevision(projectId)
  const reject = useRejectResearchRevision(projectId)
  const remove = useDeleteResearchArtifact(projectId)

  const filtered = useMemo(() => {
    const visible = artifacts.filter((artifact) => artifact.kind !== 'chapter_review')
    return kind === 'all' ? visible : visible.filter((artifact) => artifact.kind === kind)
  }, [artifacts, kind])

  const onCreatePlan = async () => {
    const payload: ChapterPlanPayload = { sections: [{ id: 's1', title: '引言', responsibility: '提出研究问题', order: 1 }] }
    try {
      const artifact = await create.mutateAsync({ kind: 'chapter_plan', payload, origin: 'user' })
      setExpanded(artifact.id)
      setEditing(artifact.id)
      toast.success('已创建章节计划')
    } catch (err) {
      showActionError('创建章节计划失败', err)
    }
  }

  // 统一处理失败提示与成功反馈：失败时编辑器保持打开，用户未保存的修改不丢。
  const runAction = async (
    artifactId: string,
    action: ActionKind,
    failTitle: string,
    successTitle: string,
    task: () => Promise<unknown>,
  ) => {
    setPending({ id: artifactId, action })
    try {
      await task()
      toast.success(successTitle)
      return true
    } catch (err) {
      showActionError(failTitle, err)
      return false
    } finally {
      setPending(null)
    }
  }

  return (
    <Card className="rounded-2xl border-border/50 shadow-sm">
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="flex items-center gap-2 text-sm"><FileText className="size-4 text-primary" />资料组织</CardTitle>
          <Button size="sm" variant="outline" onClick={onCreatePlan} disabled={create.isPending}>
            {create.isPending ? <><Loader2 className="mr-1 size-3 animate-spin" />创建中…</> : <><Plus className="mr-1 size-3" />新建章节计划</>}
          </Button>
        </div>
        <div className="flex flex-wrap gap-1 pt-2" role="tablist" aria-label="产物类型">
          {KINDS.map((item) => (
            <button
              key={item}
              role="tab"
              aria-selected={kind === item}
              className={`rounded-full px-2 py-0.5 text-[11px] ${kind === item ? 'bg-primary text-primary-foreground' : 'bg-muted text-muted-foreground'}`}
              onClick={() => setKind(item)}
            >
              {item === 'all' ? '全部' : RESEARCH_KIND_LABEL[item]}
            </button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="space-y-2 px-4 pb-4">
        {isLoading ? <p className="text-xs text-muted-foreground">正在加载…</p> : null}
        {error ? (
          <div role="alert" className="flex flex-wrap items-center gap-2 text-xs text-destructive">
            <span>产物加载失败：{friendlyErrorMessage(error)}</span>
            <Button size="sm" variant="outline" onClick={() => void refetch()}>重试</Button>
          </div>
        ) : null}
        {!isLoading && !error && filtered.length === 0 ? (
          <p className="text-xs text-muted-foreground">还没有组织产物。可以先让助手整理，或手动新建章节计划。</p>
        ) : null}
        {filtered.map((artifact) => {
          const isOpen = expanded === artifact.id
          const adopted = artifact.adoptedRevision != null && artifact.adopted?.status === 'adopted'
          const candidate = artifact.current?.status === 'candidate'
          const busy = pending !== null
          const rowAction = pending?.id === artifact.id ? pending.action : null
          return (
            <div key={artifact.id} className="rounded-xl border border-border/50">
              <button className="flex w-full items-center gap-2 px-3 py-2 text-left" onClick={() => setExpanded(isOpen ? null : artifact.id)} aria-expanded={isOpen}>
                {isOpen ? <ChevronDown className="size-3.5 shrink-0" /> : <ChevronRight className="size-3.5 shrink-0" />}
                <span className="min-w-0 flex-1 truncate text-xs font-medium">{artifactTitle(artifact)}</span>
                {artifact.needsRecheck ? (
                  <Badge variant="outline" className="shrink-0 border-amber-500/40 font-normal text-amber-700 dark:text-amber-400">待复查</Badge>
                ) : null}
                <Badge variant="outline" className="shrink-0 font-normal">{RESEARCH_KIND_LABEL[artifact.kind]}</Badge>
                <Badge variant={adopted ? 'default' : 'secondary'} className="shrink-0 font-normal">
                  {artifact.current ? RESEARCH_STATUS_LABEL[artifact.current.status] : '—'} v{artifact.currentRevision}
                </Badge>
              </button>
              {isOpen ? (
                <div className="space-y-2 border-t border-border/40 px-3 py-2">
                  {artifact.needsRecheck ? (
                    <div role="status" className="rounded-lg border border-amber-500/25 bg-amber-500/5 p-2 text-[11px] leading-5 text-amber-800 dark:text-amber-300">
                      <p className="font-medium">依据的资料已变化，请先在「待复查」里确认再采用</p>
                      <ul className="mt-1 list-disc space-y-0.5 pl-4">
                        {(artifact.recheckReasons ?? []).map((reason) => <li key={reason}>{reason}</li>)}
                      </ul>
                    </div>
                  ) : null}
                  {artifact.current ? <PayloadView payload={artifact.current.payload} kind={artifact.kind} /> : null}
                  <div className="flex flex-wrap items-center gap-2 pt-1">
                    {candidate ? (
                      <>
                        <Button
                          size="sm"
                          disabled={busy}
                          onClick={() => void runAction(artifact.id, 'adopt', '采用失败', '已采用该修订', () =>
                            adopt.mutateAsync({ artifactId: artifact.id, revision: artifact.currentRevision, expectedRevision: artifact.currentRevision }),
                          )}
                        >
                          {rowAction === 'adopt' ? <><Loader2 className="mr-1 size-3 animate-spin" />采用中…</> : '采用'}
                        </Button>
                        <Button
                          size="sm"
                          variant="outline"
                          disabled={busy}
                          onClick={() => void runAction(artifact.id, 'reject', '拒绝失败', '已拒绝该修订', () =>
                            reject.mutateAsync({ artifactId: artifact.id, revision: artifact.currentRevision, expectedRevision: artifact.currentRevision }),
                          )}
                        >
                          {rowAction === 'reject' ? <><Loader2 className="mr-1 size-3 animate-spin" />拒绝中…</> : '拒绝'}
                        </Button>
                      </>
                    ) : null}
                    {artifact.kind === 'chapter_plan' ? (
                      <Button size="sm" variant="ghost" onClick={() => setEditing(editing === artifact.id ? null : artifact.id)}>
                        {editing === artifact.id ? '收起编辑' : '编辑计划'}
                      </Button>
                    ) : null}
                    <Button
                      size="sm"
                      variant="ghost"
                      className="ml-auto text-destructive"
                      aria-label="删除产物"
                      disabled={busy}
                      onClick={() => {
                        if (!window.confirm('删除这个研究产物？删除后将不再出现在资料组织中。')) return
                        void runAction(artifact.id, 'delete', '删除失败', '已删除产物', () => remove.mutateAsync(artifact.id))
                      }}
                    >
                      {rowAction === 'delete' ? <Loader2 className="size-3.5 animate-spin" /> : <Trash2 className="size-3.5" />}
                    </Button>
                  </div>
                  {editing === artifact.id && artifact.kind === 'chapter_plan' ? (
                    <PlanEditor
                      artifact={artifact}
                      projectId={projectId}
                      busy={busy}
                      saving={rowAction === 'save'}
                      onSave={async (payload, expectedRevision) => {
                        const saved = await runAction(artifact.id, 'save', '保存修订失败', '已保存新修订', () =>
                          addRevision.mutateAsync({ artifactId: artifact.id, input: { expectedRevision, payload, origin: 'user' } }),
                        )
                        if (saved) setEditing(null)
                      }}
                    />
                  ) : null}
                  <p className="text-[10px] text-muted-foreground">当前 v{artifact.currentRevision}{artifact.adoptedRevision != null ? `，已采用 v${artifact.adoptedRevision}` : '，尚未采用'}</p>
                </div>
              ) : null}
            </div>
          )
        })}
      </CardContent>
    </Card>
  )
}

export default ResearchArtifactsPanel
