import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { EmptyPreview } from '../empty-preview'

describe('EmptyPreview', () => {
  it('runs a real compile from the empty preview instead of showing a dead button', () => {
    const onCompile = vi.fn()
    render(<EmptyPreview onCompile={onCompile} disabled={false} />)

    fireEvent.click(screen.getByRole('button', { name: '编译当前项目' }))

    expect(onCompile).toHaveBeenCalledTimes(1)
  })

  it('disables the compile action when compiling or the compiler is unsupported', () => {
    render(<EmptyPreview onCompile={vi.fn()} disabled />)

    expect(screen.getByRole('button', { name: '编译当前项目' })).toBeDisabled()
  })

  it('sets a realistic first-compile expectation instead of promising tens of seconds', () => {
    render(<EmptyPreview onCompile={vi.fn()} disabled={false} />)

    expect(screen.getByText(/1–3 分钟/)).toBeInTheDocument()
    expect(screen.getByText(/几秒/)).toBeInTheDocument()
    expect(screen.queryByText(/几十秒/)).not.toBeInTheDocument()
  })
})
