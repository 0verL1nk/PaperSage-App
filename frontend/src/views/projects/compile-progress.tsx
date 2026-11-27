import { useEffect, useState } from 'react'
import { Check, ExternalLink, Loader2, Square } from 'lucide-react'

import { StateBlock } from '@/components/shared'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { BrowserEngineLicense, CompileStage, ProjectCompileState } from '@/store/compile-store'

/** 粗阶段：细粒度引擎事件映射到这里，避免资源事件让高亮来回跳。 */
const STEPS: ReadonlyArray<{ label: string; stages: readonly CompileStage[] }> = [
  { label: '准备项目文件', stages: ['prepare'] },
  { label: '启动引擎并准备格式', stages: ['engine', 'format'] },
  { label: '下载宏包与字体', stages: ['resource'] },
  { label: '排版并生成 PDF', stages: ['compile', 'convert'] },
  { label: '保存编译结果', stages: ['upload'] },
]

function stepIndex(stage: CompileStage): number {
  const index = STEPS.findIndex((step) => step.stages.includes(stage))
  return index === -1 ? 0 : index
}

function formatElapsed(totalSeconds: number): string {
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  const paddedSeconds = String(seconds).padStart(2, '0')
  return hours > 0
    ? `${hours}:${String(minutes).padStart(2, '0')}:${paddedSeconds}`
    : `${minutes}:${paddedSeconds}`
}

/** 引擎许可证链接：进度视图与上传横幅共用，满足第三方许可展示要求。 */
export function EngineLicenseLinks({ license }: { license: BrowserEngineLicense | null }) {
  if (!license) return null
  return (
    <div className="mt-4 flex w-full flex-wrap items-center justify-center gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
      <span>{license.name} · {license.spdx}</span>
      <a className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-foreground" href={license.licenseUrl} target="_blank" rel="noreferrer">
        引擎许可证 <ExternalLink size={11} />
      </a>
      <a className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-foreground" href={license.sourceUrl} target="_blank" rel="noreferrer">
        上游源码 <ExternalLink size={11} />
      </a>
      <a className="inline-flex items-center gap-1 underline underline-offset-2 hover:text-foreground" href={license.thirdPartyNoticesUrl} target="_blank" rel="noreferrer">
        第三方许可 <ExternalLink size={11} />
      </a>
    </div>
  )
}

interface CompileProgressProps {
  state: ProjectCompileState
  /** 预览区已有旧版 PDF 时，明确告知会被本次结果替换。 */
  hasPreviousPdf: boolean
  onCancel: () => void
}

/** 编译中的预览区：阶段进度 + 实时耗时 + 资源计数 + 取消入口。 */
export function CompileProgress({ state, hasPreviousPdf, onCancel }: CompileProgressProps) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const elapsedSeconds = Math.max(0, Math.floor((now - state.startedAt) / 1000))
  const activeStep = stepIndex(state.stage)
  const resources = Object.values(state.resources)
  const loadedCount = resources.filter((resource) => resource.state === 'loaded' || resource.state === 'cacheWarning').length
  const cachedCount = resources.filter((resource) => resource.state === 'loaded' && resource.cached).length
  const loadingNames = resources.filter((resource) => resource.state === 'loading').map((resource) => resource.name)

  return (
    <StateBlock
      title="正在编译论文"
      description={state.progressMessage ?? '正在准备编译环境…'}
      icon={Loader2}
      className="mx-auto min-h-[480px] w-full max-w-[460px] py-10"
      action={state.phase === 'uploading' ? undefined : (
        <Button
          variant="outline"
          size="sm"
          data-testid="preview-cancel-compile"
          className="h-[44px] gap-2 px-5 text-sm"
          onClick={onCancel}
        >
          <Square size={13} className="fill-current" />
          取消编译
        </Button>
      )}
    >
      <p className="mt-1 text-xs text-muted-foreground" role="timer">
        已用时 <span className="font-medium tabular-nums text-foreground">{formatElapsed(elapsedSeconds)}</span>
      </p>

      {state.formatCacheHit === false && (
        <p className="mt-4 w-full rounded-md border border-amber-500/25 bg-amber-500/5 px-3 py-2 text-left text-xs leading-5 text-amber-700 dark:text-amber-400">
          首次冷编译：正在按需下载 LaTeX 格式文件与所需宏包、字体，通常需要 1–3 分钟；完成后再编译通常几秒即可。
        </p>
      )}
      {state.formatCacheHit === true && (
        <p className="mt-4 w-full rounded-md border border-emerald-500/25 bg-emerald-500/5 px-3 py-2 text-left text-xs leading-5 text-emerald-700 dark:text-emerald-400">
          已命中浏览器格式缓存，启动会更快；本次新用到的宏包与字体仍会按需下载。
        </p>
      )}

      <ol className="mt-5 w-full space-y-2 text-left" aria-label="编译阶段">
        {STEPS.map((step, index) => {
          const isDone = index < activeStep
          const isCurrent = index === activeStep
          return (
            <li key={step.label} className="flex items-center gap-2.5 text-xs">
              <span
                className={cn(
                  'flex size-5 shrink-0 items-center justify-center rounded-full border text-[10px]',
                  isDone ? 'border-emerald-500/40 bg-emerald-500/10 text-emerald-600'
                    : isCurrent ? 'border-primary/50 bg-primary/10 text-primary'
                      : 'border-border text-muted-foreground',
                )}
              >
                {isDone ? <Check size={11} /> : isCurrent ? <Loader2 size={11} className="animate-spin" /> : index + 1}
              </span>
              <span className={cn(isCurrent ? 'font-medium text-foreground' : isDone ? 'text-muted-foreground' : 'text-muted-foreground/60')}>
                {step.label}
              </span>
              {isCurrent && <span className="ml-auto text-[10px] text-muted-foreground">进行中</span>}
            </li>
          )
        })}
      </ol>

      {resources.length > 0 && (
        <div className="mt-4 w-full rounded-md bg-muted/40 px-3 py-2 text-left text-[11px] text-muted-foreground">
          <p>已加载 {loadedCount} 项宏包/字体资源{cachedCount > 0 ? `，其中 ${cachedCount} 项来自缓存` : ''}</p>
          {loadingNames.length > 0 && (
            <p className="mt-1 truncate">
              正在下载：{loadingNames.slice(0, 3).join('、')}
              {loadingNames.length > 3 ? ` 等 ${loadingNames.length} 项` : ''}
            </p>
          )}
        </div>
      )}

      {hasPreviousPdf && (
        <p className="mt-4 text-[11px] text-muted-foreground">编译完成后会自动替换当前预览。</p>
      )}

      <EngineLicenseLinks license={state.license} />

      <p className="sr-only" role="status" aria-live="polite">当前阶段：{STEPS[activeStep].label}</p>
    </StateBlock>
  )
}
