/**
 * 全项目秒级确定性体检（纯函数，无模型）。
 *
 * 只做结构层面的硬检查：主文件识别、包含关系、悬空引用/文献键、
 * 重复标签、未引用图表、残留占位符。所有结论标注 source='rule'，
 * 界面必须显示「规则 · 非 AI」；语义判断一律交给真实模型。
 */

export type PreflightSeverity = 'blocking' | 'serious' | 'suggestion'

export type PreflightIssue = {
  id: string
  rule: string
  severity: PreflightSeverity
  title: string
  detail: string
  filePath?: string
  line?: number
  source: 'rule'
}

export type PreflightFile = {
  path: string
  content: string
}

export type PreflightInput = {
  files: PreflightFile[]
  /** 服务端识别的主文件；省略时按 \documentclass 自行识别（游客态）。 */
  roots?: string[]
}

export type PreflightResult = {
  issues: PreflightIssue[]
  roots: string[]
  checkedFiles: number
  warnings: string[]
}

const DOCUMENTCLASS_RE = /\\documentclass(?:\[[^\]]*\])?\s*\{[^}]+\}/
const BEGIN_DOCUMENT_RE = /\\begin\s*\{document\}/
const END_DOCUMENT_RE = /\\end\s*\{document\}/
const LABEL_RE = /\\label\s*\{([^}]+)\}/g
const REF_RE = /\\(?:ref|eqref|autoref|pageref|cref|Cref)\s*\{([^}]+)\}/g
const CITE_RE = /\\(?:cite|parencite|textcite|autocite|citep|citet)\*?(?:\[[^\]]*\]){0,2}\s*\{([^}]+)\}/g
const BIB_ENTRY_RE = /@\w+\s*\{\s*([^,\s]+)\s*,/g
const BIBITEM_RE = /\\bibitem(?:\[[^\]]*\])?\s*\{([^}]+)\}/g
const INCLUDE_RE = /\\(?:input|include|subfile)\s*\{([^}]+)\}/g
const ENVIRONMENT_RE = /\\begin\{(figure|table)\*?\}([\s\S]*?)\\end\{\1\*?\}/g
const PLACEHOLDER_RE = /\b(?:TODO|FIXME|XXX)\b/g

const TEMPLATE_PLACEHOLDERS = ['论文标题', '作者姓名', '从这里开始撰写']

/** 与后端 latex.StripLatexComments 相同规则：% 未被奇数个反斜杠转义时开始注释。 */
export function stripLatexComments(source: string): string {
  return source
    .split('\n')
    .map((line) => {
      for (let i = 0; i < line.length; i++) {
        if (line[i] !== '%') continue
        let backslashes = 0
        for (let j = i - 1; j >= 0 && line[j] === '\\'; j--) backslashes++
        if (backslashes % 2 === 0) return line.slice(0, i)
      }
      return line
    })
    .join('\n')
}

export function normalizePreflightPath(raw: string): string {
  let p = raw.trim().replace(/\\/g, '/').replace(/\/{2,}/g, '/')
  while (p.startsWith('./')) p = p.slice(2)
  p = p.replace(/^\/+/, '')
  return p
}

function includePath(raw: string): string {
  const p = normalizePreflightPath(raw)
  return /\.[a-z0-9]+$/i.test(p) ? p : `${p}.tex`
}

function lineAt(content: string, index: number): number {
  return content.slice(0, index).split('\n').length
}

function issue(
  rule: string,
  severity: PreflightSeverity,
  title: string,
  detail: string,
  filePath?: string,
  line?: number,
): PreflightIssue {
  return {
    id: `${rule}:${filePath ?? ''}:${line ?? 0}:${title}`,
    rule,
    severity,
    title,
    detail,
    filePath,
    line,
    source: 'rule',
  }
}

/** 对项目快照跑一遍确定性体检；不做任何语义判断。 */
export function runPreflight(input: PreflightInput): PreflightResult {
  const files = input.files.map((f) => ({ path: normalizePreflightPath(f.path), content: f.content }))
  const byPath = new Map(files.map((f) => [f.path, f]))
  const issues: PreflightIssue[] = []
  const warnings: string[] = []

  const roots = input.roots?.length
    ? input.roots.map(normalizePreflightPath)
    : files.filter((f) => DOCUMENTCLASS_RE.test(stripLatexComments(f.content))).map((f) => f.path).sort()
  const defaultRoot = roots[0]

  if (roots.length === 0) {
    issues.push(
      issue(
        'no-root',
        'blocking',
        '没有找到论文主文件',
        '项目中没有包含 \\documentclass 的 .tex 文件；请确认主文件已上传，或把它加入项目。',
      ),
    )
  } else if (roots.length > 1) {
    issues.push(
      issue('multiple-roots', 'suggestion', '存在多个主文件候选', `发现 ${roots.length} 个主文件，当前按 ${defaultRoot} 检查。`, defaultRoot),
    )
  }

  if (defaultRoot) {
    const rootContent = stripLatexComments(byPath.get(defaultRoot)?.content ?? '')
    if (!BEGIN_DOCUMENT_RE.test(rootContent) || !END_DOCUMENT_RE.test(rootContent)) {
      issues.push(
        issue(
          'document-structure',
          'blocking',
          '主文件缺少正文起止标记',
          `主文件 ${defaultRoot} 缺少 \\begin{document} 或 \\end{document}，编译会失败。`,
          defaultRoot,
        ),
      )
    }
  }

  // 包含关系：从主文件递归（无主文件时从全部 .tex 的声明里收集）。
  const included = new Set<string>()
  const visited = new Set<string>()
  const walk = (filePath: string, trail: string[]) => {
    if (visited.has(filePath)) return
    visited.add(filePath)
    const file = byPath.get(filePath)
    if (!file) return
    const content = stripLatexComments(file.content)
    for (const match of content.matchAll(INCLUDE_RE)) {
      const target = includePath(match[1])
      included.add(target)
      const line = lineAt(content, match.index ?? 0)
      if (!byPath.has(target)) {
        issues.push(
          issue('missing-include', 'blocking', '引用的章节文件不存在', `${filePath} 第 ${line} 行引用了 ${target}，但项目里没有这个文件。`, filePath, line),
        )
        continue
      }
      if (trail.includes(target)) {
        issues.push(issue('include-cycle', 'serious', '章节文件循环引用', `${filePath} 与 ${target} 相互引用，检查与编译都可能异常。`, filePath, line))
        continue
      }
      walk(target, [...trail, target])
    }
  }
  if (roots.length) {
    for (const root of roots) walk(root, [root])
  } else {
    for (const f of files) {
      if (!f.path.toLowerCase().endsWith('.tex')) continue
      for (const match of stripLatexComments(f.content).matchAll(INCLUDE_RE)) included.add(includePath(match[1]))
    }
  }

  // 标签与引用
  const labels = new Map<string, { filePath: string; line: number }[]>()
  for (const file of files) {
    if (!file.path.toLowerCase().endsWith('.tex')) continue
    const content = stripLatexComments(file.content)
    for (const match of content.matchAll(LABEL_RE)) {
      const list = labels.get(match[1]) ?? []
      list.push({ filePath: file.path, line: lineAt(content, match.index ?? 0) })
      labels.set(match[1], list)
    }
  }
  for (const [name, occurrences] of labels) {
    if (occurrences.length > 1) {
      issues.push(
        issue(
          'duplicate-label',
          'serious',
          '标签重复定义',
          `\\label{${name}} 在 ${occurrences.map((o) => `${o.filePath}:${o.line}`).join('、')} 重复出现，交叉引用会指向错误位置。`,
          occurrences[1].filePath,
          occurrences[1].line,
        ),
      )
    }
  }

  const referenced = new Set<string>()
  for (const file of files) {
    if (!file.path.toLowerCase().endsWith('.tex')) continue
    const content = stripLatexComments(file.content)
    for (const match of content.matchAll(REF_RE)) {
      for (const name of match[1].split(',').map((s) => s.trim()).filter(Boolean)) {
        referenced.add(name)
        if (!labels.has(name)) {
          issues.push(
            issue('dangling-ref', 'serious', '引用了不存在的标签', `\\ref{${name}} 找不到对应的 \\label，编译后会出现「??」。`, file.path, lineAt(content, match.index ?? 0)),
          )
        }
      }
    }
  }

  // 图表标签未被正文引用
  for (const file of files) {
    if (!file.path.toLowerCase().endsWith('.tex')) continue
    const content = stripLatexComments(file.content)
    for (const env of content.matchAll(ENVIRONMENT_RE)) {
      for (const label of (env[2] ?? '').matchAll(LABEL_RE)) {
        if (!referenced.has(label[1])) {
          const inner = env[0].indexOf(label[0])
          const offset = (env.index ?? 0) + (inner >= 0 ? inner : 0)
          issues.push(
            issue('unreferenced-float', 'suggestion', '图表没有被正文引用', `${file.path} 中的 ${label[1]} 没有在正文里被 \\ref 引用，答辩时容易被问「这个图表说明了什么」。`, file.path, lineAt(content, offset)),
          )
        }
      }
    }
  }

  // 文献键
  const bibKeys = new Set<string>()
  let hasBibSource = false
  for (const file of files) {
    const content = stripLatexComments(file.content)
    if (file.path.toLowerCase().endsWith('.bib')) {
      hasBibSource = true
      for (const match of content.matchAll(BIB_ENTRY_RE)) bibKeys.add(match[1])
    } else if (file.path.toLowerCase().endsWith('.tex')) {
      for (const match of content.matchAll(BIBITEM_RE)) {
        hasBibSource = true
        bibKeys.add(match[1])
      }
    }
  }
  if (!hasBibSource) {
    const citesWithoutBib = files.some((file) => file.path.toLowerCase().endsWith('.tex') && CITE_RE.test(stripLatexComments(file.content)))
    if (citesWithoutBib) {
      const first = files.find((file) => file.path.toLowerCase().endsWith('.tex'))
      issues.push(issue('no-bibliography', 'serious', '缺少参考文献入口', `${first?.path ?? '项目'} 使用了引用命令，但项目里没有 .bib 文件或文献表。`, first?.path))
    }
  } else {
    for (const file of files) {
      if (!file.path.toLowerCase().endsWith('.tex')) continue
      const content = stripLatexComments(file.content)
      for (const match of content.matchAll(CITE_RE)) {
        for (const key of match[1].split(',').map((s) => s.trim()).filter(Boolean)) {
          if (!bibKeys.has(key)) {
            issues.push(
              issue('dangling-cite', 'serious', '引用了不存在的文献键', `\\cite{${key}} 在参考文献里找不到，编译后会出现「?」。`, file.path, lineAt(content, match.index ?? 0)),
            )
          }
        }
      }
    }
  }

  // 占位符与待办
  for (const file of files) {
    if (!file.path.toLowerCase().endsWith('.tex')) continue
    const content = stripLatexComments(file.content)
    for (const match of content.matchAll(PLACEHOLDER_RE)) {
      issues.push(
        issue('todo-marker', 'suggestion', '残留待办标记', `${file.path} 里还有 ${match[0]}，交稿前请处理。`, file.path, lineAt(content, match.index ?? 0)),
      )
    }
    for (const placeholder of TEMPLATE_PLACEHOLDERS) {
      const index = content.indexOf(placeholder)
      if (index >= 0) {
        issues.push(
          issue('template-placeholder', 'suggestion', '残留模板占位文字', `${file.path} 里还有「${placeholder}」，请替换成真实内容。`, file.path, lineAt(content, index)),
        )
      }
    }
    if (!content.trim() && included.has(file.path)) {
      issues.push(issue('empty-included-file', 'suggestion', '章节文件为空', `${file.path} 被主文件引用，但内容为空。`, file.path))
    }
  }

  issues.sort((a, b) => (a.filePath ?? '').localeCompare(b.filePath ?? '') || (a.line ?? 0) - (b.line ?? 0))
  if (files.length === 0) warnings.push('项目里还没有 .tex 文件。')
  return { issues, roots, checkedFiles: files.length, warnings }
}
