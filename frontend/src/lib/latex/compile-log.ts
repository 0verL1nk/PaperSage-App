/**
 * 确定性 LaTeX 日志解析：只定位（文件、行号、引擎原话），不做任何语义解读。
 *
 * 报错原因与修复建议必须由真实模型给出（见 compile-failure-panel / 服务端
 * compile-diagnosis）；本文件绝不产出「修复建议」或按关键词映射错误类型。
 *
 * TeX 日志不是行记录，而是带文件作用域的流：`(./chapters/intro.tex` 打开
 * 一个文件作用域，`)` 关闭；错误归属 = 出错位置的栈顶文件。解析不出来时
 * 显式给 `filePath: null`（不可跳转），不猜。
 */

export type CompileIssueSeverity = 'error' | 'warning'

export interface CompileIssue {
  severity: CompileIssueSeverity
  /** 归一化后的项目内路径；无法归属时为 null（仍然展示，不可跳转） */
  filePath: string | null
  line: number | null
  /** 引擎原话（多行合并为一行，≤500 字符），不改写、不翻译 */
  message: string
  source: 'latex' | 'bibtex'
}

export interface ParseLatexLogInput {
  log: string
  projectFiles: string[]
  rootFile: string
  /** 默认 200 */
  maxIssues?: number
}

const MAX_LOG_CHARS = 2 * 1024 * 1024
const MAX_ISSUES = 200
const MAX_MESSAGE_CHARS = 500
const EXCERPT_MAX_CHARS = 32 * 1024

const FILE_EXT = 'tex|sty|cls|bib|def|bbl|aux'
// `(./chapters/intro.tex` —— 只有紧跟 `(` 且带已知扩展名的 token 才是文件作用域。
const FILE_OPEN_RE = new RegExp(`\\(([^\\s()]+\\.(?:${FILE_EXT}))`, 'g')
const ERROR_FILE_LINE_RE = new RegExp(`^(.+?\\.(?:${FILE_EXT})):(\\d+):\\s?(.*)$`)
const BANG_ERROR_RE = /^!\s?(.*)$/
const LINE_NO_RE = /^l\.(\d+)\s?(.*)$/
const WARNING_RE = /^(?:LaTeX|Package\s+\S+|Class\s+\S+)\s+Warning:\s*(.*)$/
const BIB_MESSAGE_RE = /^I (?:couldn't|found|was expecting|got)\b.*$/
const BIB_LINE_RE = /^---line (\d+) of file (\S+)/
// 警告续行形如 `(hyperref)   removing ...`，括号里不是文件路径。
const WARNING_CONTINUATION_RE = /^\([^()\s]+\)\s?/
const PACKAGE_PAREN_RE = /\([A-Za-z][\w.+-]*\)/g

interface LogSegment {
  kind: 'tex' | 'bibtex'
  lines: string[]
}

function splitSegments(log: string): LogSegment[] {
  const segments: LogSegment[] = []
  let name = ''
  let lines: string[] = []
  const push = () => {
    if (lines.length === 0 && name === '') return
    segments.push({ kind: /^bibtex\b/i.test(name) ? 'bibtex' : 'tex', lines })
  }
  for (const line of log.split(/\r?\n/)) {
    const header = /^---\s+(.+?)\s+---\s*$/.exec(line)
    if (header) {
      push()
      name = header[1]
      lines = []
      continue
    }
    lines.push(line)
  }
  push()
  return segments
}

function cleanMessage(raw: string): string {
  const text = raw.replace(/\s+/g, ' ').trim()
  return text.length > MAX_MESSAGE_CHARS ? `${text.slice(0, MAX_MESSAGE_CHARS - 1)}…` : text
}

/** 去 `./`、`/work/`、Windows 反斜杠；先精确匹配，再唯一后缀/唯一基名，仍不中 → null。 */
export function normalizeLogPath(raw: string | null | undefined, projectFiles: string[]): string | null {
  if (!raw) return null
  let path = raw.trim().replace(/\\/g, '/').replace(/^[(`]+/, '')
  path = path.replace(/^\.\//, '')
  const work = path.lastIndexOf('/work/')
  if (work >= 0) path = path.slice(work + '/work/'.length)
  path = path.replace(/^\/+/, '')
  if (path === '') return null

  const exact = projectFiles.find((file) => file === path)
  if (exact) return exact
  const suffix = projectFiles.filter((file) => path.endsWith(`/${file}`))
  if (suffix.length === 1) return suffix[0]
  const base = path.slice(path.lastIndexOf('/') + 1)
  const byBase = projectFiles.filter((file) => file.slice(file.lastIndexOf('/') + 1) === base)
  return byBase.length === 1 ? byBase[0] : null
}

/**
 * 每行的栈顶文件。开：`(` + 已知扩展名；关：`)`（跳过 `(pkg)` 这类非路径括号）。
 * 同一行既有开又有闭时不计数，避免 `(./x.tex)` 立刻把自己的作用域弹掉。
 */
function trackFileStack(lines: string[]): Array<string | null> {
  const stack: string[] = []
  const tops: Array<string | null> = []
  for (const line of lines) {
    FILE_OPEN_RE.lastIndex = 0
    const opened: string[] = []
    let match: RegExpExecArray | null
    while ((match = FILE_OPEN_RE.exec(line))) opened.push(match[1])
    for (const file of opened) stack.push(file)
    if (opened.length === 0) {
      const closings = (line.replace(PACKAGE_PAREN_RE, '').match(/\)/g) ?? []).length
      for (let i = 0; i < closings && stack.length > 0; i++) stack.pop()
    }
    tops.push(stack.length > 0 ? stack[stack.length - 1] : null)
  }
  return tops
}

function hasErrorRecord(segment: LogSegment): boolean {
  return segment.lines.some((line) => {
    const trimmed = line.trim()
    return BANG_ERROR_RE.test(trimmed) || ERROR_FILE_LINE_RE.test(trimmed)
  })
}

function parseTexErrors(segment: LogSegment, projectFiles: string[], fallbackFile: string): CompileIssue[] {
  const lines = segment.lines
  const tops = trackFileStack(lines)
  // 整段日志没有任何可识别的文件作用域时，错误属于本次编译入口文件（已知输入，不是猜测）。
  const hasScope = tops.some((top) => top !== null)
  const issues: CompileIssue[] = []
  for (let i = 0; i < lines.length; i++) {
    const trimmed = lines[i].trim()
    const fileLine = ERROR_FILE_LINE_RE.exec(trimmed)
    if (fileLine) {
      issues.push({
        severity: 'error',
        filePath: normalizeLogPath(fileLine[1], projectFiles),
        line: toLineNumber(fileLine[2]),
        message: cleanMessage(fileLine[3]) || 'LaTeX 编译错误',
        source: 'latex',
      })
      continue
    }
    const bang = BANG_ERROR_RE.exec(trimmed)
    if (!bang) continue
    let line: number | null = null
    for (let j = i + 1; j < Math.min(i + 10, lines.length); j++) {
      const lineNo = LINE_NO_RE.exec(lines[j].trim())
      if (lineNo) {
        line = toLineNumber(lineNo[1])
        break
      }
      if (BANG_ERROR_RE.test(lines[j].trim())) break
    }
    const top = tops[i]
    issues.push({
      severity: 'error',
      filePath: normalizeLogPath(top ?? (hasScope ? null : fallbackFile), projectFiles),
      line,
      message: cleanMessage(bang[1]) || 'LaTeX 编译错误',
      source: 'latex',
    })
  }
  return issues
}

function parseTexWarnings(segment: LogSegment, projectFiles: string[]): CompileIssue[] {
  const lines = segment.lines
  const tops = trackFileStack(lines)
  const issues: CompileIssue[] = []
  for (let i = 0; i < lines.length; i++) {
    const warning = WARNING_RE.exec(lines[i].trim())
    if (!warning) continue
    const parts = [warning[1]]
    for (let j = i + 1; j < lines.length && WARNING_CONTINUATION_RE.test(lines[j].trim()); j++) {
      parts.push(lines[j].trim().replace(WARNING_CONTINUATION_RE, ''))
    }
    const message = cleanMessage(parts.join(' '))
    // Overfull/Underfull 是排版噪音，动辄成百上千条，明确排除。
    if (/Overfull|Underfull/i.test(message)) continue
    const lineMatch = /on input line (\d+)/.exec(message)
    issues.push({
      severity: 'warning',
      filePath: normalizeLogPath(tops[i], projectFiles),
      line: lineMatch ? toLineNumber(lineMatch[1]) : null,
      message,
      source: 'latex',
    })
  }
  return issues
}

function parseBibtexErrors(segment: LogSegment): CompileIssue[] {
  const lines = segment.lines
  const issues: CompileIssue[] = []
  for (let i = 0; i < lines.length; i++) {
    const text = lines[i].trim()
    const message = BIB_MESSAGE_RE.exec(text)
    const location = BIB_LINE_RE.exec(text)
    if (message) {
      let line: number | null = null
      for (let j = i + 1; j < Math.min(i + 6, lines.length); j++) {
        const nearby = BIB_LINE_RE.exec(lines[j].trim())
        if (nearby) {
          line = toLineNumber(nearby[1])
          break
        }
      }
      issues.push({ severity: 'error', filePath: null, line, message: cleanMessage(text), source: 'bibtex' })
      continue
    }
    if (location) {
      // `---line N of file X` 是上一条 BibTeX 消息的定位；上一条已消费则不重复产出。
      const previous = i > 0 ? lines[i - 1].trim() : ''
      if (previous && BIB_MESSAGE_RE.test(previous)) continue
      issues.push({
        severity: 'error',
        filePath: null,
        line: toLineNumber(location[1]),
        message: cleanMessage(previous ? `${previous} (${text})` : text),
        source: 'bibtex',
      })
    }
  }
  return issues
}

function toLineNumber(raw: string): number | null {
  const value = Number(raw)
  return Number.isInteger(value) && value > 0 ? value : null
}

function dedupeIssues(issues: CompileIssue[]): CompileIssue[] {
  const seen = new Set<string>()
  const out: CompileIssue[] = []
  for (const issue of issues) {
    const key = `${issue.severity}\0${issue.filePath ?? ''}\0${issue.line ?? ''}\0${issue.message}`
    if (seen.has(key)) continue
    seen.add(key)
    out.push(issue)
  }
  return out
}

/**
 * 解析浏览器 WASM 编译日志。错误取最后一个含错误的 TeX 段（多遍编译早期遍
 * 常有假报错）；警告取最后一个 TeX 段；BibTeX 错误取最后一个 BibTeX 段。
 */
export function parseLatexLog(input: ParseLatexLogInput): CompileIssue[] {
  const maxIssues = input.maxIssues ?? MAX_ISSUES
  const log = input.log.length > MAX_LOG_CHARS ? input.log.slice(-MAX_LOG_CHARS) : input.log
  if (!log.trim()) return []

  const segments = splitSegments(log)
  const texSegments = segments.filter((segment) => segment.kind === 'tex')
  const errorSegment = [...texSegments].reverse().find(hasErrorRecord)
  const warningSegment = texSegments[texSegments.length - 1]
  const bibtexSegment = [...segments].reverse().find((segment) => segment.kind === 'bibtex')

  const errors = errorSegment ? parseTexErrors(errorSegment, input.projectFiles, input.rootFile) : []
  const warnings = warningSegment ? parseTexWarnings(warningSegment, input.projectFiles) : []
  const bibtex = bibtexSegment ? parseBibtexErrors(bibtexSegment) : []

  return dedupeIssues([...errors, ...bibtex, ...warnings]).slice(0, maxIssues)
}

/**
 * 取该错误附近的日志窗口（默认 32 KiB），供 AI 诊断请求使用。
 * 优先命中 `l.N` 或错误原文；找不到时退化为日志末尾。
 */
export function logExcerptForIssue(log: string, issue: CompileIssue, maxChars = EXCERPT_MAX_CHARS): string {
  if (log.length <= maxChars) return log
  const anchors = [
    issue.line !== null ? `l.${issue.line}` : '',
    issue.message.slice(0, 40),
  ].filter(Boolean)
  let index = -1
  for (const anchor of anchors) {
    index = log.indexOf(anchor)
    if (index >= 0) break
  }
  if (index < 0) return log.slice(-maxChars)
  const before = Math.floor(maxChars / 2)
  const start = Math.max(0, Math.min(index - before, log.length - maxChars))
  return log.slice(start, start + maxChars)
}
