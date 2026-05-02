import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import { InlineInput } from '../file-tree-node'

describe('InlineInput', () => {
  it('submits on Enter with trimmed value', () => {
    const onSubmit = vi.fn()
    render(<InlineInput onSubmit={onSubmit} onCancel={() => {}} defaultValue="test" />)
    const input = document.querySelector('input')!
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onSubmit).toHaveBeenCalledWith('test')
  })

  it('cancels on empty Enter', () => {
    const onCancel = vi.fn()
    render(<InlineInput onSubmit={() => {}} onCancel={onCancel} defaultValue="" />)
    const input = document.querySelector('input')!
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onCancel).toHaveBeenCalled()
  })

  it('cancels on Escape', () => {
    const onCancel = vi.fn()
    render(<InlineInput onSubmit={() => {}} onCancel={onCancel} defaultValue="x" />)
    const input = document.querySelector('input')!
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(onCancel).toHaveBeenCalled()
  })

  it('cancels on blur (does NOT submit)', () => {
    const onCancel = vi.fn()
    const onSubmit = vi.fn()
    render(<InlineInput onSubmit={onSubmit} onCancel={onCancel} defaultValue="name" />)
    const input = document.querySelector('input')!
    fireEvent.blur(input)
    expect(onCancel).toHaveBeenCalled()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('uses defaultValue', () => {
    render(<InlineInput onSubmit={() => {}} onCancel={() => {}} defaultValue="old_name.tex" />)
    const input = document.querySelector('input') as HTMLInputElement
    expect(input.value).toBe('old_name.tex')
  })

  it('trims whitespace before submit', () => {
    const onSubmit = vi.fn()
    render(<InlineInput onSubmit={onSubmit} onCancel={() => {}} defaultValue="  spaced  " />)
    const input = document.querySelector('input')!
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onSubmit).toHaveBeenCalledWith('spaced')
  })
})
