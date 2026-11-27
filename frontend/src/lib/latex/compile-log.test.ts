import { describe, expect, it } from 'vitest'

import { logExcerptForIssue, normalizeLogPath, parseLatexLog } from './compile-log'

const FILES = ['main.tex', 'chapters/intro.tex', 'chapters/method.tex', 'refs.bib', 'custom.sty']

function parse(log: string, extra: Partial<Parameters<typeof parseLatexLog>[0]> = {}) {
  return parseLatexLog({ log, projectFiles: FILES, rootFile: 'main.tex', ...extra })
}

describe('parseLatexLog', () => {
  it('parses bang errors with l.N and attributes them to the innermost open file', () => {
    const issues = parse(`--- pdflatex pass 2 ---
(./main.tex
(./chapters/intro.tex
! Undefined control sequence.
l.42 \\badcommand
)
)
`)
    expect(issues).toHaveLength(1)
    expect(issues[0]).toMatchObject({
      severity: 'error',
      filePath: 'chapters/intro.tex',
      line: 42,
      message: 'Undefined control sequence.',
      source: 'latex',
    })
  })

  it('parses file:line: records and normalizes ./ prefixes', () => {
    const issues = parse('(./main.tex\n./chapters/method.tex:12: LaTeX Error: Broken.\n)')
    expect(issues[0]).toMatchObject({ filePath: 'chapters/method.tex', line: 12, message: 'LaTeX Error: Broken.' })
  })

  it('returns to the outer file after a nested scope closes', () => {
    const issues = parse(`(./main.tex
(./chapters/intro.tex
)
! Missing $ inserted.
l.7 x
`)
    expect(issues[0]).toMatchObject({ filePath: 'main.tex', line: 7 })
  })

  it('keeps file-level errors without a line number', () => {
    const issues = parse(`(./main.tex
! LaTeX Error: File \`missing.sty' not found.
`)
    expect(issues[0]).toMatchObject({ filePath: 'main.tex', line: null })
    expect(issues[0].message).toContain("File `missing.sty' not found")
  })

  it('falls back to the root file only when the log has no file scopes at all', () => {
    const issues = parse('! Emergency stop.\nl.3 \\foo\n')
    expect(issues[0]).toMatchObject({ filePath: 'main.tex', line: 3 })
  })

  it('does not guess a file when a scope exists but the error is outside it', () => {
    const issues = parse('(./chapters/intro.tex\n)\n! Emergency stop.\nl.3 x\n')
    expect(issues[0].filePath).toBeNull()
  })

  it('parses warnings with on input line and multi-line continuations', () => {
    const issues = parse(`(./chapters/intro.tex
LaTeX Warning: Reference \`fig:1' undefined on input line 23.
Package hyperref Warning: Token not allowed in a PDF string (Unicode):
(hyperref)                removing \`\\text' on input line 42.
)
`)
    expect(issues).toHaveLength(2)
    expect(issues[0]).toMatchObject({ severity: 'warning', filePath: 'chapters/intro.tex', line: 23 })
    expect(issues[1]).toMatchObject({ severity: 'warning', line: 42 })
    expect(issues[1].message).toContain('removing')
  })

  it('ignores Overfull/Underfull boxes', () => {
    expect(parse('Overfull \\hbox (12.3pt too wide) in paragraph at lines 1--2\n')).toEqual([])
  })

  it('uses only the last error-bearing pass and the last pass for warnings', () => {
    const issues = parse(`--- pdflatex pass 1 ---
! Undefined control sequence.
l.10 \\a
LaTeX Warning: Reference \`x' undefined on input line 4.
--- pdflatex pass 2 ---
! Undefined control sequence.
l.11 \\b
LaTeX Warning: Reference \`y' undefined on input line 8.
`)
    expect(issues.filter((issue) => issue.severity === 'error')).toHaveLength(1)
    expect(issues.find((issue) => issue.severity === 'error')?.line).toBe(11)
    expect(issues.filter((issue) => issue.severity === 'warning')).toHaveLength(1)
    expect(issues.find((issue) => issue.severity === 'warning')?.line).toBe(8)
  })

  it('parses BibTeX failures as non-navigable errors', () => {
    const issues = parse(`--- BibTeX main ---
I couldn't open database file refs.bib
---line 12 of file main.aux
`)
    expect(issues).toHaveLength(1)
    expect(issues[0]).toMatchObject({ severity: 'error', filePath: null, line: 12, source: 'bibtex' })
    expect(issues[0].message).toContain("I couldn't open database file refs.bib")
  })

  it('orders errors before warnings', () => {
    const issues = parse(`(./main.tex
LaTeX Warning: Something odd on input line 1.
! Real failure.
l.2 x
`)
    expect(issues.map((issue) => issue.severity)).toEqual(['error', 'warning'])
  })

  it('caps issues at 200', () => {
    const lines = ['(./main.tex']
    for (let i = 1; i <= 260; i++) lines.push(`! Failure ${i}.`, `l.${i} x`)
    const issues = parse(lines.join('\n'))
    expect(issues).toHaveLength(200)
  })

  it('keeps the tail of oversized logs so errors near the end survive', () => {
    const log = `${'x'.repeat(2 * 1024 * 1024)}!\n! Emergency stop.\nl.9 x`
    expect(parse(log).some((issue) => issue.message === 'Emergency stop.')).toBe(true)
  })

  it('returns no issues for empty or non-error logs', () => {
    expect(parse('')).toEqual([])
    expect(parse('(./main.tex\nThis is pdfTeX.\n)\n')).toEqual([])
  })
})

describe('normalizeLogPath', () => {
  it('normalizes ./ and /work/ prefixes and backslashes', () => {
    expect(normalizeLogPath('./chapters/intro.tex', FILES)).toBe('chapters/intro.tex')
    expect(normalizeLogPath('/work/chapters/intro.tex', FILES)).toBe('chapters/intro.tex')
    expect(normalizeLogPath('.\\chapters\\intro.tex', FILES)).toBe('chapters/intro.tex')
  })

  it('matches a unique basename and refuses ambiguous ones', () => {
    expect(normalizeLogPath('/tmp/build/intro.tex', FILES)).toBe('chapters/intro.tex')
    expect(normalizeLogPath('custom.sty', FILES)).toBe('custom.sty')
    expect(normalizeLogPath('unknown.tex', FILES)).toBeNull()
    expect(normalizeLogPath('', FILES)).toBeNull()
  })
})

describe('logExcerptForIssue', () => {
  it('returns the whole log when it fits', () => {
    expect(logExcerptForIssue('abc', { severity: 'error', filePath: null, line: null, message: 'x', source: 'latex' })).toBe('abc')
  })

  it('centers the window on the error anchor', () => {
    const log = `${'a'.repeat(50_000)}l.42 bad${'b'.repeat(50_000)}`
    const excerpt = logExcerptForIssue(
      log,
      { severity: 'error', filePath: null, line: 42, message: 'bad', source: 'latex' },
      1000,
    )
    expect(excerpt).toHaveLength(1000)
    expect(excerpt).toContain('l.42 bad')
  })
})
