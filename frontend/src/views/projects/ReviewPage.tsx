import { Link, useParams } from '@tanstack/react-router'
import { AlertTriangle, ArrowRight, CheckCircle2, CircleAlert, FileCheck2, LoaderCircle, RefreshCw } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { useLatexFileContent, useLatexFiles } from '@/hooks/use-latex'
import { evaluateReviewChecks } from './review-checks'

export function ReviewPage() {
  const { projectId = '' } = useParams({ strict: false })
  const { data: files = [], isPending: filesLoading, error: filesError, refetch } = useLatexFiles(projectId)
  const mainFile = files.find((file) => file.file_path === 'main.tex')
  const { data: mainContent, isPending: contentLoading, error: contentError } = useLatexFileContent(projectId, mainFile?.file_path ?? null)
  const checks = evaluateReviewChecks(files, mainContent?.content)
  const pending = filesLoading || (!!mainFile && contentLoading)
  const error = filesError || contentError
  const actionCount = checks.filter((check) => check.status === 'action').length
  const warningCount = checks.filter((check) => check.status === 'warning').length
  const passCount = checks.filter((check) => check.status === 'pass').length

  return (
    <div className="h-full overflow-y-auto overscroll-contain">
      <div className="mx-auto max-w-4xl space-y-6 p-4 sm:p-7 lg:p-9">
        <header className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
          <div className="min-w-0">
            <Badge variant="outline" className="mb-3 gap-1.5 rounded-full border-primary/20 bg-primary/5 px-3 py-1 text-primary">
              <FileCheck2 className="size-3.5" /> 项目文件检查
            </Badge>
            <h1 className="text-balance text-2xl font-semibold tracking-tight sm:text-3xl">提交前，把容易漏的地方过一遍</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
              检查当前 LaTeX 文件结构、模板占位文字、参考文献入口和章节文件引用。结果来自项目文件本身，不会替你判断研究结论或学术质量。
            </p>
          </div>
          <Button asChild className="min-h-11 w-full shrink-0 sm:w-auto">
            <Link to="/project/$projectId/writing" params={{ projectId }}>
              打开论文撰写 <ArrowRight className="ml-2 size-4" />
            </Link>
          </Button>
        </header>

        <Card className="rounded-2xl border-border/50 shadow-sm">
          <CardHeader className="pb-3">
            <CardTitle className="text-base">检查结果</CardTitle>
            <CardDescription>
              {pending ? '正在读取项目文件并检查…' : error ? '文件读取未完成，请重试后再查看结果。' : `已检查 ${checks.length} 项，发现 ${actionCount} 项待修复、${warningCount} 项建议确认。`}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {error ? (
              <div role="alert" className="flex flex-col gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-2 text-sm text-destructive">
                  <CircleAlert className="mt-0.5 size-4 shrink-0" />
                  <span className="break-words">{error.message}</span>
                </div>
                <Button type="button" variant="outline" className="min-h-11 shrink-0" onClick={() => void refetch()}>
                  <RefreshCw className="mr-2 size-4" />重试读取
                </Button>
              </div>
            ) : pending ? (
              <div className="flex min-h-32 items-center justify-center gap-2 text-sm text-muted-foreground">
                <LoaderCircle className="size-4 animate-spin" />正在检查文件…
              </div>
            ) : (
              checks.map((check) => {
                const Icon = check.status === 'pass' ? CheckCircle2 : check.status === 'warning' ? AlertTriangle : CircleAlert
                const tone = check.status === 'pass'
                  ? 'text-emerald-700 dark:text-emerald-400'
                  : check.status === 'warning'
                    ? 'text-amber-700 dark:text-amber-400'
                    : 'text-destructive'
                const label = check.status === 'pass' ? '已通过' : check.status === 'warning' ? '建议确认' : '需要处理'
                return (
                  <div key={check.id} className="flex min-w-0 flex-col gap-3 rounded-xl border border-border/50 bg-card p-4 sm:flex-row sm:items-start">
                    <Icon className={`mt-0.5 size-4 shrink-0 ${tone}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <h2 className="text-sm font-semibold">{check.title}</h2>
                        <span className={`text-[11px] font-medium ${tone}`}>{label}</span>
                      </div>
                      <p className="mt-1 break-words text-xs leading-5 text-muted-foreground">{check.detail}</p>
                    </div>
                    {check.status !== 'pass' && (
                      <Button asChild variant="ghost" className="min-h-10 shrink-0 justify-start px-2 sm:justify-center">
                        <Link to="/project/$projectId/writing" params={{ projectId }}>去修改 <ArrowRight className="ml-1 size-3.5" /></Link>
                      </Button>
                    )}
                  </div>
                )
              })
            )}
          </CardContent>
        </Card>

        {!pending && !error && (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-2 rounded-xl bg-muted/35 px-4 py-3 text-xs text-muted-foreground" aria-live="polite">
            <span className="font-medium text-foreground">{passCount} 项结构检查通过</span>
            <span>{warningCount} 项建议确认</span>
            <span>{actionCount} 项需要处理</span>
          </div>
        )}

        <p className="flex items-start gap-2 px-1 text-xs leading-5 text-muted-foreground">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-amber-700" />
          这页只做可重复的文件结构检查，不评估论证是否成立、引用是否真实或内容是否符合学校规范。完成结构检查后，请结合导师意见和原始文献进行人工核验。
        </p>
      </div>
    </div>
  )
}
