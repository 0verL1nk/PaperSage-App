import { describe, it, expect } from 'vitest'
import { getFileTypeIcon, FileTypeIcon } from '../file-tree-icons'
import { FileText, FileCode, FileImage, File, BookOpen } from 'lucide-react'
import { render } from '@testing-library/react'

describe('getFileTypeIcon', () => {
  it('returns FileText + primary for .tex', () => {
    const def = getFileTypeIcon('main.tex')
    expect(def.icon).toBe(FileText)
    expect(def.className).toBe('text-primary')
  })

  it('returns BookOpen + amber for .bib', () => {
    const def = getFileTypeIcon('refs.bib')
    expect(def.icon).toBe(BookOpen)
    expect(def.className).toBe('text-amber-500')
  })

  it('returns FileCode + purple for .cls', () => {
    const def = getFileTypeIcon('template.cls')
    expect(def.icon).toBe(FileCode)
    expect(def.className).toBe('text-purple-500')
  })

  it('returns FileCode + indigo for .sty', () => {
    const def = getFileTypeIcon('package.sty')
    expect(def.icon).toBe(FileCode)
    expect(def.className).toBe('text-indigo-500')
  })

  it('returns FileCode + pink for .bst', () => {
    const def = getFileTypeIcon('style.bst')
    expect(def.icon).toBe(FileCode)
    expect(def.className).toBe('text-pink-500')
  })

  it('returns FileCode + cyan for .tikz', () => {
    const def = getFileTypeIcon('diagram.tikz')
    expect(def.icon).toBe(FileCode)
    expect(def.className).toBe('text-cyan-500')
  })

  it('returns FileImage + green for .png', () => {
    const def = getFileTypeIcon('photo.png')
    expect(def.icon).toBe(FileImage)
    expect(def.className).toBe('text-green-500')
  })

  it('returns FileImage + green for .jpg', () => {
    const def = getFileTypeIcon('photo.jpg')
    expect(def.icon).toBe(FileImage)
    expect(def.className).toBe('text-green-500')
  })

  it('returns FileText + red for .pdf', () => {
    const def = getFileTypeIcon('output.pdf')
    expect(def.icon).toBe(FileText)
    expect(def.className).toBe('text-red-500')
  })

  it('returns File + muted-foreground for unknown extension', () => {
    const def = getFileTypeIcon('data.json')
    expect(def.icon).toBe(File)
    expect(def.className).toBe('text-muted-foreground')
  })

  it('returns default for file without extension', () => {
    const def = getFileTypeIcon('Makefile')
    expect(def.icon).toBe(File)
    expect(def.className).toBe('text-muted-foreground')
  })

  it('is case-insensitive', () => {
    const def = getFileTypeIcon('FILE.TEX')
    expect(def.icon).toBe(FileText)
    expect(def.className).toBe('text-primary')
  })
})

describe('FileTypeIcon', () => {
  it('renders without crashing', () => {
    const { container } = render(<FileTypeIcon name="main.tex" />)
    const svg = container.querySelector('svg')
    expect(svg).toBeInTheDocument()
  })
})
