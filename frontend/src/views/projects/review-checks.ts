import type { LatexFileItem } from '@/services/latex-api'

export type ReviewCheckStatus = 'pass' | 'warning' | 'action'

export type ReviewCheck = {
  id: string
  title: string
  status: ReviewCheckStatus
  detail: string
}

export function evaluateReviewChecks(files: LatexFileItem[], mainContent: string | undefined): ReviewCheck[] {
  const mainFile = files.find((file) => file.file_path === 'main.tex')
  const checks: ReviewCheck[] = [
    {
      id: 'main-file',
      title: '论文主文件',
      status: mainFile ? 'pass' : 'action',
      detail: mainFile ? '已找到 main.tex，可以继续检查文档结构。' : '项目中没有 main.tex，请到论文撰写页创建主文件。',
    },
  ]
  if (!mainFile || mainContent === undefined) return checks

  const hasClass = /\\documentclass(?:\[[^\]]*\])?\s*\{[^}]+\}/.test(mainContent)
  const hasBegin = /\\begin\s*\{document\}/.test(mainContent)
  const hasEnd = /\\end\s*\{document\}/.test(mainContent)
  checks.push({
    id: 'document-structure',
    title: 'LaTeX 文档结构',
    status: hasClass && hasBegin && hasEnd ? 'pass' : 'action',
    detail: hasClass && hasBegin && hasEnd
      ? '文档类、正文起止标记均已找到。'
      : `缺少${[!hasClass && '\\documentclass', !hasBegin && '\\begin{document}', !hasEnd && '\\end{document}'].filter(Boolean).join('、')}。`,
  })

  const placeholders = ['论文标题', '作者姓名', '从这里开始撰写'].filter((placeholder) => mainContent.includes(placeholder))
  checks.push({
    id: 'template-placeholders',
    title: '模板占位文字',
    status: placeholders.length ? 'warning' : 'pass',
    detail: placeholders.length ? `仍有默认内容：${placeholders.join('、')}。` : '没有发现默认模板中的标题、作者或正文提示。',
  })

  const citations = [...mainContent.matchAll(/\\(?:cite|parencite|textcite|autocite)(?:\[[^\]]*\]){0,2}\s*\{([^}]+)\}/g)]
  const bibFiles = new Set(files.filter((file) => file.file_path.toLowerCase().endsWith('.bib')).map((file) => file.file_path.replace(/\\/g, '/').toLowerCase()))
  const resourceNames = [...mainContent.matchAll(/\\(?:bibliography|addbibresource)\s*\{([^}]+)\}/g)]
    .flatMap((match) => match[1].split(',').map((resource) => resource.trim().replace(/\\/g, '/').toLowerCase()))
    .filter(Boolean)
  const missingBibFiles = resourceNames.filter((resource) => {
    const path = resource.endsWith('.bib') ? resource : `${resource}.bib`
    return !bibFiles.has(path)
  })
  const hasBibliography = resourceNames.length > 0 || /\\begin\s*\{thebibliography\}/.test(mainContent)
  const referenceConfigured = hasBibliography || bibFiles.size > 0
  checks.push({
    id: 'references',
    title: '参考文献入口',
    status: (citations.length && !referenceConfigured) || missingBibFiles.length ? 'warning' : 'pass',
    detail: missingBibFiles.length
      ? `找不到参考文献文件：${missingBibFiles.join('、')}。`
      : citations.length && !referenceConfigured
        ? '主文件包含引用命令，但没有找到参考文献声明或 .bib 文件。'
        : citations.length ? '已发现引用命令和参考文献入口；请继续核对文献来源与引用键。' : '暂未发现引用命令；如果需要引用文献，请补充引用及参考文献表。',
  })

  const texFiles = new Set(files.filter((file) => file.file_path.toLowerCase().endsWith('.tex')).map((file) => file.file_path.replace(/\\/g, '/')))
  const missingInputs = [...mainContent.matchAll(/\\(?:input|include)\s*\{([^}]+)\}/g)]
    .map((match) => match[1].replace(/\\/g, '/').replace(/\.tex$/i, ''))
    .filter((input) => !texFiles.has(input.endsWith('.tex') ? input : `${input}.tex`))
  checks.push({
    id: 'included-files',
    title: '章节文件引用',
    status: missingInputs.length ? 'action' : 'pass',
    detail: missingInputs.length ? `找不到被引用的章节文件：${missingInputs.join('、')}。` : '主文件中的 input/include 目标均可在文件树中找到。',
  })
  return checks
}
