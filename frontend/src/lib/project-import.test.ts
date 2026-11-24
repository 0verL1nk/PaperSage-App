import JSZip from 'jszip'
import { describe, expect, it, vi } from 'vitest'
import {
  buildImportPlan,
  collectDroppedFiles,
  commonRelativePaths,
  DEFAULT_IMPORT_LIMITS,
  filesFromInput,
  importEntries,
  normalizeImportPath,
  type DroppedImportFile,
  type ImportEntry,
  type ImportLimits,
} from './project-import'

async function makeZip(name: string, entries: Record<string, string>): Promise<File> {
  const zip = new JSZip()
  for (const [path, content] of Object.entries(entries)) zip.file(path, content)
  const bytes = await zip.generateAsync({ type: 'arraybuffer' })
  return new File([bytes], name, { type: 'application/zip' })
}

async function loadedZip(entries: Record<string, string>): Promise<JSZip> {
  const zip = new JSZip()
  for (const [path, content] of Object.entries(entries)) zip.file(path, content)
  return JSZip.loadAsync(await zip.generateAsync({ type: 'arraybuffer' }))
}

/** Records which ZIP entries the implementation actually decompresses. */
function spyExtractions(zip: JSZip): string[] {
  const extracted: string[] = []
  for (const [name, entry] of Object.entries(zip.files)) {
    if (entry.dir) continue
    const original = entry.async.bind(entry) as (type: 'arraybuffer') => Promise<ArrayBuffer>
    entry.async = ((type: 'arraybuffer') => {
      extracted.push(name)
      return original(type)
    }) as typeof entry.async
  }
  return extracted
}

async function expectRejectedWithoutExtraction(
  zip: JSZip,
  limits: Partial<ImportLimits>,
  message: string,
): Promise<string[]> {
  const extracted = spyExtractions(zip)
  const load = vi.spyOn(JSZip, 'loadAsync').mockResolvedValue(zip)
  try {
    await expect(
      buildImportPlan([{ file: new File(['zip'], 'project.zip'), path: 'project.zip' }], new Set(), {
        ...DEFAULT_IMPORT_LIMITS,
        ...limits,
      }),
    ).rejects.toThrow(message)
  } finally {
    load.mockRestore()
  }
  return extracted
}

function dropped(path: string, content = 'x'): DroppedImportFile {
  return { file: new File([content], path.split('/').pop() || path), path }
}

function fileEntry(name: string, content = 'x'): FileSystemFileEntry {
  return {
    isFile: true,
    isDirectory: false,
    name,
    file: (success: (file: File) => void) => success(new File([content], name)),
  } as unknown as FileSystemFileEntry
}

function directoryEntry(name: string, children: FileSystemEntry[]): FileSystemDirectoryEntry {
  let drained = false
  return {
    isFile: false,
    isDirectory: true,
    name,
    createReader: () => ({
      readEntries: (success: (entries: FileSystemEntry[]) => void) => {
        if (drained) success([])
        else {
          drained = true
          success(children)
        }
      },
    }),
  } as unknown as FileSystemDirectoryEntry
}

describe('normalizeImportPath', () => {
  it('keeps nested project paths and normalizes separators', () => {
    expect(normalizeImportPath('body/ch1_intro.tex')).toBe('body/ch1_intro.tex')
    expect(normalizeImportPath('./body\\ch1_intro.tex')).toBe('body/ch1_intro.tex')
    expect(normalizeImportPath('a//b.tex')).toBe('a/b.tex')
  })

  it.each([
    '../evil.tex',
    'body/../../evil.tex',
    '/etc/passwd',
    'C:\\Windows\\evil.tex',
    'bad<name>.tex',
    '',
  ])('rejects unsafe path %s', (raw) => {
    expect(normalizeImportPath(raw)).toBeNull()
  })
})

describe('buildImportPlan', () => {
  it('unpacks a ZIP with nested folders, skips system junk, and keeps text/binary content apart', async () => {
    const zip = await makeZip('hithesis.zip', {
      'thesis.tex': '\\documentclass{book}',
      'body/ch1_intro.tex': 'chapter one',
      'figures/logo.png': 'not really a png',
      'data/raw.dat': '\u0000binary',
      '__MACOSX/._thesis.tex': 'resource fork',
      '.DS_Store': 'junk',
    })

    const plan = await buildImportPlan([{ file: zip, path: 'hithesis.zip' }], new Set())

    expect(plan.entries.map((entry) => entry.path)).toEqual([
      'thesis.tex',
      'body/ch1_intro.tex',
      'figures/logo.png',
      'data/raw.dat',
    ])
    const thesis = plan.entries.find((entry) => entry.path === 'thesis.tex')
    expect(thesis?.isText).toBe(true)
    expect(thesis?.content).toBe('\\documentclass{book}')
    expect(plan.entries.find((entry) => entry.path === 'figures/logo.png')?.isText).toBe(false)
    expect(plan.entries.find((entry) => entry.path === 'data/raw.dat')?.isText).toBe(false)
    expect(plan.skipped.map((item) => item.path)).toEqual(
      expect.arrayContaining(['__MACOSX/._thesis.tex', '.DS_Store']),
    )
  })

  it('rejects path traversal from dropped files', async () => {
    const plan = await buildImportPlan(
      [{ file: new File(['evil'], 'evil.tex'), path: '../evil.tex' }],
      new Set(),
    )

    expect(plan.entries).toEqual([])
    expect(plan.skipped).toEqual([
      { path: '../evil.tex', reason: expect.stringContaining('路径不安全') },
    ])
  })

  it('skips files that collide with the project or with each other', async () => {
    const plan = await buildImportPlan(
      [dropped('main.tex'), dropped('body/ch1.tex'), dropped('body/ch1.tex')],
      new Set(['main.tex']),
    )

    expect(plan.entries.map((entry) => entry.path)).toEqual(['body/ch1.tex'])
    expect(plan.skipped).toEqual([
      { path: 'main.tex', reason: '项目内已存在同名文件' },
      { path: 'body/ch1.tex', reason: '导入内容中路径重复' },
    ])
  })

  it('enforces per-file size limits', async () => {
    const zip = await makeZip('big.zip', { 'huge.tex': 'x'.repeat(20), 'ok.tex': 'ok' })

    const plan = await buildImportPlan([{ file: zip, path: 'big.zip' }], new Set(), {
      ...DEFAULT_IMPORT_LIMITS,
      maxFileBytes: 10,
    })

    expect(plan.entries.map((entry) => entry.path)).toEqual(['ok.tex'])
    expect(plan.skipped).toContainEqual({ path: 'huge.tex', reason: '文件超过大小上限' })
  })

  it('rejects archives over the file-count or total-size limit', async () => {
    const many = await makeZip('many.zip', { 'a.tex': 'a', 'b.tex': 'b', 'c.tex': 'c' })
    await expect(
      buildImportPlan([{ file: many, path: 'many.zip' }], new Set(), {
        ...DEFAULT_IMPORT_LIMITS,
        maxFiles: 2,
      }),
    ).rejects.toThrow('文件过多')

    const heavy = await makeZip('heavy.zip', { 'a.tex': 'aaaa', 'b.tex': 'bbbb' })
    await expect(
      buildImportPlan([{ file: heavy, path: 'heavy.zip' }], new Set(), {
        ...DEFAULT_IMPORT_LIMITS,
        maxTotalBytes: 5,
      }),
    ).rejects.toThrow('超过')
  })

  it('rejects an over-count ZIP before decompressing any entry', async () => {
    const zip = await loadedZip({ 'a.tex': 'a', 'b.tex': 'b', 'c.tex': 'c' })

    const extracted = await expectRejectedWithoutExtraction(zip, { maxFiles: 2 }, '文件过多')

    expect(extracted).toEqual([])
  })

  it('rejects an over-size ZIP from declared sizes before decompressing any entry', async () => {
    const zip = await loadedZip({ 'a.tex': 'aaaa', 'b.tex': 'bbbb' })

    const extracted = await expectRejectedWithoutExtraction(zip, { maxTotalBytes: 5 }, '超过')

    expect(extracted).toEqual([])
  })

  it('stops reading entries when actual sizes outrun the declared totals', async () => {
    const zip = await loadedZip({ 'a.tex': 'a', 'b.tex': 'b', 'c.tex': 'c' })
    const extracted: string[] = []
    for (const [name, entry] of Object.entries(zip.files)) {
      // Headers claim 1 byte each; pretend every payload is actually 10 bytes.
      entry.async = (() => {
        extracted.push(name)
        return Promise.resolve(new ArrayBuffer(10))
      }) as typeof entry.async
    }

    const load = vi.spyOn(JSZip, 'loadAsync').mockResolvedValue(zip)
    try {
      await expect(
        buildImportPlan([{ file: new File(['zip'], 'project.zip'), path: 'project.zip' }], new Set(), {
          ...DEFAULT_IMPORT_LIMITS,
          maxTotalBytes: 15,
        }),
      ).rejects.toThrow('超过')
    } finally {
      load.mockRestore()
    }

    expect(extracted).toEqual(['a.tex', 'b.tex'])
  })

  it('reports unreadable ZIP files with an actionable message', async () => {
    const broken = new File(['not a zip'], 'broken.zip')
    await expect(buildImportPlan([{ file: broken, path: 'broken.zip' }], new Set())).rejects.toThrow(
      '无法读取 ZIP',
    )
  })
})

describe('collectDroppedFiles', () => {
  it('walks dropped folders and keeps their relative structure', async () => {
    const dataTransfer = {
      items: [
        {
          kind: 'file',
          webkitGetAsEntry: () =>
            directoryEntry('hithesis', [
              fileEntry('thesis.tex'),
              directoryEntry('body', [fileEntry('ch1_intro.tex')]),
            ]),
        },
      ],
      files: [],
    } as unknown as DataTransfer

    const files = await collectDroppedFiles(dataTransfer)

    expect(files.map((item) => item.path)).toEqual(['hithesis/thesis.tex', 'hithesis/body/ch1_intro.tex'])
  })

  it('falls back to the file list and honors webkitRelativePath', async () => {
    const file = new File(['x'], 'thesis.tex')
    Object.defineProperty(file, 'webkitRelativePath', { value: 'hithesis/thesis.tex' })
    const dataTransfer = { items: [], files: [file] } as unknown as DataTransfer

    const files = await collectDroppedFiles(dataTransfer)

    expect(files.map((item) => item.path)).toEqual(['hithesis/thesis.tex'])
  })
})

describe('filesFromInput', () => {
  it('uses webkitRelativePath when the picker supplied one', () => {
    const file = new File(['x'], 'ch1.tex')
    Object.defineProperty(file, 'webkitRelativePath', { value: 'body/ch1.tex' })

    expect(filesFromInput([file])).toEqual([{ file, path: 'body/ch1.tex' }])
  })
})

describe('commonRelativePaths', () => {
  it('derives paths relative to the deepest shared parent folder', () => {
    const relative = commonRelativePaths([
      '/home/u/thesis/thesis.tex',
      '/home/u/thesis/body/ch1.tex',
    ])

    expect(relative.get('/home/u/thesis/thesis.tex')).toBe('thesis.tex')
    expect(relative.get('/home/u/thesis/body/ch1.tex')).toBe('body/ch1.tex')

    const windows = commonRelativePaths([
      'C:\\Users\\u\\thesis\\thesis.tex',
      'C:\\Users\\u\\thesis\\figures\\logo.png',
    ])
    expect(windows.get('C:\\Users\\u\\thesis\\thesis.tex')).toBe('thesis.tex')
    expect(windows.get('C:\\Users\\u\\thesis\\figures\\logo.png')).toBe('figures/logo.png')
  })

  it('keeps a single dropped file at the project root', () => {
    expect(commonRelativePaths(['/home/u/thesis.zip']).get('/home/u/thesis.zip')).toBe('thesis.zip')
  })
})

describe('importEntries', () => {
  const entries: ImportEntry[] = ['a.tex', 'b.tex', 'c.tex', 'd.tex', 'e.tex'].map((path) => ({
    path,
    file: new File([''], path),
    isText: true,
  }))

  it('keeps importing after a failure and reports progress for every file', async () => {
    const progress = vi.fn()
    const outcome = await importEntries(
      entries,
      async (entry) => {
        if (entry.path === 'c.tex') throw new Error('同名文件已存在')
      },
      progress,
    )

    expect(outcome.imported.map((entry) => entry.path)).toEqual(['a.tex', 'b.tex', 'd.tex', 'e.tex'])
    expect(outcome.failed).toEqual([{ path: 'c.tex', message: '同名文件已存在' }])
    expect(progress).toHaveBeenCalledTimes(entries.length)
    expect(progress).toHaveBeenLastCalledWith(entries.length, entries.length)
  })

  it('caps concurrent writes', async () => {
    let active = 0
    let peak = 0

    await importEntries(entries, async () => {
      active += 1
      peak = Math.max(peak, active)
      await new Promise((resolve) => setTimeout(resolve, 5))
      active -= 1
    }, undefined, 2)

    expect(peak).toBeLessThanOrEqual(2)
  })
})
