/**
 * 从 LaTeX 源文本里确定性地取出「光标所在段落 + 最近的章节路径」。
 *
 * 只做结构切分（空行、章节命令、环境边界），不做任何语义判断：
 * 「这段写得好不好」由真实模型判断，前端不能也不该猜。
 */

export interface ParagraphFocus {
  /** 段落的逐字文本（去掉首尾空白；仍是原文的连续子串）。 */
  paragraphText: string
  /** 最近的章节路径，如「第1章 绪论 / 1.1 研究背景」；没有则为空串。 */
  sectionPath: string
}

const SECTION_RE = /^\s*\\(chapter|section|subsection|subsubsection)\*?(?:\[[^\]]*\])?\{([^}]*)\}/
const STRUCTURAL_RE = /^\s*\\(?:begin|end)\s*\{|^\s*\\(?:documentclass|usepackage|newcommand|renewcommand|input|include|bibliography|bibliographystyle)\b/
const BLANK_RE = /^\s*$/
const ENV_BEGIN_RE = /^\s*\\begin\{([a-zA-Z*]+)\}/
const ENV_END_RE = /^\s*\\end\{([a-zA-Z*]+)\}/
const MATH_ENVS = new Set([
  'equation', 'equation*', 'align', 'align*', 'gather', 'gather*', 'multline', 'multline*',
  'eqnarray', 'eqnarray*', 'displaymath', 'math', 'split', 'cases', 'array', 'matrix',
  'pmatrix', 'bmatrix', 'vmatrix',
])

interface Line {
  text: string
  start: number
}

function splitLines(text: string): Line[] {
  const lines: Line[] = []
  let start = 0
  for (const raw of text.split('\n')) {
    lines.push({ text: raw, start })
    start += raw.length + 1
  }
  return lines
}

function cleanTitle(raw: string): string {
  return raw.replace(/\\[a-zA-Z@]+\*?(?:\[[^\]]*\])?/g, '').replace(/[{}]/g, '').trim()
}

export interface SectionRange {
  /** 章节路径（与 sectionPathAt 保持一致）。 */
  path: string
  /** 章节命令结束后的正文起点。 */
  start: number
  /** 下一章节命令起点（文件末尾则为文本长度）。 */
  end: number
}

/**
 * 把全文按章节命令切成区间，用于「同段文字多处出现」时按章节消歧。
 * 只做结构切分，不做语义判断。
 */
export function sectionRanges(text: string): SectionRange[] {
  const lines = splitLines(text)
  const heads: Array<{ start: number; path: string }> = []
  const levels: string[] = []
  for (const line of lines) {
    const m = SECTION_RE.exec(line.text)
    if (!m) continue
    const index = m[1] === 'chapter' ? 0 : m[1] === 'section' ? 1 : 2
    levels.length = index
    levels[index] = cleanTitle(m[2])
    heads.push({ start: line.start, path: levels.filter(Boolean).join(' / ') })
  }
  if (!heads.length) return []
  return heads.map((head, i) => ({
    path: head.path,
    start: head.start,
    end: i + 1 < heads.length ? heads[i + 1].start : text.length,
  }))
}

/** 光标之前最近的三级章节标题（chapter / section / subsection）。 */
export function sectionPathAt(text: string, offset: number): string {
  const lines = splitLines(text)
  const levels: string[] = []
  for (const line of lines) {
    if (line.start > offset) break
    const m = SECTION_RE.exec(line.text)
    if (!m) continue
    const level = m[1]
    const title = cleanTitle(m[2])
    const index = level === 'chapter' ? 0 : level === 'section' ? 1 : 2
    levels.length = index
    levels[index] = title
  }
  return levels.filter(Boolean).join(' / ')
}

interface ParagraphBounds {
  start: number
  end: number
}

function findParagraphBounds(text: string, offset: number): ParagraphBounds | null {
  if (offset < 0 || offset > text.length) return null
  const lines = splitLines(text)
  let current: Line[] = []

  const flush = (): ParagraphBounds | null => {
    if (!current.length) return null
    const first = current[0]
    const last = current[current.length - 1]
    const start = first.start
    const end = last.start + last.text.length
    if (!text.slice(start, end).trim()) return null
    return { start, end }
  }
  const contains = (line: Line) => offset >= line.start && offset <= line.start + line.text.length
  let mathDepth = 0

  for (const line of lines) {
    const begin = ENV_BEGIN_RE.exec(line.text)
    const end = ENV_END_RE.exec(line.text)
    const mathBoundary = (begin && MATH_ENVS.has(begin[1])) || (end && MATH_ENVS.has(end[1]))
    if (mathBoundary) {
      if (contains(line)) return null
      if (begin) mathDepth++
      if (end && mathDepth > 0) mathDepth--
      current = []
      continue
    }
    if (mathDepth > 0) {
      // 公式环境内部不是可给写作建议的正文段落。
      if (contains(line)) return null
      current = []
      continue
    }
    const isBoundary = BLANK_RE.test(line.text) || SECTION_RE.test(line.text) || STRUCTURAL_RE.test(line.text)
    if (isBoundary) {
      if (current.some(contains)) {
        const bounds = flush()
        if (bounds) return bounds
      }
      current = []
      if (contains(line)) return null
      continue
    }
    current.push(line)
  }
  if (current.some(contains)) return flush()
  return null
}

/**
 * 取光标所在段落。光标落在章节命令、环境边界或空行上时返回 null——
 * 那不是一个可给写作建议的正文段落。
 */
export function paragraphAt(text: string, offset: number): ParagraphFocus | null {
  const bounds = findParagraphBounds(text, offset)
  if (!bounds) return null
  return {
    paragraphText: text.slice(bounds.start, bounds.end).trim(),
    sectionPath: sectionPathAt(text, bounds.start),
  }
}

/** 段落原始区间（未 trim），用于「同段多条批注」翻页。 */
export function paragraphRangeAt(text: string, offset: number): ParagraphBounds | null {
  return findParagraphBounds(text, offset)
}
