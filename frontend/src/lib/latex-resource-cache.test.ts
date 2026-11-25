import { describe, expect, it } from 'vitest'
import {
  selectSupersededTexliveEntries,
  summarizeTexliveEntries,
  type TexliveCacheEntry,
} from './latex-resource-cache'

function entry(overrides: Partial<TexliveCacheEntry> & Pick<TexliveCacheEntry, 'url'>): TexliveCacheEntry {
  return { bytes: new ArrayBuffer(1024), ...overrides }
}

describe('summarizeTexliveEntries', () => {
  it('按版本与类型汇总占用，并取占用最大的宏包', () => {
    const summary = summarizeTexliveEntries([
      entry({
        url: 'https://api.example.test/texlive/tl-new/resolve/xetex?format=26&name=ctex.sty',
        bytes: new ArrayBuffer(3000), fileid: 'a'.repeat(64), package: 'ctex',
        revision: 'tl-new', lastUsed: 1_700_000_000_000,
      }),
      entry({
        url: 'https://api.example.test/texlive/tl-new/resolve/font?format=11&name=FandolSong.otf',
        bytes: new ArrayBuffer(5000), fontid: 'b'.repeat(64), package: 'fandol',
        revision: 'tl-new', lastUsed: 1_700_000_100_000,
      }),
      entry({
        url: 'https://api.example.test/texlive/tl-old/resolve/xetex?format=26&name=geometry.sty',
        bytes: new ArrayBuffer(2000), fileid: 'c'.repeat(64), package: 'geometry',
      }),
    ])

    expect(summary.entries).toBe(3)
    expect(summary.bytes).toBe(10000)
    expect(summary.packages).toBe(2)
    expect(summary.fonts).toBe(1)
    expect(summary.lastUsed).toBe(1_700_000_100_000)
    expect(summary.byProfile).toEqual([
      { profile: 'tl-new', entries: 2, bytes: 8000 },
      { profile: 'tl-old', entries: 1, bytes: 2000 },
    ])
    expect(summary.topPackages.map((pkg) => pkg.package)).toEqual(['fandol', 'ctex', 'geometry'])
  })

  it('缺少 revision 时从 URL 推出 profile，完全无信息时归为未知版本', () => {
    const summary = summarizeTexliveEntries([
      entry({ url: 'https://api.example.test/texlive/tl-url/resolve/pdftex?format=11&name=article.cls' }),
      entry({ url: 'https://api.example.test/latex-engines/pdftex.map?sha256=abc', fileid: 'd'.repeat(64) }),
    ])

    expect(summary.byProfile.map((profile) => profile.profile)).toEqual(['tl-url', '内置资源'])
  })

  it('空缓存返回零值而不是抛错', () => {
    expect(summarizeTexliveEntries([])).toMatchObject({ entries: 0, bytes: 0, packages: 0, fonts: 0, lastUsed: null })
  })
})

describe('selectSupersededTexliveEntries', () => {
  it('只选中其他版本与无版本信息的条目', () => {
    const entries = [
      entry({ url: '/texlive/tl-new/resolve/xetex?format=26&name=a.sty', revision: 'tl-new' }),
      entry({ url: '/texlive/tl-old/resolve/xetex?format=26&name=b.sty', revision: 'tl-old' }),
      entry({ url: '/texlive/tl-old/resolve/xetex?format=26&name=c.sty' }),
      entry({ url: '/latex-engines/pdftex.map?sha256=abc' }),
    ]

    expect(selectSupersededTexliveEntries(entries, 'tl-new').map((item) => item.url)).toEqual([
      '/texlive/tl-old/resolve/xetex?format=26&name=b.sty',
      '/texlive/tl-old/resolve/xetex?format=26&name=c.sty',
    ])
  })
})
