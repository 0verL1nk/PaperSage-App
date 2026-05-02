import { describe, it, expect } from 'vitest'
import { toArboristData } from '../writing-file-tree'
import type { LatexFileItem } from '@/services/latex-api'

const makeFile = (filePath: string): LatexFileItem => ({
  id: filePath,
  file_path: filePath,
  content_type: 'text/x-tex',
  size_bytes: 100,
  last_saved_at: '2024-01-01',
})

describe('toArboristData', () => {
  it('creates flat file nodes', () => {
    const data = toArboristData([makeFile('main.tex'), makeFile('refs.bib')])
    expect(data).toHaveLength(2)
    expect(data.map((n) => n.name).sort()).toEqual(['main.tex', 'refs.bib'])
  })

  it('creates folder from nested path', () => {
    const data = toArboristData([makeFile('sections/intro.tex')])
    expect(data).toHaveLength(1)
    expect(data[0].name).toBe('sections')
    expect(data[0].children).toHaveLength(1)
    expect(data[0].children![0].name).toBe('intro.tex')
  })

  it('handles deep nesting', () => {
    const data = toArboristData([makeFile('a/b/c/deep.tex')])
    const a = data[0]
    expect(a.name).toBe('a')
    const b = a.children![0]
    expect(b.name).toBe('b')
    const c = b.children![0]
    expect(c.name).toBe('c')
    expect(c.children![0].name).toBe('deep.tex')
  })

  it('sorts folders before files', () => {
    const data = toArboristData([
      makeFile('zeta.tex'),
      makeFile('alpha/intro.tex'),
      makeFile('beta.tex'),
    ])
    expect(data[0].name).toBe('alpha')
    expect(data.map((n) => n.name)).toEqual(['alpha', 'beta.tex', 'zeta.tex'])
  })

  it('handles unsorted input', () => {
    const data = toArboristData([
      makeFile('z/2.tex'),
      makeFile('a/1.tex'),
      makeFile('a/0.tex'),
    ])
    expect(data[0].name).toBe('a')
    expect(data[1].name).toBe('z')
    expect(data[0].children!.map((c) => c.name)).toEqual(['0.tex', '1.tex'])
  })

  it('filters .gitkeep files', () => {
    const data = toArboristData([makeFile('images/.gitkeep')])
    expect(data).toHaveLength(1)
    expect(data[0].name).toBe('images')
    expect(data[0].children).toHaveLength(0)
  })

  it('shows real files alongside .gitkeep', () => {
    const data = toArboristData([makeFile('images/.gitkeep'), makeFile('images/photo.png')])
    expect(data[0].children).toHaveLength(1)
    expect(data[0].children![0].name).toBe('photo.png')
  })

  it('returns empty array for empty input', () => {
    expect(toArboristData([])).toEqual([])
  })

  it('merges files under same folder', () => {
    const data = toArboristData([
      makeFile('sections/intro.tex'),
      makeFile('sections/method.tex'),
      makeFile('sections/conclusion.tex'),
    ])
    expect(data).toHaveLength(1)
    expect(data[0].name).toBe('sections')
    expect(data[0].children).toHaveLength(3)
  })

  it('orphan nodes without parent become roots', () => {
    const data = toArboristData([makeFile('orphan.tex'), makeFile('root/child.tex')])
    const rootNames = data.map((n) => n.name)
    expect(rootNames).toContain('orphan.tex')
    expect(rootNames).toContain('root')
  })
})
