import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { LatexEnvironmentSetup } from './latex-environment-setup'

const resourceCache = vi.hoisted(() => ({
  clearLatexResourceCache: vi.fn(),
  downloadLatexResource: vi.fn(),
  getLatexResourceStatus: vi.fn(),
}))

vi.mock('@/lib/latex-resource-cache', () => resourceCache)

describe('LatexEnvironmentSetup', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    resourceCache.getLatexResourceStatus.mockResolvedValue({ downloaded: 0, cached: false })
    resourceCache.downloadLatexResource.mockResolvedValue(undefined)
    resourceCache.clearLatexResourceCache.mockResolvedValue(undefined)
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

    fireEvent.click(screen.getByRole('button', { name: /pdfLaTeX/ }))
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
})
