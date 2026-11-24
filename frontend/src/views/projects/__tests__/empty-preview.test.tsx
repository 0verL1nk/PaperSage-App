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
})
