/**
 * AI 工作流模块（内容分类/逻辑链条/报告生成）的「建设中」页。
 * v0.5 路线功能；当前版本聚焦文献管理与论文撰写——给用户明确的
 * 状态反馈与去向引导，而不是死白页。
 */
import { useTranslation } from 'react-i18next'
import { useNavigate, useParams } from '@tanstack/react-router'
import { Construction, PenLine, FileUp, ArrowRight } from 'lucide-react'
import { Button } from '@/components/ui/button'

const TAB_KEY: Record<string, string> = {
  classifying: 'nav.tabClassifying',
  'logic-chain': 'nav.tabLogicChain',
  generating: 'nav.tabGenerating',
}

export function ComingSoon({ moduleKey }: { moduleKey: string }) {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { projectId = '' } = useParams({ strict: false })
  const name = t(TAB_KEY[moduleKey] ?? moduleKey)

  return (
    <div className="flex flex-1 items-center justify-center bg-muted/10 p-6">
      <div className="w-full max-w-md rounded-3xl border border-border/40 bg-card p-10 text-center shadow-sm">
        <div className="mx-auto mb-5 flex h-16 w-16 items-center justify-center rounded-2xl bg-primary/5 text-primary/70">
          <Construction className="h-8 w-8 stroke-[1.5]" />
        </div>
        <span className="mb-3 inline-block rounded-full bg-muted px-3 py-1 text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
          {t('comingSoon.badge')}
        </span>
        <h2 className="mb-2 text-lg font-semibold text-foreground">
          {t('comingSoon.title', { name })}
        </h2>
        <p className="mb-8 text-[13px] leading-relaxed text-muted-foreground">
          {t('comingSoon.desc')}
        </p>
        <div className="flex items-center justify-center gap-3">
          <Button
            variant="outline"
            size="sm"
            className="rounded-full text-xs"
            onClick={() => navigate({ to: '/project/$projectId/uploading', params: { projectId } })}
          >
            <FileUp className="mr-1.5 h-3.5 w-3.5" />
            {t('comingSoon.goUploading')}
          </Button>
          <Button
            size="sm"
            className="rounded-full text-xs"
            onClick={() => navigate({ to: '/project/$projectId/writing', params: { projectId } })}
          >
            <PenLine className="mr-1.5 h-3.5 w-3.5" />
            {t('comingSoon.goWriting')}
            <ArrowRight className="ml-1.5 h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </div>
  )
}
