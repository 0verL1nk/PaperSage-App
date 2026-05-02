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

const noop = {
  onSelect: () => {},
  onDeleteFile: () => {},
  onCreateFile: async () => makeFile('dummy'),
  onRenameFile: async () => makeFile('dummy'),
  onMoveFile: async () => makeFile('dummy'),
  onExternalDrop: undefined as ((files: FileList) => void) | undefined,
}

function renderTree(overrides: Partial<typeof noop> = {}, files: LatexFileItem[], activeFilePath: string | null = null) {
  const props = { ...noop, ...overrides }
  return render(
    <div style={{ height: 300 }}>
      <WritingFileTree
        files={files}
        activeFilePath={activeFilePath}
        onSelect={props.onSelect}
        onDeleteFile={props.onDeleteFile}
        onCreateFile={props.onCreateFile}
        onRenameFile={props.onRenameFile}
        onMoveFile={props.onMoveFile}
        onExternalDrop={props.onExternalDrop}
      />
    </div>,
  )
}

describe('WritingFileTree', () => {
  it('renders file items', () => {
    renderTree({}, [makeFile('main.tex'), makeFile('refs.bib')])

    expect(screen.getByText('main.tex')).toBeInTheDocument()
    expect(screen.getByText('refs.bib')).toBeInTheDocument()
  })

  it('renders directory groups', () => {
    renderTree({}, [makeFile('sections/intro.tex'), makeFile('sections/method.tex')])

    expect(screen.getByText('sections')).toBeInTheDocument()
    expect(screen.getByText('intro.tex')).toBeInTheDocument()
    expect(screen.getByText('method.tex')).toBeInTheDocument()
  })

  it('calls onSelect when file is activated', () => {
    const onSelect = vi.fn()
    renderTree({ onSelect }, [makeFile('main.tex')])

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
        onCreateFile={async () => makeFile('x')}
        onRenameFile={async () => makeFile('x')}
        onMoveFile={async () => makeFile('x')}
        onExternalDrop={undefined}
      />,
    )

    expect(screen.getByText('暂无文件')).toBeInTheDocument()
  })

  it('highlights active file', () => {
    renderTree({}, [makeFile('main.tex'), makeFile('other.tex')], 'main.tex')

    const activeItem = screen.getByText('main.tex').closest('div')
    expect(activeItem?.className).toContain('bg-primary')
  })

  it('toggles folder on click', () => {
    renderTree({}, [makeFile('sections/intro.tex')])

    expect(screen.getByText('intro.tex')).toBeInTheDocument()
    fireEvent.click(screen.getByText('sections'))
    expect(screen.queryByText('intro.tex')).not.toBeInTheDocument()
  })

  // --- .gitkeep folder tests ---

  it('shows folder when only .gitkeep exists inside', () => {
    renderTree({}, [makeFile('images/.gitkeep')])

    expect(screen.getByText('images')).toBeInTheDocument()
    expect(screen.queryByText('.gitkeep')).not.toBeInTheDocument()
  })

  it('shows folder with .gitkeep alongside real files', () => {
    renderTree({}, [makeFile('images/.gitkeep'), makeFile('images/photo.png')])

    expect(screen.getByText('images')).toBeInTheDocument()
    expect(screen.getByText('photo.png')).toBeInTheDocument()
    expect(screen.queryByText('.gitkeep')).not.toBeInTheDocument()
  })

  it('shows nested folder from .gitkeep only', () => {
    renderTree({}, [makeFile('assets/figures/.gitkeep')])

    expect(screen.getByText('assets')).toBeInTheDocument()
    expect(screen.getByText('figures')).toBeInTheDocument()
    expect(screen.queryByText('.gitkeep')).not.toBeInTheDocument()
  })

  it('sorts folders before files', () => {
    renderTree({}, [makeFile('zeta.tex'), makeFile('alpha/.gitkeep'), makeFile('beta.tex')])

    const folderEl = screen.getByText('alpha')
    const fileEl = screen.getByText('zeta.tex')
    expect(folderEl).toBeInTheDocument()
    expect(fileEl).toBeInTheDocument()
  })

  // --- Create/Rename/Move handler tests ---

  it('accepts all required props without errors', () => {
    const onCreateFile = vi.fn().mockResolvedValue(makeFile('x'))
    const onRenameFile = vi.fn().mockResolvedValue(makeFile('x'))
    const onMoveFile = vi.fn().mockResolvedValue(makeFile('x'))
    const onExternalDrop = vi.fn()

    const { container } = render(
      <div style={{ height: 300 }}>
        <WritingFileTree
          files={[makeFile('main.tex')]}
          activeFilePath={null}
          onSelect={() => {}}
          onDeleteFile={() => {}}
          onCreateFile={onCreateFile}
          onRenameFile={onRenameFile}
          onMoveFile={onMoveFile}
          onExternalDrop={onExternalDrop}
        />
      </div>,
    )

    expect(container).toBeInTheDocument()
    expect(screen.getByText('main.tex')).toBeInTheDocument()
  })
})
