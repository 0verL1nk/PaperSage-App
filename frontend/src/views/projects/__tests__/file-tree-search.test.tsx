import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { FileTreeSearch } from '../file-tree-search'

describe('FileTreeSearch', () => {
  it('renders search input', () => {
    render(<FileTreeSearch value="" onChange={() => {}} />)
    expect(screen.getByPlaceholderText('搜索文件...')).toBeInTheDocument()
  })

  it('displays current value', () => {
    render(<FileTreeSearch value="main" onChange={() => {}} />)
    expect(screen.getByDisplayValue('main')).toBeInTheDocument()
  })

  it('calls onChange when typing', () => {
    const onChange = vi.fn()
    render(<FileTreeSearch value="" onChange={onChange} />)
    fireEvent.change(screen.getByPlaceholderText('搜索文件...'), {
      target: { value: 'intro' },
    })
    expect(onChange).toHaveBeenCalledWith('intro')
  })

  it('shows clear button when value is non-empty', () => {
    render(<FileTreeSearch value="test" onChange={() => {}} />)
    const clearBtn = screen.getByRole('button')
    expect(clearBtn).toBeInTheDocument()
  })

  it('hides clear button when value is empty', () => {
    render(<FileTreeSearch value="" onChange={() => {}} />)
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('clears value on clear button click', () => {
    const onChange = vi.fn()
    render(<FileTreeSearch value="test" onChange={onChange} />)
    fireEvent.click(screen.getByRole('button'))
    expect(onChange).toHaveBeenCalledWith('')
  })
})
