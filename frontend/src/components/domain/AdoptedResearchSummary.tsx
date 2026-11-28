import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { ChevronDown, ChevronRight, ClipboardCheck } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { useResearchArtifacts } from '@/services/research-api'
import type { ChapterDigestPayload, ChapterPlanPayload, ResearchArtifact } from '@/types/research'

function adoptedBody(artifact: ResearchArtifact) {
  const payload = artifact.adopted?.payload
  if (!payload) return null
  if (artifact.kind === 'chapter_plan') {
    const plan = payload as unknown as ChapterPlanPayload
    return (
      <ul className="space-y-1 text-[11px] leading-5">
        {plan.sections.map((section) => (
          <li key={section.id}>
            <span className="font-medium">{section.order}. {section.title}</span>
            <span className="text-muted-foreground">：{section.responsibility}</span>
            {section.filePath ? <span className="text-muted-foreground">（{section.filePath}）</span> : null}
          </li>
        ))}
      </ul>
    )
  }
  if (artifact.kind === 'chapter_digest') {
    const digest = payload as unknown as ChapterDigestPayload
    return <p className="line-clamp-3 text-[11px] leading-5 text-muted-foreground">{digest.content}</p>
  }
  return null
}

/** 撰写侧栏：只展示用户已采用的章节计划/摘要，不把较新候选冒充正式计划。 */
export function AdoptedResearchSummary({ projectId }: { projectId: string }) {
  const { data: artifacts = [] } = useResearchArtifacts(projectId)
  const [open, setOpen] = useState(true)
  const adopted = artifacts.filter((artifact) =>
    artifact.adoptedRevision != null &&
    (artifact.kind === 'chapter_plan' || artifact.kind === 'chapter_digest'),
  )
  if (adopted.length === 0) return null
  return (
    <div className="flex-none border-b border-border/40 bg-muted/10">
      <button className="flex w-full items-center gap-2 px-4 py-2 text-left" onClick={() => setOpen((prev) => !prev)} aria-expanded={open}>
        {open ? <ChevronDown className="size-3.5" /> : <ChevronRight className="size-3.5" />}
        <ClipboardCheck className="size-3.5 text-primary" />
        <span className="text-xs font-semibold">已采用的写作计划</span>
        <span className="ml-auto text-[10px] text-muted-foreground">{adopted.length} 项</span>
      </button>
      {open ? (
        <div className="max-h-48 space-y-2 overflow-y-auto px-4 pb-3">
          {adopted.map((artifact) => (
            <div key={artifact.id}>
              <p className="flex flex-wrap items-center gap-1 text-[10px] font-medium text-muted-foreground">
                {artifact.kind === 'chapter_plan' ? '章节计划' : '章节摘要'} · 已采用
                {artifact.needsRecheck ? (
                  <Badge variant="outline" className="border-amber-500/40 font-normal text-amber-700 dark:text-amber-400">待复查</Badge>
                ) : null}
              </p>
              {adoptedBody(artifact)}
              {artifact.needsRecheck ? (
                <div role="status" className="mt-1 rounded-lg border border-amber-500/25 bg-amber-500/5 p-2 text-[11px] leading-5 text-amber-800 dark:text-amber-300">
                  <p>所依据的资料发生变化，请确认原计划/摘要是否仍成立。</p>
                  {(artifact.recheckReasons ?? []).length > 0 ? (
                    <ul className="mt-1 list-disc space-y-0.5 pl-4">
                      {(artifact.recheckReasons ?? []).map((reason) => <li key={reason}>{reason}</li>)}
                    </ul>
                  ) : null}
                  <Link
                    to="/project/$projectId/prepare"
                    params={{ projectId }}
                    className="mt-1 inline-flex min-h-[44px] items-center font-medium text-primary underline"
                  >
                    去准备页处理
                  </Link>
                </div>
              ) : null}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  )
}
