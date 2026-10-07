import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { AlertTriangle, Check, ChevronRight, CirclePlus, FileText, RefreshCw, Save, Trash2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { useDocuments } from '@/services/files-api'
import { getConflictState, useSaveWritingState, useWritingState, type WritingBrief, type WritingRequirement, type WritingState } from '@/services/writing-state-api'
import { useSessionStore } from '@/store'

function textToLines(value: string) {
  return value.split('\n').map((line) => line.trim()).filter(Boolean)
}

function Field({ label, value, onChange, placeholder, multiline = false }: {
  label: string
  value: string
  onChange: (value: string) => void
  placeholder?: string
  multiline?: boolean
}) {
  return (
    <label className="block space-y-2">
      <span className="text-xs font-semibold">{label}</span>
      {multiline ? (
        <Textarea value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} className="min-h-24 resize-y bg-background" />
      ) : (
        <Input value={value} onChange={(event) => onChange(event.target.value)} placeholder={placeholder} className="h-11 bg-background" />
      )}
    </label>
  )
}

function ListField({ label, values, onChange, placeholder }: {
  label: string
  values: string[]
  onChange: (values: string[]) => void
  placeholder?: string
}) {
  return (
    <Field
      label={label}
      value={values.join('\n')}
      onChange={(value) => onChange(textToLines(value))}
      placeholder={placeholder ?? '每行填写一项'}
      multiline
    />
  )
}

function emptyRequirement(): WritingRequirement {
  const id = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `req-${Date.now()}-${Math.random().toString(36).slice(2)}`
  return { id, text: '', source: '', note: '' }
}

export function PreparePage() {
  const { projectId = '' } = useParams({ strict: false })
  const isGuest = useSessionStore((session) => session.isGuest)
  const { data: remote, isLoading, error: loadError } = useWritingState(projectId)
  const { data: documents = [] } = useDocuments(projectId, null)
  const saveMutation = useSaveWritingState(projectId)
  const save = saveMutation.mutate
  const [draft, setDraft] = useState<WritingState | null>(null)
  const [dirty, setDirty] = useState(false)
  const [conflict, setConflict] = useState<WritingState | null>(null)
  const [saveError, setSaveError] = useState('')
  const editVersion = useRef(0)
  const phaseSyncProject = useRef('')

  useEffect(() => {
    if (!remote || phaseSyncProject.current === remote.project_id) return
    phaseSyncProject.current = remote.project_id
    if (remote.current_phase === 'prepare') return
    save({
      brief: remote.brief,
      requirements: remote.requirements,
      phases: remote.phases,
      current_phase: 'prepare',
      revision: remote.revision,
    }, {
      onSuccess: (saved) => {
        setDraft((current) => current ? { ...current, revision: saved.revision, current_phase: 'prepare' } : current)
      },
      onError: (error) => setSaveError(error.message || '阶段状态同步失败。'),
    })
  }, [remote, save])

  useEffect(() => {
    if (!draft || !dirty || conflict) return
    const version = editVersion.current
    const timer = window.setTimeout(() => {
      setSaveError('')
      save({
        brief: draft.brief,
        requirements: draft.requirements,
        phases: draft.phases,
        current_phase: 'prepare',
        revision: draft.revision,
      }, {
        onSuccess: (saved) => {
          setDraft((current) => current ? { ...current, revision: saved.revision, current_phase: 'prepare' } : saved)
          if (editVersion.current === version) setDirty(false)
        },
        onError: (error) => {
          const latest = getConflictState(error)
          if (latest) setConflict(latest)
          else setSaveError(error.message || '保存失败，请重试。')
        },
      })
    }, 800)
    return () => window.clearTimeout(timer)
  }, [draft, dirty, conflict, save])

  const updateDraft = (updater: (current: WritingState) => WritingState) => {
    editVersion.current += 1
    setDraft((current) => current ? updater(current) : remote ? updater(remote) : current)
    setDirty(true)
  }

  if (loadError) {
    return <div className="m-8 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">写作准备状态加载失败：{loadError.message}</div>
  }
  const activeDraft = draft ?? remote
  if (isLoading || !activeDraft) {
    return <div className="p-8 text-sm text-muted-foreground">正在加载写作准备状态…</div>
  }

  const brief = activeDraft.brief
  const requirements = activeDraft.requirements
  const confirmedDocuments = documents.filter((doc) => doc.status === 'uploaded')
  const latestDocumentDate = confirmedDocuments
    .map((doc) => doc.updatedAt || doc.createdAt)
    .filter((date): date is string => Boolean(date))
    .sort((a, b) => b.localeCompare(a))[0]
  const missing = [
    !brief.topic.trim() && '补充研究主题',
    brief.questions.length === 0 && '明确核心研究问题',
    brief.goals.length === 0 && '写下预期目标',
    !brief.methods.trim() && '选择研究方法',
    requirements.length === 0 && '添加课题或导师要求',
    confirmedDocuments.length === 0 && '将已确认材料加入项目资料库',
  ].filter((item): item is string => Boolean(item))

  const changeBrief = (key: keyof WritingBrief, value: string | string[]) => {
    updateDraft((current) => ({ ...current, brief: { ...current.brief, [key]: value } }))
  }

  const loadLatest = () => {
    if (!conflict) return
    setDraft(conflict)
    setConflict(null)
    setDirty(false)
    setSaveError('')
  }

  const overwriteLatest = () => {
    if (!conflict || !draft) return
    setDraft({ ...draft, revision: conflict.revision })
    setConflict(null)
    setDirty(true)
    setSaveError('')
  }

  const retrySave = () => {
    if (!draft) return
    setDirty(true)
    setSaveError('')
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto grid max-w-7xl gap-6 p-4 sm:p-7 lg:grid-cols-[250px_minmax(0,1fr)] lg:gap-8">
        <aside className="space-y-5 lg:sticky lg:top-6 lg:h-fit">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-primary">写作准备</p>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight">先把研究说清楚</h1>
            <p className="mt-2 text-sm leading-6 text-muted-foreground">整理研究蓝图和课题要求。以下提示只帮助你发现空缺，不会阻止进入其他阶段。</p>
          </div>
          <Card className="rounded-2xl border border-border/40 bg-card shadow-sm">
            <CardHeader className="px-4 pb-0 pt-4">
              <CardTitle className="text-xs">准备清单</CardTitle>
            </CardHeader>
            <CardContent className="space-y-1 p-3">
              {[
                ['研究蓝图', 'brief'],
                ['课题要求', 'requirements'],
                ['材料概览', 'materials'],
              ].map(([label, id]) => (
                <a key={id} href={`#${id}`} className="flex items-center justify-between rounded-lg px-2 py-2 text-xs text-muted-foreground hover:bg-muted hover:text-foreground">
                  {label}<ChevronRight className="size-3.5" />
                </a>
              ))}
            </CardContent>
          </Card>
          <Card className="rounded-2xl border border-border/40 bg-card shadow-sm">
            <CardHeader className="px-4 pb-0 pt-4">
              <CardTitle className="text-xs">待补项</CardTitle>
              <CardDescription>根据当前蓝图和已确认材料即时整理</CardDescription>
            </CardHeader>
            <CardContent className="p-4 pt-3">
              {missing.length ? (
                <ul className="space-y-2">
                  {missing.map((item) => <li key={item} className="flex gap-2 text-xs leading-5 text-muted-foreground"><span className="mt-1 size-1.5 shrink-0 rounded-full bg-amber-500" />{item}</li>)}
                </ul>
              ) : <p className="flex items-center gap-2 text-xs text-emerald-700"><Check className="size-3.5" />目前没有明显待补项</p>}
            </CardContent>
          </Card>
        </aside>

        <div className="min-w-0 space-y-5">
          {conflict && (
            <div role="alert" className="flex flex-col gap-3 rounded-2xl border border-amber-500/30 bg-amber-500/10 p-4 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-start gap-2 text-sm"><AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-700" /><div><p className="font-semibold">内容已在其他位置更新</p><p className="mt-1 text-xs text-muted-foreground">你的本地输入仍保留。可以加载服务端版本，或以最新修订号覆盖保存。</p></div></div>
              <div className="flex gap-2 sm:shrink-0"><Button variant="outline" size="sm" onClick={loadLatest}>加载最新版本</Button><Button size="sm" onClick={overwriteLatest}>基于最新版本覆盖保存</Button></div>
            </div>
          )}

          <Card id="brief" className="scroll-mt-6 rounded-2xl border border-border/40 bg-card shadow-sm">
            <CardHeader className="border-b border-border/30 px-5 py-4 sm:px-6">
              <div className="flex items-start justify-between gap-3">
                <div><p className="mb-1 text-[10px] font-bold uppercase tracking-[0.18em] text-muted-foreground">01 · RESEARCH BLUEPRINT</p><CardTitle className="text-base">研究蓝图</CardTitle><CardDescription className="mt-1">把已知事实、待验证假设和研究边界分开记录。</CardDescription></div>
                <Badge variant="outline" className="shrink-0">修订 {activeDraft.revision}</Badge>
              </div>
            </CardHeader>
            <CardContent className="grid gap-5 p-5 sm:grid-cols-2 sm:p-6">
              <Field label="研究主题" value={brief.topic} onChange={(value) => changeBrief('topic', value)} placeholder="用一句话概括研究对象与问题" />
              <Field label="研究背景" value={brief.background} onChange={(value) => changeBrief('background', value)} placeholder="为什么这个问题值得研究？" multiline />
              <ListField label="核心研究问题" values={brief.questions} onChange={(value) => changeBrief('questions', value)} />
              <ListField label="研究目标与预期成果" values={brief.goals} onChange={(value) => changeBrief('goals', value)} />
              <Field label="研究对象与数据 / 实验条件" value={brief.objects_and_data} onChange={(value) => changeBrief('objects_and_data', value)} placeholder="对象、数据来源、设备或时间限制" multiline />
              <Field label="方法与技术路线" value={brief.methods} onChange={(value) => changeBrief('methods', value)} placeholder="拟采用的方法及主要步骤" multiline />
              <ListField label="研究范围之外" values={brief.scope_out} onChange={(value) => changeBrief('scope_out', value)} />
              <ListField label="关键章节" values={brief.key_chapters} onChange={(value) => changeBrief('key_chapters', value)} />
              <ListField label="时间节点" values={brief.milestones} onChange={(value) => changeBrief('milestones', value)} />
              <ListField label="学校 / 导师约束" values={brief.constraints} onChange={(value) => changeBrief('constraints', value)} />
              <ListField label="已知事实" values={brief.known_facts} onChange={(value) => changeBrief('known_facts', value)} />
              <ListField label="待验证假设" values={brief.assumptions} onChange={(value) => changeBrief('assumptions', value)} />
              <ListField label="尚未证实的事项" values={brief.unverified} onChange={(value) => changeBrief('unverified', value)} />
            </CardContent>
          </Card>

          <Card id="requirements" className="scroll-mt-6 rounded-2xl border border-border/40 bg-card shadow-sm">
            <CardHeader className="border-b border-border/30 px-5 py-4 sm:px-6">
              <div className="flex items-center justify-between gap-3"><div><p className="mb-1 text-[10px] font-bold uppercase tracking-[0.18em] text-muted-foreground">02 · REQUIREMENTS</p><CardTitle className="text-base">课题要求</CardTitle><CardDescription className="mt-1">记录要求内容、来源和适用说明，避免遗漏导师或学校约束。</CardDescription></div><Button variant="outline" size="sm" onClick={() => updateDraft((current) => ({ ...current, requirements: [...current.requirements, emptyRequirement()] }))}><CirclePlus className="size-3.5" />添加条目</Button></div>
            </CardHeader>
            <CardContent className="space-y-3 p-5 sm:p-6">
              {requirements.length === 0 && <p className="rounded-xl border border-dashed border-border p-5 text-center text-xs text-muted-foreground">还没有要求条目。可以从任务书、导师意见或学院规范开始整理。</p>}
              {requirements.map((requirement, index) => (
                <div key={requirement.id} className="grid gap-3 rounded-xl border border-border/50 bg-muted/10 p-3 sm:grid-cols-[minmax(0,1.5fr)_minmax(160px,0.8fr)_minmax(0,1fr)_auto]">
                  <Input aria-label={`要求内容 ${index + 1}`} placeholder="要求内容" value={requirement.text} onChange={(event) => updateDraft((current) => ({ ...current, requirements: current.requirements.map((row) => row.id === requirement.id ? { ...row, text: event.target.value } : row) }))} />
                  <Input aria-label={`要求来源 ${index + 1}`} placeholder="来源" value={requirement.source} onChange={(event) => updateDraft((current) => ({ ...current, requirements: current.requirements.map((row) => row.id === requirement.id ? { ...row, source: event.target.value } : row) }))} />
                  <Input aria-label={`适用说明 ${index + 1}`} placeholder="适用说明" value={requirement.note} onChange={(event) => updateDraft((current) => ({ ...current, requirements: current.requirements.map((row) => row.id === requirement.id ? { ...row, note: event.target.value } : row) }))} />
                  <Button variant="ghost" size="icon-sm" aria-label={`删除要求 ${index + 1}`} onClick={() => updateDraft((current) => ({ ...current, requirements: current.requirements.filter((row) => row.id !== requirement.id) }))}><Trash2 className="size-4 text-muted-foreground" /></Button>
                </div>
              ))}
            </CardContent>
          </Card>

          <Card id="materials" className="scroll-mt-6 rounded-2xl border border-border/40 bg-card shadow-sm">
            <CardHeader className="border-b border-border/30 px-5 py-4 sm:px-6">
              <p className="mb-1 text-[10px] font-bold uppercase tracking-[0.18em] text-muted-foreground">03 · MATERIALS</p><CardTitle className="text-base">项目材料</CardTitle><CardDescription>只读汇总已确认入库材料；在资料库管理上传与确认。</CardDescription>
            </CardHeader>
            <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6">
              <div className="flex items-center gap-3"><div className="flex size-10 items-center justify-center rounded-xl bg-primary/5 text-primary"><FileText className="size-4" /></div><div><p className="text-sm font-semibold">{confirmedDocuments.length} 份已确认材料</p><p className="mt-1 text-xs text-muted-foreground">最近更新：{latestDocumentDate ? new Date(latestDocumentDate).toLocaleDateString('zh-CN') : '暂无材料'}</p></div></div>
              <Button asChild variant="outline" size="sm" className="rounded-full"><Link to="/project/$projectId/uploading" params={{ projectId }}>进入资料库<ChevronRight className="size-3.5" /></Link></Button>
            </CardContent>
          </Card>

          <div className="flex min-h-10 items-center justify-between px-1 text-xs" aria-live="polite">
            <span className={saveError ? 'text-destructive' : 'text-muted-foreground'}>{saveError || (saveMutation.isPending || dirty ? isGuest ? '正在保存在此设备…' : '保存中…' : isGuest ? '已保存在此设备' : '所有更改已保存')}</span>
            {saveError && <Button variant="ghost" size="sm" onClick={retrySave}><RefreshCw className="size-3.5" />重试</Button>}
            {!saveError && !saveMutation.isPending && !dirty && <span className="flex items-center gap-1 text-emerald-700"><Check className="size-3.5" />修订 {activeDraft.revision}</span>}
            {saveMutation.isPending && <Save className="size-3.5 animate-pulse text-muted-foreground" />}
          </div>
        </div>
      </div>
    </div>
  )
}
