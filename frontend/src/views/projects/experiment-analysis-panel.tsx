import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { AlertTriangle, Loader2, Sparkles, Square } from 'lucide-react'
import { toast } from 'sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { escapeLatex } from '@/lib/experiment-table/booktabs'
import type { BoldMode, ExperimentTable } from '@/lib/experiment-table/types'
import { friendlyErrorMessage } from '@/services/errors'
import { generateExperimentAnalysis, type ExperimentAnalysisResult } from '@/services/experiment-api'
import { useSessionStore } from '@/store'
import { useSettingsStore } from '@/store/settings-store'

interface ExperimentAnalysisPanelProps {
  projectId: string
  /** 与生成 LaTeX 完全一致的显示矩阵。 */
  table: ExperimentTable
  bold: BoldMode[]
  caption: string
  label: string
  onInsert: (text: string) => void
  /** 生成前先冲刷编辑器未保存内容，保证服务端读到最新正文。 */
  onBeforeGenerate: () => Promise<void>
}

export function ExperimentAnalysisPanel({
  projectId, table, bold, caption, label, onInsert, onBeforeGenerate,
}: ExperimentAnalysisPanelProps) {
  const navigate = useNavigate()
  const isGuest = useSessionStore((state) => state.isGuest)
  const { settings, fetchSettings } = useSettingsStore()
  const agent = settings?.agent
  const hasModel = (agent?.providers?.length ?? 0) > 0 || Boolean(agent?.server_fallback?.configured)

  // 与 AI 面板同样：没有设置数据时自己去取，否则「未配置」会是假象。
  useEffect(() => {
    if (!settings && !isGuest) void fetchSettings().catch(() => {})
  }, [settings, fetchSettings, isGuest])

  const [language, setLanguage] = useState<'zh' | 'en'>('zh')
  const [focus, setFocus] = useState('')
  const [status, setStatus] = useState<'idle' | 'loading' | 'error' | 'success'>('idle')
  const [error, setError] = useState('')
  const [result, setResult] = useState<ExperimentAnalysisResult | null>(null)
  const [paragraphs, setParagraphs] = useState<string[]>([])
  const abortRef = useRef<AbortController | null>(null)

  // 表格设置变化后旧引用可能与当前表格不一致，必须清空重生成。
  const signature = useMemo(() => JSON.stringify({ headers: table.headers, rows: table.rows, bold, caption }), [table, bold, caption])
  useEffect(() => {
    setStatus('idle')
    setResult(null)
    setParagraphs([])
    setError('')
  }, [signature])

  useEffect(() => () => abortRef.current?.abort(), [])

  const generate = async () => {
    setStatus('loading')
    setError('')
    const controller = new AbortController()
    abortRef.current = controller
    try {
      await onBeforeGenerate()
      const response = await generateExperimentAnalysis(projectId, {
        table: { headers: table.headers, rows: table.rows, caption, label, bold },
        language,
        focus: focus.trim() || undefined,
      }, controller.signal)
      setResult(response)
      setParagraphs(response.paragraphs.map((item) => item.text))
      setStatus('success')
    } catch (err) {
      if (controller.signal.aborted) {
        setStatus('idle')
        return
      }
      setStatus('error')
      setError(friendlyErrorMessage(err))
    } finally {
      if (abortRef.current === controller) abortRef.current = null
    }
  }

  const cancel = () => {
    abortRef.current?.abort()
    abortRef.current = null
    setStatus('idle')
  }

  const insert = () => {
    const text = paragraphs
      .map((paragraph) => paragraph.trim())
      .filter(Boolean)
      .map((paragraph) => escapeLatex(paragraph))
      .join('\n\n')
    if (!text) return
    onInsert(text)
    toast.success('分析草稿已插入编辑器', { description: '请核对表述与数值后再定稿。' })
  }

  if (isGuest) {
    return <p className="text-sm text-muted-foreground">实验分析需要登录后连接云端模型；「表格生成」仍可离线使用。</p>
  }

  if (!hasModel) {
    return (
      <div className="rounded-lg border border-border/60 p-4">
        <p className="text-sm font-medium">尚未配置 AI 模型</p>
        <p className="mt-1 text-xs text-muted-foreground">
          表格生成不受影响；要让模型基于这张表写分析草稿，请先在设置里添加模型服务商。
        </p>
        <Button size="sm" variant="outline" className="mt-3 min-h-11 md:min-h-9" onClick={() => void navigate({ to: '/app/settings' })}>
          去设置
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 gap-3 sm:grid-cols-[140px_1fr]">
        <div className="space-y-1">
          <Label htmlFor="experiment-language" className="text-xs">写作语言</Label>
          <Select value={language} onValueChange={(value) => setLanguage(value as 'zh' | 'en')} disabled={status === 'loading'}>
            <SelectTrigger id="experiment-language" className="h-11 w-full md:h-9"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="zh">中文</SelectItem>
              <SelectItem value="en">English</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label htmlFor="experiment-focus" className="text-xs">补充要求（可选）</Label>
          <Textarea
            id="experiment-focus"
            value={focus}
            maxLength={500}
            rows={2}
            placeholder="例如：重点说明本文方法相对基线的提升，并指出局限"
            disabled={status === 'loading'}
            onChange={(event) => setFocus(event.target.value)}
          />
        </div>
      </div>

      {status === 'idle' && (
        <Button onClick={() => void generate()} className="min-h-11 md:min-h-9">
          <Sparkles size={14} /> 基于这张表生成分析草稿
        </Button>
      )}

      {status === 'loading' && (
        <div className="flex items-center gap-3 text-sm text-muted-foreground">
          <Loader2 size={16} className="animate-spin" />
          正在结合项目规范与章节计划生成，最多约 2 分钟…
          <Button size="sm" variant="outline" className="min-h-11 md:min-h-9" onClick={cancel}>
            <Square size={12} /> 取消
          </Button>
        </div>
      )}

      {status === 'error' && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/5 p-3">
          <p className="text-sm text-destructive">{error}</p>
          <Button size="sm" variant="outline" className="mt-2 min-h-11 md:min-h-9" onClick={() => void generate()}>
            重新生成
          </Button>
        </div>
      )}

      {status === 'success' && result && (
        <div className="space-y-3">
          {result.context.warnings.length > 0 && (
            <div className="rounded-lg border border-amber-500/40 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-400">
              <p className="font-medium">生成时缺少以下上下文，结论可能不完整：</p>
              <ul className="mt-1 list-disc pl-4">
                {result.context.warnings.map((warning) => <li key={warning}>{warning}</li>)}
              </ul>
            </div>
          )}

          <p className="text-[11px] text-muted-foreground">
            来源：{result.model.name || '已配置模型'}（{result.model.source === 'user-settings' ? '我的设置' : '服务端配置'}）；
            规范条款 {result.context.normsCount} 条，章节计划{result.context.planStatus === 'missing' ? '缺失' : result.context.planStatus === 'candidate' ? '为候选未采用' : '已采用'}。
          </p>

          {paragraphs.map((paragraph, index) => {
            const citations = result.paragraphs[index]?.citations ?? []
            return (
              <div key={index} className="space-y-1.5">
                <Label htmlFor={`experiment-paragraph-${index}`} className="text-xs text-muted-foreground">第 {index + 1} 段（可编辑）</Label>
                <Textarea
                  id={`experiment-paragraph-${index}`}
                  value={paragraph}
                  rows={Math.min(8, Math.max(3, Math.ceil(paragraph.length / 46)))}
                  onChange={(event) => setParagraphs((prev) => prev.map((item, i) => (i === index ? event.target.value : item)))}
                />
                {citations.length > 0 && (
                  <div className="flex flex-wrap gap-1">
                    {citations.map((citation) => (
                      <Badge key={`${citation.row}-${citation.column}-${citation.value}`} variant="secondary" className="font-normal">
                        {citation.rowLabel || `第 ${citation.row + 1} 行`} · {citation.columnLabel || `第 ${citation.column + 1} 列`} = {citation.value}
                      </Badge>
                    ))}
                  </div>
                )}
              </div>
            )
          })}

          {result.normRefs.length > 0 && (
            <div className="flex flex-wrap gap-1">
              {result.normRefs.map((ref) => (
                <Badge key={ref.cite} variant="outline" title={`${ref.rule}\n出处：${ref.source}`}>{ref.cite}</Badge>
              ))}
            </div>
          )}

          <div className="flex flex-wrap items-center gap-2">
            <Button onClick={insert} className="min-h-11 md:min-h-9">插入到编辑器</Button>
            <Button variant="outline" onClick={() => void generate()} className="min-h-11 md:min-h-9">
              <Sparkles size={14} /> 重新生成
            </Button>
          </div>

          <p className="flex items-start gap-1 text-[11px] text-muted-foreground">
            <AlertTriangle size={12} className="mt-0.5 shrink-0" />
            AI 生成内容：引用的单元格数值已由系统逐字核对，但结论与措辞请结合实验设计自行判断；插入后可在编辑器里继续修改。
          </p>
        </div>
      )}
    </div>
  )
}
