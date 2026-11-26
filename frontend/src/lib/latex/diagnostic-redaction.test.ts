import { describe, expect, it } from 'vitest'

import { redactDiagnosticLog, redactDiagnosticMessage } from './diagnostic-redaction'

// 与 server/internal/compilediag/redact_test.go 同一组隐私断言：
// 源码回显、文件展开、论文正文一律不得出现在上报片段里。
describe('diagnostic redaction', () => {
  it('keeps only TeX diagnostic lines', () => {
    const raw = [
      'This is XeTeX, Version 3.141592653-2.6-0.999996 (TeX Live 2026)',
      '(./chapters/intro.tex',
      '! Undefined control sequence.',
      'l.42 \\secretCommandWithPaperText',
      '                    ^^',
      'LaTeX Warning: Reference `fig:1\' on page 1 undefined on input line 12.',
      'Package hyperref Warning: Token not allowed in a PDF string.',
      'Overfull \\hbox (12.3pt too wide) in paragraph at lines 5--6',
      '我的论文正文这一行绝对不能上传',
      'Output written on main.pdf (3 pages, 12345 bytes).',
    ].join('\n')

    const got = redactDiagnosticLog(raw)

    expect(got).toContain('! Undefined control sequence.')
    expect(got).toContain('l.42')
    expect(got).toContain('LaTeX Warning')
    expect(got).toContain('Package hyperref Warning')
    expect(got).toContain('Overfull')
    expect(got).toContain('This is XeTeX')
    expect(got).toContain('Output written')
    expect(got).not.toContain('secretCommandWithPaperText')
    expect(got).not.toContain('我的论文正文')
    expect(got).not.toContain('chapters/intro.tex')
    expect(got).not.toContain('^^')
  })

  it('replaces known project paths and strips ANSI escapes', () => {
    const got = redactDiagnosticLog('\x1b[31m! Emergency stop.\x1b[0m\n! in main.tex', ['main.tex'])
    expect(got).not.toContain('\x1b[')
    expect(got).toContain('! Emergency stop.')
    expect(got).toContain('<project-file>')
  })

  it('caps the excerpt at 32 KiB on line boundaries', () => {
    const got = redactDiagnosticLog('! Emergency stop.\n' + '! another error line\n'.repeat(5000))
    expect(got.startsWith('! Emergency stop.')).toBe(true)
    expect(new TextEncoder().encode(got).length).toBeLessThanOrEqual(32 * 1024)
  })

  it('prefers the first bang line for latex error summaries', () => {
    expect(redactDiagnosticMessage('latex', '编译失败\n! Undefined control sequence.\nl.42 \\x'))
      .toBe('! Undefined control sequence.')
    expect(redactDiagnosticMessage('resource', '资源加载失败')).toBe('资源加载失败')
  })
})
