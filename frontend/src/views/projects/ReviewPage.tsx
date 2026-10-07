import { useEffect, useRef, useState } from 'react'
import { useParams } from '@tanstack/react-router'
import { AlertTriangle, Check, Circle, LoaderCircle, Save } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useSaveWritingState, useWritingState, type PhaseStatus, type WritingPhase } from '@/services/writing-state-api'

const phases: { id: WritingPhase; title: string; description: string }[] = [
  { id: 'prepare', title: '写作准备', description: '研究问题、材料与课题要求是否已整理。' },
  { id: 'write', title: '论文撰写', description: '正文内容与 LaTeX 文件是否已完成并保存。' },
  { id: 'review', title: '检查完善', description: '跨章节逻辑、论据与格式是否已由你检查。' },
]

const statusOptions: { value: PhaseStatus; label: string }[] = [
  { value: 'not_started', label: '未开始' },
  { value: 'in_progress', label: '进行中' },
  { value: 'done', label: '已完成' },
]

function nextStatus(value: PhaseStatus): PhaseStatus {
  const index = statusOptions.findIndex((status) => status.value === value)
  return statusOptions[(index + 1) % statusOptions.length].value
}

export function ReviewPage() {
  const { projectId = '' } = useParams({ strict: false })
  const { data: state, isLoading, error } = useWritingState(projectId)
  const saveMutation = useSaveWritingState(projectId)
  const [saveError, setSaveError] = useState('')
  const [savedRevision, setSavedRevision] = useState<number | null>(null)
  const phaseSyncProject = useRef('')

  useEffect(() => {
    if (!state || phaseSyncProject.current === state.project_id) return
    phaseSyncProject.current = state.project_id
    if (state.current_phase === 'review') return
    saveMutation.mutate({
      brief: state.brief,
      requirements: state.requirements,
      phases: state.phases,
      current_phase: 'review',
      revision: state.revision,
    }, {
      onSuccess: (saved) => setSavedRevision(saved.revision),
      onError: (failure) => setSaveError(failure.message || '阶段状态同步失败。'),
    })
  }, [state, saveMutation])

  const updatePhase = (phase: WritingPhase) => {
    if (!state) return
    setSaveError('')
    saveMutation.mutate({
      brief: state.brief,
      requirements: state.requirements,
      phases: { ...state.phases, [phase]: nextStatus(state.phases[phase]) },
      current_phase: 'review',
      revision: state.revision,
    }, {
      onSuccess: (saved) => setSavedRevision(saved.revision),
      onError: (failure) => setSaveError(failure.message || '阶段状态保存失败。'),
    })
  }

  if (error) return <div className="m-4 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive sm:m-8">检查完善状态加载失败：{error.message}</div>
  if (isLoading || !state) return <div className="p-8 text-sm text-muted-foreground">正在加载阶段状态…</div>

  return (
    <div className="h-full overflow-y-auto overscroll-contain">
      <div className="mx-auto grid max-w-6xl gap-6 p-4 sm:p-7 lg:grid-cols-[minmax(0,1.4fr)_minmax(260px,0.8fr)] lg:gap-8">
        <section className="min-w-0 space-y-5">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-primary">检查与完善</p>
            <h1 className="mt-2 text-2xl font-semibold tracking-tight">完成情况由你确认</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">这里记录各阶段的人工进度。PaperSage 当前不会自动判定论文质量，也不会生成未经核验的检查结果。</p>
          </div>

          <div className="space-y-3">
            {phases.map((phase, index) => {
              const status = state.phases[phase.id]
              return (
                <Card key={phase.id} className="min-w-0 rounded-2xl border border-border/40 bg-card shadow-sm">
                  <CardContent className="flex min-w-0 flex-col gap-4 p-4 sm:flex-row sm:items-center sm:justify-between sm:p-5">
                    <div className="flex min-w-0 items-start gap-3">
                      <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-xl bg-primary/5 text-primary">
                        {status === 'done' ? <Check className="size-4" /> : status === 'in_progress' ? <LoaderCircle className="size-4" /> : <Circle className="size-3.5" />}
                      </div>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <h2 className="text-sm font-semibold">{String(index + 1).padStart(2, '0')} · {phase.title}</h2>
                          <Badge variant="outline" className="whitespace-nowrap">{statusOptions.find((item) => item.value === status)?.label}</Badge>
                        </div>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">{phase.description}</p>
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      className="min-h-11 w-full shrink-0 sm:min-h-9 sm:w-auto"
                      disabled={saveMutation.isPending}
                      onClick={() => updatePhase(phase.id)}
                      aria-label={`${phase.title}状态：${statusOptions.find((item) => item.value === status)?.label}，点击切换`}
                    >
                      更新状态
                    </Button>
                  </CardContent>
                </Card>
              )
            })}
          </div>
          <p className="flex items-start gap-2 rounded-xl border border-amber-500/20 bg-amber-500/5 p-3 text-xs leading-5 text-muted-foreground"><AlertTriangle className="mt-0.5 size-4 shrink-0 text-amber-700" />阶段状态仅用于个人进度记录。学术论证、引文可靠性和格式规范仍需结合原始材料人工核验。</p>
        </section>

        <aside className="min-w-0 space-y-4">
          <Card className="rounded-2xl border border-border/40 bg-card shadow-sm">
            <CardHeader className="px-5 pb-0 pt-5">
              <CardTitle className="text-sm">当前研究蓝图</CardTitle>
              <CardDescription>检查阶段可回看准备内容；这里不会改写你的记录。</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4 p-5">
              <div className="min-w-0"><p className="text-[11px] font-semibold text-muted-foreground">研究主题</p><p className="mt-1 break-words text-sm">{state.brief.topic || '尚未填写'}</p></div>
              <div className="min-w-0"><p className="text-[11px] font-semibold text-muted-foreground">核心研究问题</p>{state.brief.questions.length ? <ul className="mt-1 list-inside list-disc space-y-1 text-sm">{state.brief.questions.map((question, index) => <li key={`${index}-${question}`} className="break-words">{question}</li>)}</ul> : <p className="mt-1 text-sm text-muted-foreground">尚未填写</p>}</div>
              <div className="flex flex-wrap gap-2 border-t border-border/40 pt-3 text-xs text-muted-foreground"><span>{state.requirements.length} 项课题要求</span><span aria-hidden="true">·</span><span>修订 {savedRevision ?? state.revision}</span></div>
            </CardContent>
          </Card>
          <div aria-live="polite" className="flex min-h-10 items-center gap-2 px-1 text-xs">
            {saveError ? <span role="alert" className="text-destructive">{saveError}</span> : saveMutation.isPending ? <><Save className="size-3.5 animate-pulse" /><span className="text-muted-foreground">正在保存阶段状态…</span></> : <><Check className="size-3.5 text-emerald-700" /><span className="text-muted-foreground">状态已同步</span></>}
          </div>
        </aside>
      </div>
    </div>
  )
}
