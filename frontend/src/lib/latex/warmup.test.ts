import { beforeEach, describe, expect, it, vi } from 'vitest'
import { WARMUP_SAMPLES, warmupBrowserLatex } from './warmup'

const compiler = vi.hoisted(() => ({ compileLatexInBrowser: vi.fn() }))
vi.mock('@/lib/latex/swiftlatex-browser-compiler', () => compiler)

describe('warmupBrowserLatex', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    compiler.compileLatexInBrowser.mockResolvedValue({ pdf: new Uint8Array([1]), log: '' })
  })

  it('两套样例分别用 XeLaTeX 与 pdfLaTeX 编译 main.tex', async () => {
    expect(WARMUP_SAMPLES.map((sample) => sample.compiler)).toEqual(['xelatex', 'pdflatex'])

    for (const sample of WARMUP_SAMPLES) {
      await warmupBrowserLatex(sample)
      const [files, rootFile, engine] = compiler.compileLatexInBrowser.mock.calls.at(-1)!
      expect(files).toEqual([{ path: 'main.tex', content: sample.source }])
      expect(rootFile).toBe('main.tex')
      expect(engine).toBe(sample.compiler)
    }
    expect(compiler.compileLatexInBrowser).toHaveBeenCalledTimes(2)
  })

  it('样例覆盖中文与常用宏包，并透传进度与取消信号', async () => {
    const controller = new AbortController()
    const onProgress = vi.fn()

    await warmupBrowserLatex(WARMUP_SAMPLES[0], { signal: controller.signal, onProgress })

    const [, , , options] = compiler.compileLatexInBrowser.mock.calls[0]
    expect(options).toEqual({ signal: controller.signal, onProgress })
    expect(WARMUP_SAMPLES[0].source).toContain('ctexart')
    expect(WARMUP_SAMPLES[0].source).toContain('中文排版预热')
  })
})
