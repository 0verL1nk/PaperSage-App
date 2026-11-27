import { describe, expect, it } from 'vitest'
import { normalizePreflightPath, runPreflight, stripLatexComments } from './preflight'

const text = String.raw

describe('runPreflight', () => {
  it('recognizes a multi-file thesis root and cross-file labels', () => {
    const result = runPreflight({
      files: [
        { path: 'thesis.tex', content: text`\documentclass{ctexart}
\begin{document}
\input{body/ch1}
\input{body/ch2}
\end{document}` },
        { path: 'body/ch1.tex', content: text`\chapter{绪论}
\label{sec:intro}
正文` },
        { path: 'body/ch2.tex', content: text`\chapter{方法}
参见第 \ref{sec:intro} 节。` },
      ],
    })
    expect(result.roots).toEqual(['thesis.tex'])
    expect(result.issues).toEqual([])
  })

  it('reports missing includes with file and line', () => {
    const result = runPreflight({
      files: [
        { path: 'thesis.tex', content: text`\documentclass{article}
\begin{document}
\input{body/missing}
\end{document}` },
      ],
    })
    const issue = result.issues.find((item) => item.rule === 'missing-include')
    expect(issue).toMatchObject({ severity: 'blocking', filePath: 'thesis.tex', line: 3, source: 'rule' })
  })

  it('ignores commands inside comments', () => {
    const result = runPreflight({
      files: [
        { path: 'main.tex', content: text`\documentclass{article}
\begin{document}
% \ref{missing} \cite{missing} TODO
正文
\end{document}` },
      ],
    })
    expect(result.issues).toEqual([])
  })

  it('reports dangling refs, duplicate labels and unreferenced figures', () => {
    const result = runPreflight({
      files: [
        { path: 'ch1.tex', content: text`\section{一}
\label{fig:a}
\begin{figure}\label{fig:b}\end{figure}
\ref{fig:missing}` },
        { path: 'ch2.tex', content: text`\section{二}
\label{fig:a}` },
      ],
    })
    const rules = result.issues.map((item) => item.rule)
    expect(rules).toContain('dangling-ref')
    expect(rules).toContain('duplicate-label')
    expect(rules).toContain('unreferenced-float')
    expect(result.issues.find((item) => item.rule === 'dangling-ref')?.detail).toContain('fig:missing')
  })

  it('checks citation keys against bib entries', () => {
    const result = runPreflight({
      files: [
        { path: 'main.tex', content: text`\documentclass{article}
\begin{document}
\cite{known,missing}
\end{document}` },
        { path: 'refs.bib', content: '@article{known, title={A}}' },
      ],
    })
    const dangling = result.issues.filter((item) => item.rule === 'dangling-cite')
    expect(dangling).toHaveLength(1)
    expect(dangling[0].detail).toContain('missing')
  })

  it('flags missing bibliography when citations exist', () => {
    const result = runPreflight({
      files: [{ path: 'main.tex', content: text`\documentclass{article}
\begin{document}
\cite{known}
\end{document}` }],
    })
    expect(result.issues.map((item) => item.rule)).toContain('no-bibliography')
  })

  it('flags missing root as blocking', () => {
    const result = runPreflight({ files: [{ path: 'chapter.tex', content: text`\section{没有主文件}` }] })
    expect(result.issues.find((item) => item.rule === 'no-root')).toMatchObject({ severity: 'blocking' })
  })

  it('flags TODO and leftover template text', () => {
    const result = runPreflight({
      files: [
        { path: 'main.tex', content: text`\documentclass{article}
\begin{document}
论文标题 TODO
\end{document}` },
      ],
    })
    const rules = result.issues.map((item) => item.rule)
    expect(rules).toContain('todo-marker')
    expect(rules).toContain('template-placeholder')
  })

  it('tags every issue as a deterministic rule', () => {
    const result = runPreflight({ files: [] })
    expect(result.warnings).toContain('项目里还没有 .tex 文件。')
    for (const item of result.issues) expect(item.source).toBe('rule')
  })
})

describe('stripLatexComments', () => {
  it('keeps escaped percent signs', () => {
    expect(stripLatexComments(text`50\% 不是注释`)).toBe(text`50\% 不是注释`)
    expect(stripLatexComments('正文 % 注释\n下一行')).toBe('正文 \n下一行')
  })
})

describe('normalizePreflightPath', () => {
  it('normalizes windows separators and leading ./', () => {
    expect(normalizePreflightPath('./body\\ch1.tex')).toBe('body/ch1.tex')
  })
})
