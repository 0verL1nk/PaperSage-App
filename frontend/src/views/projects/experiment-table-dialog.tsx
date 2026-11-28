import { useMemo, useRef, useState } from 'react'
import { Copy, FileUp, Table2 } from 'lucide-react'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { buildBooktabs, displayRows } from '@/lib/experiment-table/booktabs'
import { CsvParseError, decodeExperimentFile, parseDelimited } from '@/lib/experiment-table/csv'
import type { BoldMode, BuildOptions, ExperimentTable } from '@/lib/experiment-table/types'
import { ExperimentAnalysisPanel } from './experiment-analysis-panel'

const PRECISION_OPTIONS = [
  { value: 'raw', label: '保留原始数值' },
  { value: '0', label: '0 位小数' },
  { value: '1', label: '1 位小数' },
  { value: '2', label: '2 位小数' },
  { value: '3', label: '3 位小数' },
  { value: '4', label: '4 位小数' },
] as const

const BOLD_OPTIONS: { value: BoldMode; label: string }[] = [
  { value: 'none', label: '不加粗' },
  { value: 'max', label: '最大值为优' },
  { value: 'min', label: '最小值为优' },
]

interface ExperimentTableDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  projectId: string
  activeFilePath: string | null
  canInsert: boolean
  onInsert: (latex: string) => void
  onBeforeGenerate: () => Promise<void>
}

export function ExperimentTableDialog({
  open, onOpenChange, projectId, activeFilePath, canInsert, onInsert, onBeforeGenerate,
}: ExperimentTableDialogProps) {
  const [tab, setTab] = useState('table')
  const [rawText, setRawText] = useState('')
  const [hasHeader, setHasHeader] = useState(true)
  const [table, setTable] = useState<ExperimentTable | null>(null)
  const [warnings, setWarnings] = useState<string[]>([])
  const [error, setError] = useState('')
  const [caption, setCaption] = useState('实验结果对比')
  const [label, setLabel] = useState('tab:results')
  const [precision, setPrecision] = useState('raw')
  const [bold, setBold] = useState<BoldMode[]>([])
  const [includeFloat, setIncludeFloat] = useState(true)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const parse = (text: string, header: boolean) => {
    try {
      const result = parseDelimited(text, { hasHeader: header })
      setTable(result.table)
      setBold(result.table.headers.map(() => 'none'))
      setWarnings(result.warnings)
      setError('')
    } catch (err) {
      setTable(null)
      setBold([])
      setWarnings([])
      setError(err instanceof CsvParseError ? err.message : '解析失败，请检查数据格式后重试')
    }
  }

  const handleFile = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    event.target.value = ''
    if (!file) return
    try {
      const text = await decodeExperimentFile(file)
      setRawText(text)
      parse(text, hasHeader)
    } catch (err) {
      setTable(null)
      setError(err instanceof CsvParseError ? err.message : '读取文件失败，请重试')
    }
  }

  const options: BuildOptions = useMemo(() => ({
    caption,
    label,
    precision: precision === 'raw' ? null : Number(precision),
    bold,
    includeFloat,
  }), [caption, label, precision, bold, includeFloat])

  const latex = useMemo(() => (table ? buildBooktabs(table, options) : ''), [table, options])
  const displayTable = useMemo<ExperimentTable | null>(
    () => (table ? { headers: table.headers, rows: displayRows(table, options.precision) } : null),
    [table, options.precision],
  )

  const updateHeader = (index: number, value: string) => {
    setTable((prev) => (prev ? { ...prev, headers: prev.headers.map((header, i) => (i === index ? value : header)) } : prev))
  }

  const updateBold = (index: number, mode: BoldMode) => {
    setBold((prev) => prev.map((item, i) => (i === index ? mode : item)))
  }

  const copy = async (text: string, message: string) => {
    try {
      await navigator.clipboard.writeText(text)
      toast.success(message)
    } catch {
      toast.error('复制失败', { description: '请手动选中文本复制。' })
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[92vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2"><Table2 size={16} /> 实验数据 → 表格与分析</DialogTitle>
          <DialogDescription>
            表格由浏览器内的确定性解析生成，数值不会被模型改写；分析草稿由真实模型基于这张表、项目规范与章节计划生成。
          </DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={setTab}>
          <TabsList className="w-full">
            <TabsTrigger value="table" className="min-h-11 flex-1 md:min-h-8">表格生成</TabsTrigger>
            <TabsTrigger value="analysis" className="min-h-11 flex-1 md:min-h-8" disabled={!table}>结果分析</TabsTrigger>
          </TabsList>

          <TabsContent value="table" className="space-y-4">
            <div className="space-y-2">
              <div className="flex flex-wrap items-center gap-2">
                <Label className="text-xs">粘贴 CSV / TSV（Excel 直接复制即可）</Label>
                <input
                  ref={fileInputRef}
                  type="file"
                  accept=".csv,.tsv,.txt,text/csv,text/tab-separated-values"
                  className="hidden"
                  onChange={(event) => void handleFile(event)}
                />
                <Button type="button" size="sm" variant="outline" className="min-h-11 md:min-h-8" onClick={() => fileInputRef.current?.click()}>
                  <FileUp size={14} /> 上传文件
                </Button>
                <label className="flex min-h-11 items-center gap-2 text-xs text-muted-foreground md:min-h-0">
                  <Checkbox checked={hasHeader} onCheckedChange={(checked) => {
                    const next = checked === true
                    setHasHeader(next)
                    if (rawText.trim()) parse(rawText, next)
                  }} />
                  首行是表头
                </label>
              </div>
              <Textarea
                value={rawText}
                rows={4}
                placeholder={'方法,准确率,F1\nBaseline,88.1,0.842\nOurs,92.5,0.881'}
                onChange={(event) => setRawText(event.target.value)}
              />
              <Button type="button" size="sm" onClick={() => parse(rawText, hasHeader)} disabled={!rawText.trim()} className="min-h-11 md:min-h-8">
                解析数据
              </Button>
            </div>

            {error && (
              <p role="alert" className="rounded-md border border-destructive/40 bg-destructive/5 p-2 text-sm text-destructive">
                {error}
              </p>
            )}
            {warnings.length > 0 && (
              <ul className="list-disc rounded-md border border-amber-500/40 bg-amber-500/5 p-2 pl-6 text-xs text-amber-700 dark:text-amber-400">
                {warnings.map((warning) => <li key={warning}>{warning}</li>)}
              </ul>
            )}

            {!table && !error && (
              <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
                粘贴或上传实验数据后点「解析数据」，这里会出现表格预览。
              </p>
            )}

            {table && (
              <>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div className="space-y-1">
                    <Label htmlFor="experiment-caption" className="text-xs">表题（caption）</Label>
                    <Input id="experiment-caption" value={caption} onChange={(event) => setCaption(event.target.value)} />
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="experiment-label" className="text-xs">引用标签（label）</Label>
                    <Input id="experiment-label" value={label} onChange={(event) => setLabel(event.target.value)} />
                    <p className="text-[10px] text-muted-foreground">只保留英文、数字、冒号、点与短横线。</p>
                  </div>
                  <div className="space-y-1">
                    <Label htmlFor="experiment-precision" className="text-xs">数值精度（只影响显示）</Label>
                    <Select value={precision} onValueChange={setPrecision}>
                      <SelectTrigger id="experiment-precision" className="h-11 w-full md:h-9"><SelectValue /></SelectTrigger>
                      <SelectContent>
                        {PRECISION_OPTIONS.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                      </SelectContent>
                    </Select>
                  </div>
                </div>

                <div className="max-h-72 overflow-auto rounded-md border">
                  <table className="w-full min-w-max text-sm">
                    <thead className="sticky top-0 z-10 bg-muted">
                      <tr>
                        {table.headers.map((header, index) => (
                          <th key={index} className="min-w-36 border-b p-1.5 text-left align-top">
                            <Input
                              aria-label={`第 ${index + 1} 列表头`}
                              value={header}
                              className="h-8"
                              onChange={(event) => updateHeader(index, event.target.value)}
                            />
                            <Select value={bold[index] ?? 'none'} onValueChange={(value) => updateBold(index, value as BoldMode)}>
                              <SelectTrigger aria-label={`第 ${index + 1} 列加粗规则`} className="mt-1 h-8 w-full text-xs">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {BOLD_OPTIONS.map((option) => <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>)}
                              </SelectContent>
                            </Select>
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {displayTable?.rows.map((row, rowIndex) => (
                        <tr key={rowIndex} className="border-b last:border-0">
                          {row.map((cell, columnIndex) => (
                            <td key={columnIndex} className="px-2 py-1.5 whitespace-nowrap">{cell}</td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>

                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  <Checkbox checked={includeFloat} onCheckedChange={(checked) => setIncludeFloat(checked === true)} />
                  包含 table 浮动环境（关闭后只生成 tabular，便于插入已有 table）
                </label>

                <div className="space-y-2">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <Label className="text-xs">LaTeX 预览</Label>
                    <div className="flex flex-wrap gap-2">
                      <Button type="button" size="sm" variant="outline" className="min-h-11 md:min-h-8" onClick={() => void copy(latex, 'LaTeX 已复制')}>
                        <Copy size={14} /> 复制 LaTeX
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        className="min-h-11 md:min-h-8"
                        disabled={!canInsert}
                        onClick={() => onInsert(latex)}
                      >
                        插入到编辑器
                      </Button>
                    </div>
                  </div>
                  {!canInsert && <p className="text-[11px] text-muted-foreground">请先在左侧打开一个可编辑的 .tex 文件，再插入表格。</p>}
                  <pre className="max-h-56 overflow-auto rounded-md bg-muted p-3 text-xs leading-relaxed"><code>{latex}</code></pre>
                </div>

                <div className="rounded-md border border-border/60 p-2 text-[11px] text-muted-foreground">
                  表格依赖 booktabs 宏包。新项目模板已包含；存量项目若编译报 <code>Undefined control sequence \toprule</code>，
                  请在 main.tex 导言区加入：
                  <code className="mx-1 rounded bg-muted px-1 py-0.5">\usepackage{'{'}booktabs{'}'}</code>
                  <Button type="button" size="sm" variant="ghost" className="h-11 md:h-7" onClick={() => void copy('\\usepackage{booktabs}', '宏包命令已复制')}>
                    复制
                  </Button>
                </div>
              </>
            )}
          </TabsContent>

          <TabsContent value="analysis">
            {table && displayTable ? (
              <ExperimentAnalysisPanel
                projectId={projectId}
                table={displayTable}
                bold={bold}
                caption={caption}
                label={label}
                onInsert={onInsert}
                onBeforeGenerate={onBeforeGenerate}
              />
            ) : (
              <p className="rounded-md border border-dashed p-6 text-center text-sm text-muted-foreground">
                先在「表格生成」里解析实验数据，再来生成有据分析草稿。
              </p>
            )}
            {activeFilePath && table && (
              <p className="mt-2 text-[11px] text-muted-foreground">插入目标：{activeFilePath}</p>
            )}
          </TabsContent>
        </Tabs>
      </DialogContent>
    </Dialog>
  )
}
