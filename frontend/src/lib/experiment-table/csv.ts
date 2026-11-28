import Papa from 'papaparse'
import type { ExperimentTable, ParseResult } from './types'

/** 单文件上限：超过后浏览器解析与表格预览都会明显卡顿。 */
export const MAX_FILE_BYTES = 2 * 1024 * 1024

export class CsvParseError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'CsvParseError'
  }
}

export interface ParseOptions {
  /** 首行是否作为表头；关闭时生成「列 1..N」并允许在界面上改。 */
  hasHeader: boolean
}

function normalizeCell(value: unknown): string {
  // booktabs 单元格不支持多行：引号内的换行折成空格，其余只去首尾空白。
  return String(value ?? '').replace(/\r\n|\r|\n/g, ' ').trim()
}

export function parseDelimited(text: string, options: ParseOptions = { hasHeader: true }): ParseResult {
  const input = text.replace(/^\uFEFF/, '')
  if (!input.trim()) throw new CsvParseError('没有可解析的内容：请粘贴或上传 CSV/TSV 数据')

  const parsed = Papa.parse<string[]>(input.trim(), {
    delimiter: '', // 自动识别逗号/制表符/分号
    skipEmptyLines: 'greedy',
    dynamicTyping: false, // 单元格保持原样字符串，数值转换只发生在生成 LaTeX 时
  })
  const quoteError = parsed.errors.find((err) => err.type === 'Quotes')
  if (quoteError) {
    throw new CsvParseError(`第 ${(quoteError.row ?? 0) + 1} 行的引号不匹配，请检查该行是否漏了成对的引号`)
  }

  const raw = parsed.data as unknown[][]
  if (raw.length === 0) throw new CsvParseError('没有可解析的内容：请粘贴或上传 CSV/TSV 数据')

  const width = raw[0].length
  const headers = options.hasHeader
    ? raw[0].map(normalizeCell)
    : Array.from({ length: width }, (_, i) => `列 ${i + 1}`)
  if (headers.every((h) => h === '')) {
    throw new CsvParseError('表头为空：请把第一行填成列名，或取消「首行是表头」')
  }

  const warnings: string[] = []
  const rows: string[][] = []
  raw.slice(options.hasHeader ? 1 : 0).forEach((row, index) => {
    // 行号按原始文件计（含表头），方便用户回到 Excel 定位。
    const line = index + (options.hasHeader ? 2 : 1)
    const cells = row.map(normalizeCell)
    if (cells.length > width) {
      throw new CsvParseError(`第 ${line} 行有 ${cells.length} 列，表头只有 ${width} 列。请检查分隔符，或删掉多余列`)
    }
    if (cells.length < width) {
      warnings.push(`第 ${line} 行只有 ${cells.length} 列，已按表头补齐空单元格`)
      cells.push(...Array.from({ length: width - cells.length }, () => ''))
    }
    if (cells.every((cell) => cell === '')) return
    rows.push(cells)
  })

  if (rows.length === 0) throw new CsvParseError('只有表头、没有数据行：请补上实验数据')
  const table: ExperimentTable = { headers, rows }
  return { table, warnings }
}

/** UTF-8 优先；Excel 常见的 GBK 导出回退 GBK 解码，仍失败则要求另存。 */
export async function decodeExperimentFile(file: File): Promise<string> {
  if (file.size > MAX_FILE_BYTES) {
    throw new CsvParseError('文件超过 2 MB，请先精简数据后再上传')
  }
  const bytes = await file.arrayBuffer()
  const utf8 = new TextDecoder('utf-8').decode(bytes)
  if (!utf8.includes('�')) return utf8
  try {
    const gbk = new TextDecoder('gbk').decode(bytes)
    if (!gbk.includes('�')) return gbk
  } catch {
    // 浏览器不支持 GBK 解码器时走下面的统一提示
  }
  throw new CsvParseError('无法识别文件编码：请在 Excel 里「另存为 CSV UTF-8」后重试')
}
