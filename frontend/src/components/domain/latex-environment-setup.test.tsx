import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { LatexEnvironmentSetup } from './latex-environment-setup'

const resourceCache = vi.hoisted(() => ({
  clearLatexResourceCache: vi.fn(),
  downloadLatexResource: vi.fn(),
  getBrowserStorageStatus: vi.fn(),
  getLatexResourceStatus: vi.fn(),
  pruneTexliveCacheExcept: vi.fn(),
  readLatexFormatSummary: vi.fn(),
  readTexliveCacheSummary: vi.fn(),
}))

vi.mock('@/lib/latex-resource-cache', () => resourceCache)

const resourceProfile = vi.hoisted(() => ({
  browserResourceOrigin: vi.fn(() => 'http://localhost:5478'),
  discoverResourceProfile: vi.fn(),
}))

vi.mock('@/lib/latex/resource-profile', () => resourceProfile)

const warmup = vi.hoisted(() => ({
  WARMUP_SAMPLES: [
    { id: 'xelatex', name: '中文论文环境（XeLaTeX）', compiler: 'xelatex', source: 'x' },
    { id: 'pdflatex', name: '英文论文环境（pdfLaTeX）', compiler: 'pdflatex', source: 'y' },
  ],
  warmupBrowserLatex: vi.fn(),
}))

vi.mock('@/lib/latex/warmup', () => warmup)

const TEXLIVE_SUMMARY = {
  entries: 3,
  bytes: 3 * 1024 * 1024,
  packages: 2,
  fonts: 1,
  byProfile: [
    { profile: 'tl-new', entries: 2, bytes: 2 * 1024 * 1024 },
    { profile: 'tl-old', entries: 1, bytes: 1024 * 1024 },
  ],
  topPackages: [{ package: 'ctex', entries: 2, bytes: 2 * 1024 * 1024 }],
  lastUsed: 1_700_000_000_000,
}

describe('LatexEnvironmentSetup', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resourceCache.getLatexResourceStatus.mockResolvedValue({ downloaded: 0, cached: false })
    resourceCache.downloadLatexResource.mockResolvedValue(undefined)
    resourceCache.clearLatexResourceCache.mockResolvedValue(undefined)
    resourceCache.getBrowserStorageStatus.mockResolvedValue({
      available: true, unavailableReason: null, persisted: true, usage: 8 * 1024 * 1024, quota: 1024 * 1024 * 1024,
    })
    resourceCache.pruneTexliveCacheExcept.mockResolvedValue({ entries: 1, bytes: 1024 * 1024 })
    resourceCache.readLatexFormatSummary.mockResolvedValue([])
    resourceCache.readTexliveCacheSummary.mockResolvedValue(TEXLIVE_SUMMARY)
    resourceProfile.discoverResourceProfile.mockResolvedValue('tl-new')
    warmup.warmupBrowserLatex.mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: {
        register: vi.fn().mockResolvedValue(undefined),
        ready: Promise.resolve({}),
      },
    })
  })

  it('只初始化用户选择的引擎资源包', async () => {
    render(<LatexEnvironmentSetup open onOpenChange={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /^pdfLaTeX/ }))
    expect(screen.getByText(/0\/7 项已验证/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '下载并准备' }))

    await waitFor(() => expect(resourceCache.downloadLatexResource).toHaveBeenCalledTimes(7))
    const downloadedIds = resourceCache.downloadLatexResource.mock.calls.map(([resource]) => resource.id)
    expect(downloadedIds).toEqual([
      'papersage-swift-worker',
      'swiftlatex-pdftex-js',
      'swiftlatex-pdftex-wasm',
      'wasmtex-bibtex-entry',
      'wasmtex-bibtex-worker',
      'wasmtex-bibtex-js',
      'wasmtex-bibtex-wasm',
    ])
  })

  it('展示 TeX Live 统计并可按版本清理旧缓存', async () => {
    render(<LatexEnvironmentSetup open onOpenChange={vi.fn()} />)

    expect(await screen.findByText('tl-new')).toBeInTheDocument()
    expect(screen.getByText('2 / 1')).toBeInTheDocument()
    expect(screen.getByText('1 个 · 1.0 MB')).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /清理其他版本/ }))
    fireEvent.click(await screen.findByRole('button', { name: '确认清理' }))

    await waitFor(() => expect(resourceCache.pruneTexliveCacheExcept).toHaveBeenCalledWith('tl-new'))
    expect(await screen.findByText(/已清理其他版本缓存：1 个文件/)).toBeInTheDocument()
  })

  it('存储不可用时说明原因并禁用预热与清理', async () => {
    resourceCache.getBrowserStorageStatus.mockResolvedValue({
      available: false,
      unavailableReason: '当前浏览器禁用了站点数据存储，编译资源无法缓存在本机',
      persisted: null,
      usage: 0,
      quota: 0,
    })

    render(<LatexEnvironmentSetup open onOpenChange={vi.fn()} />)

    expect(await screen.findByText('当前浏览器禁用了站点数据存储，编译资源无法缓存在本机')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /预热中文论文环境/ })).toBeDisabled()
    expect(screen.getByRole('button', { name: '清理全部缓存' })).toBeDisabled()
  })

  it('预热走真实样例编译并显示进度', async () => {
    let finish: () => void = () => undefined
    warmup.warmupBrowserLatex.mockImplementation((
      _sample: unknown,
      options: { onProgress?: (progress: { stage: string; message: string }) => void },
    ) => new Promise<void>((resolve) => {
      options.onProgress?.({ stage: 'compile', message: '正在编译（第 1/5 遍）' })
      finish = resolve
    }))

    render(<LatexEnvironmentSetup open onOpenChange={vi.fn()} />)
    fireEvent.click(await screen.findByRole('button', { name: /预热中文论文环境/ }))

    expect(await screen.findByText('正在编译（第 1/5 遍）')).toBeInTheDocument()
    expect(screen.getByText(/已加载 0 个资源 · 已用 0 秒/)).toBeInTheDocument()
    expect(resourceCache.downloadLatexResource).toHaveBeenCalled()
    expect(warmup.warmupBrowserLatex).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'xelatex', compiler: 'xelatex' }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )

    finish()
    expect(await screen.findByText(/预热完成/)).toBeInTheDocument()
  })

  it('取消预热后立即恢复缓存操作并给出提示', async () => {
    warmup.warmupBrowserLatex.mockImplementation((
      _sample: unknown,
      options: { signal?: AbortSignal },
    ) => new Promise<void>((_resolve, reject) => {
      const abort = () => reject(new DOMException('编译已取消', 'AbortError'))
      if (options.signal?.aborted) {
        abort()
        return
      }
      options.signal?.addEventListener('abort', abort, { once: true })
    }))

    render(<LatexEnvironmentSetup open onOpenChange={vi.fn()} />)
    fireEvent.click(await screen.findByRole('button', { name: /预热中文论文环境/ }))
    fireEvent.click(await screen.findByRole('button', { name: /取消预热/ }))

    expect(await screen.findByText(/已取消预热/)).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('button', { name: '清理全部缓存' })).toBeEnabled())
  })
})
