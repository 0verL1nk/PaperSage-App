import { describe, expect, it } from 'vitest'
import { evaluateReviewChecks } from './review-checks'

const file = (file_path: string) => ({ id: file_path, file_path, content_type: 'text/x-tex', size_bytes: 0, last_saved_at: '' })

describe('evaluateReviewChecks', () => {
  it('reports a missing main file as an action', () => {
    expect(evaluateReviewChecks([], undefined)[0]).toMatchObject({ id: 'main-file', status: 'action' })
  })

  it('finds structure, leftover template text, missing references and missing included chapters', () => {
    const checks = evaluateReviewChecks([file('main.tex')], String.raw`\documentclass{ctexart}
\begin{document}
\title{论文标题} \cite{smith2025} \input{chapters/method}
从这里开始撰写。
\end{document}`)
    expect(checks.find((check) => check.id === 'document-structure')?.status).toBe('pass')
    expect(checks.find((check) => check.id === 'template-placeholders')?.status).toBe('warning')
    expect(checks.find((check) => check.id === 'references')?.status).toBe('warning')
    expect(checks.find((check) => check.id === 'included-files')?.detail).toContain('chapters/method')
  })

  it('recognizes existing bibliography and chapter files', () => {
    const checks = evaluateReviewChecks([file('main.tex'), file('refs.bib'), file('chapters/method.tex')], String.raw`\documentclass{article}
\begin{document}
\cite{smith2025} \bibliography{refs} \input{chapters/method}
\end{document}`)
    expect(checks.find((check) => check.id === 'references')?.status).toBe('pass')
    expect(checks.find((check) => check.id === 'included-files')?.status).toBe('pass')
  })

  it('flags a declared bibliography file that is absent from the project', () => {
    const checks = evaluateReviewChecks([file('main.tex')], String.raw`\documentclass{article}
\begin{document}
\cite{smith2025} \addbibresource{references/library.bib}
\end{document}`)
    expect(checks.find((check) => check.id === 'references')?.detail).toContain('references/library.bib')
  })
})
