import { useEffect, useRef } from 'react'
import { Link, useParams } from '@tanstack/react-router'
import { ArrowUpRight, Check, FileText, Sparkles } from 'lucide-react'

import { AiPanel } from './ai-panel'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useDocuments } from '@/services/files-api'
import { useSaveWritingState, useWritingState } from '@/services/writing-state-api'
import type { WritingBrief } from '@/services/writing-state-api'
import { useSessionStore } from '@/store'

function briefSummary(brief: WritingBrief) {
  return [
    ['研究主题', brief.topic],
    ['研究背景', brief.background],
    ['核心问题', brief.questions.join('；')],
    ['研究目标', brief.goals.join('；')],
    ['研究对象与数据', brief.objects_and_data],
    ['研究方法', brief.methods],
    ['关键章节', brief.key_chapters.join('、')],
    ['范围边界', brief.scope_out.join('；')],
    ['时间节点', brief.milestones.join('；')],
    ['已知事实', brief.known_facts.join('；')],
    ['待验证假设', [...brief.assumptions, ...brief.unverified].join('；')],
  ].filter(([, value]) => value.trim())
}

export function PreparePage() {
  const { projectId = '' } = useParams({ strict: false })
  const isGuest = useSessionStore((session) => session.isGuest)
  const { data: state, isLoading, error } = useWritingState(projectId)
  const { data: documents = [] } = useDocuments(projectId, null)
  const save = useSaveWritingState(projectId).mutate
  const phaseSyncProject = useRef('')

  // Entering this route records the user's current phase without changing any preparation content.
  useEffect(() => {
    if (!state || phaseSyncProject.current === state.project_id) return
    phaseSyncProject.current = state.project_id
    if (state.current_phase === 'prepare') return
    save({
      brief: state.brief,
      requirements: state.requirements,
      phases: state.phases,
      current_phase: 'prepare',
      revision: state.revision,
    })
  }, [state, save])

  if (error) {
    return <div role="alert" className="m-6 rounded-2xl border border-destructive/20 bg-destructive/5 p-5 text-sm text-destructive">写作准备状态加载失败：{error.message}</div>
  }
  if (isLoading || !state) {
    return <div className="p-8 text-sm text-muted-foreground">正在加载研究计划…</div>
  }

  const confirmedDocuments = documents.filter((document) => document.status === 'uploaded')
  const latestDocumentDate = confirmedDocuments
    .map((document) => document.updatedAt || document.createdAt)
    .filter((date): date is string => Boolean(date))
    .sort((a, b) => b.localeCompare(a))[0]
  const missing = [
    !state.brief.topic.trim() && '找到一个值得研究的问题',
    state.brief.questions.length === 0 && '把兴趣收敛成可回答的研究问题',
    state.brief.goals.length === 0 && '明确预期成果',
    !state.brief.methods.trim() && '讨论可行的方法与数据',
    state.requirements.length === 0 && '整理导师或学校的要求',
    confirmedDocuments.length === 0 && '将任务书、论文或其他材料加入资料库',
  ].filter((item): item is string => Boolean(item))
  const summary = briefSummary(state.brief)

  return (
    <div className="grid h-full min-h-0 gap-4 overflow-y-auto p-3 sm:p-5 lg:grid-cols-[minmax(250px,0.78fr)_minmax(0,1.8fr)] lg:overflow-hidden">
      <aside className="order-2 min-w-0 space-y-4 lg:order-1 lg:overflow-y-auto lg:pr-1">
        <div className="px-1 py-1">
          <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-primary">写作准备</p>
          <h1 className="mt-2 text-2xl font-semibold tracking-tight">和研究顾问一起理清思路</h1>
          <p className="mt-2 text-sm leading-6 text-muted-foreground">直接说你的想法、困惑或手头材料。助手会循序追问、整理成研究蓝图；确认后才会保存。</p>
        </div>

        <Card className="rounded-2xl border-border/50 shadow-sm">
          <CardHeader className="px-4 pb-2 pt-4">
            <CardTitle className="flex items-center gap-2 text-sm"><Sparkles className="size-4 text-primary" />研究蓝图</CardTitle>
            <CardDescription>由你和助手的对话逐步形成</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 px-4 pb-4">
            {summary.length ? summary.map(([label, value]) => (
              <div key={label} className="border-b border-border/40 pb-2 last:border-0 last:pb-0">
                <p className="text-[11px] font-semibold text-muted-foreground">{label}</p>
                <p className="mt-1 whitespace-pre-wrap text-sm leading-5">{value}</p>
              </div>
            )) : (
              <p className="rounded-xl border border-dashed border-border px-3 py-4 text-sm leading-6 text-muted-foreground">
                还没有研究蓝图。先从你感兴趣的方向聊起，不需要逐项填写表格。
              </p>
            )}
            {state.requirements.length > 0 && (
              <div className="rounded-xl bg-muted/40 p-3">
                <p className="text-[11px] font-semibold text-muted-foreground">已整理的课题要求</p>
                <ul className="mt-2 space-y-2">
                  {state.requirements.map((requirement) => (
                    <li key={requirement.id} className="text-sm leading-5">
                      <span>{requirement.text}</span>
                      {requirement.source && <span className="ml-1 text-xs text-muted-foreground">· {requirement.source}</span>}
                    </li>
                  ))}
                </ul>
              </div>
            )}
          </CardContent>
        </Card>

        <Card className="rounded-2xl border-border/50 shadow-sm">
          <CardHeader className="px-4 pb-2 pt-4">
            <div className="flex items-center justify-between gap-2">
              <CardTitle className="text-sm">接下来可以聊</CardTitle>
              <Badge variant="outline" className="font-normal">{missing.length ? `${missing.length} 个待探索方向` : '蓝图已较完整'}</Badge>
            </div>
          </CardHeader>
          <CardContent className="px-4 pb-4">
            {missing.length ? (
              <ul className="space-y-2">
                {missing.map((item) => <li key={item} className="flex gap-2 text-xs leading-5 text-muted-foreground"><span className="mt-1 size-1.5 shrink-0 rounded-full bg-amber-500" />{item}</li>)}
              </ul>
            ) : <p className="flex items-center gap-2 text-xs text-emerald-700"><Check className="size-3.5" />目前没有明显缺项，可以继续完善或进入撰写。</p>}
          </CardContent>
        </Card>

        <Card className="rounded-2xl border-border/50 shadow-sm">
          <CardContent className="flex items-center justify-between gap-3 p-4">
            <div className="flex min-w-0 items-center gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary/5 text-primary"><FileText className="size-4" /></div>
              <div className="min-w-0"><p className="text-sm font-semibold">{confirmedDocuments.length} 份已确认材料</p><p className="mt-1 text-xs text-muted-foreground">最近更新：{latestDocumentDate ? new Date(latestDocumentDate).toLocaleDateString('zh-CN') : '暂无材料'}</p></div>
            </div>
            <Button asChild variant="ghost" size="icon-sm" className="shrink-0" aria-label="进入资料库"><Link to="/project/$projectId/uploading" params={{ projectId }}><ArrowUpRight className="size-4" /></Link></Button>
          </CardContent>
        </Card>
      </aside>

      <section aria-label="研究准备对话" className="order-1 flex h-[min(72dvh,720px)] min-h-[480px] min-w-0 flex-col overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm lg:order-2 lg:h-full lg:min-h-0">
        {isGuest ? (
          <div className="flex flex-1 flex-col items-center justify-center px-6 text-center">
            <Sparkles className="size-8 text-primary" />
            <h2 className="mt-4 text-lg font-semibold">研究顾问需要登录后使用</h2>
            <p className="mt-2 max-w-sm text-sm leading-6 text-muted-foreground">游客项目和当前研究蓝图保存在此设备。登录账号并配置 AI 模型后，可以继续用对话整理研究计划。</p>
          </div>
        ) : <AiPanel projectId={projectId} mode="preparation" />}
      </section>
    </div>
  )
}
