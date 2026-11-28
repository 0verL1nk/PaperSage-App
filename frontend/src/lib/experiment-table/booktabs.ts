import Big from 'big.js'
import type { BoldMode, BuildOptions, ExperimentTable } from './types'

/** 严格数值单元格：可选正负号/小数/科学计数法，允许 `%` 后缀。 */
const NUMERIC_RE = /^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?%?$/

const LATEX_ESCAPES: Record<string, string> = {
  '\\': '\\textbackslash{}',
  '&': '\\&',
  '%': '\\%',
  '$': '\\$',
  '#': '\\#',
  '_': '\\_',
  '{': '\\{',
  '}': '\\}',
  '~': '\\textasciitilde{}',
  '^': '\\textasciicircum{}',
}

export function isNumericCell(value: string): boolean {
  const trimmed = value.trim()
  return trimmed !== '' && NUMERIC_RE.test(trimmed)
}

function numericValue(value: string): Big | null {
  if (!isNumericCell(value)) return null
  try {
    return new Big(value.trim().replace(/%$/, ''))
  } catch {
    return null
  }
}

/** 单遍替换，避免 `\textbackslash{}` 里新引入的花括号被二次转义。 */
export function escapeLatex(value: string): string {
  return value.replace(/[\\&%$#_{}~^]/g, (ch) => LATEX_ESCAPES[ch])
}

/** label 只保留 LaTeX 引用安全的字符；空值回退到默认值。 */
export function sanitizeLabel(value: string): string {
  return value.trim().replace(/[^A-Za-z0-9:.-]/g, '') || 'tab:results'
}

/** 精度只影响显示：四舍五入用十进制运算，不走浮点。 */
export function formatCell(value: string, precision: number | null): string {
  const trimmed = value.trim()
  if (precision === null || !isNumericCell(trimmed)) return trimmed
  const percent = trimmed.endsWith('%')
  try {
    const formatted = new Big(percent ? trimmed.slice(0, -1) : trimmed).toFixed(precision)
    return percent ? `${formatted}%` : formatted
  } catch {
    return trimmed
  }
}

/** 生成与 LaTeX 完全一致的显示矩阵；分析请求也用它，避免数值口径漂移。 */
export function displayRows(table: ExperimentTable, precision: number | null): string[][] {
  return table.rows.map((row) => row.map((cell) => formatCell(cell, precision)))
}

function columnAlignment(table: ExperimentTable, column: number): 'l' | 'r' {
  const values = table.rows.map((row) => row[column] ?? '').filter((value) => value.trim() !== '')
  return values.length > 0 && values.every(isNumericCell) ? 'r' : 'l'
}

/** 返回该列需要加粗的数据行下标；并列最优全部加粗。 */
function boldRows(table: ExperimentTable, column: number, mode: BoldMode): Set<number> {
  if (mode === 'none') return new Set()
  const values = table.rows
    .map((row, index) => ({ index, value: numericValue(row[column] ?? '') }))
    .filter((item): item is { index: number; value: Big } => item.value !== null)
  if (values.length === 0) return new Set()
  let best = values[0].value
  for (const item of values) {
    if (mode === 'max' ? item.value.gt(best) : item.value.lt(best)) best = item.value
  }
  return new Set(values.filter((item) => item.value.eq(best)).map((item) => item.index))
}

export function buildBooktabs(table: ExperimentTable, options: BuildOptions): string {
  const width = table.headers.length
  const alignment = Array.from({ length: width }, (_, column) => columnAlignment(table, column)).join('')
  const bold = Array.from({ length: width }, (_, column) => boldRows(table, column, options.bold[column] ?? 'none'))
  const display = displayRows(table, options.precision)

  const renderRow = (cells: string[], index?: number) =>
    cells
      .map((cell, column) => {
        const text = escapeLatex(cell)
        return index !== undefined && bold[column].has(index) ? `\\textbf{${text}}` : text
      })
      .join(' & ') + ' \\\\'

  const header = renderRow(table.headers.map((h, column) => h.trim() || `列 ${column + 1}`))
  const body = display.map((row, index) => renderRow(row, index))
  const padding = options.includeFloat ? '  ' : ''

  const lines: string[] = []
  if (options.includeFloat) {
    lines.push('\\begin{table}[htbp]', '  \\centering')
    const caption = options.caption.trim()
    if (caption) lines.push(`  \\caption{${escapeLatex(caption)}}`)
    lines.push(`  \\label{${sanitizeLabel(options.label)}}`)
  }
  lines.push(`${padding}\\begin{tabular}{${alignment}}`, `${padding}  \\toprule`, `${padding}  ${header}`, `${padding}  \\midrule`)
  for (const row of body) lines.push(`${padding}  ${row}`)
  lines.push(`${padding}  \\bottomrule`, `${padding}\\end{tabular}`)
  if (options.includeFloat) lines.push('\\end{table}')
  return lines.join('\n')
}
