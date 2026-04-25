import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { WritingFileTree } from '../writing-file-tree'
import type { LatexFileItem } from '@/services/latex-api'

const makeFile = (filePath: string): LatexFileItem => ({
  id: filePath,
  file_path: filePath,
  content_type: 'text/x-tex',
  size_bytes: 100,
  last_saved_at: '2024-01-01',
})

describe('WritingFileTree', () => {
  it('renders file items', () => {
    const files = [makeFile('main.tex'), makeFile('refs.bib')]

    render(
      <WritingFileTree
        files={files}
        activeFilePath="main.tex"
        onSelect={() => {}}
        onDeleteFile={() => {}}
      />,
    )

    expect(screen.getByText('main.tex')).toBeInTheDocument()
    expect(screen.getByText('refs.bib')).toBeInTheDocument()
  })

  it('renders directory groups', () => {
    const files = [makeFile('sections/intro.tex'), makeFile('sections/method.tex')]

    render(
      <WritingFileTree
        files={files}
        activeFilePath={null}
        onSelect={() => {}}
        onDeleteFile={() => {}}
      />,
    )

    expect(screen.getByText('sections')).toBeInTheDocument()
    expect(screen.getByText('intro.tex')).toBeInTheDocument()
    expect(screen.getByText('method.tex')).toBeInTheDocument()
  })

  it('calls onSelect when file is clicked', () => {
    const files = [makeFile('main.tex')]
    const onSelect = vi.fn()

    render(
      <WritingFileTree
        files={files}
        activeFilePath={null}
        onSelect={onSelect}
        onDeleteFile={() => {}}
      />,
    )

    fireEvent.click(screen.getByText('main.tex'))
    expect(onSelect).toHaveBeenCalledWith('main.tex')
  })

  it('shows empty state when no files', () => {
    render(
      <WritingFileTree
        files={[]}
        activeFilePath={null}
        onSelect={() => {}}
        onDeleteFile={() => {}}
      />,
    )

    expect(screen.getByText('暂无文件')).toBeInTheDocument()
  })

  it('highlights active file', () => {
    const files = [makeFile('main.tex'), makeFile('other.tex')]

    render(
      <WritingFileTree
        files={files}
        activeFilePath="main.tex"
        onSelect={() => {}}
        onDeleteFile={() => {}}
      />,
    )

    const activeItem = screen.getByText('main.tex').closest('div')
    expect(activeItem?.className).toContain('bg-primary')
  })

  it('toggles folder on click', () => {
    const files = [makeFile('sections/intro.tex')]

    render(
      <WritingFileTree
        files={files}
        activeFilePath={null}
        onSelect={() => {}}
        onDeleteFile={() => {}}
      />,
    )

    // File should be visible initially (folder is open by default)
    expect(screen.getByText('intro.tex')).toBeInTheDocument()

    // Click folder to close
    fireEvent.click(screen.getByText('sections'))

    // File should still be in DOM since we use state toggle
    // After clicking, folder is closed — children not rendered
    expect(screen.queryByText('intro.tex')).not.toBeInTheDocument()
  })
})
