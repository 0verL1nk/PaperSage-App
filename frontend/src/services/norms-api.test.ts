import { describe, expect, it } from 'vitest'

import { adoptionIndex, adoptionSummary, entriesURL } from './norms-api'
import type { NormAdoption, NormEntry } from '@/types/norms'

function entry(overrides: Partial<NormEntry> = {}): NormEntry {
  return {
    id: 'e1',
    packId: 'p1',
    packName: '本科毕业论文通用学术规范（2026 版）',
    code: 'GBT7714-TYPE-MARKS',
    category: '参考文献',
    ruleKind: 'official',
    binding: 'must',
    scenario: 'undergraduate_thesis',
    version: 1,
    sortOrder: 10,
    state: 'active',
    title: '按文献类型标注类型标识',
    ruleText: '期刊文章用 [J] 标注。',
    sourceRef: 'GB/T 7714—2015',
    applicableSections: ['参考文献'],
    tags: ['参考文献'],
    isBuiltin: true,
    ...overrides,
  }
}

function adoption(overrides: Partial<NormAdoption> = {}): NormAdoption {
  return {
    entryId: 'e1',
    version: 1,
    state: 'enabled',
    hasNewerVersion: false,
    latestVersion: 1,
    entry: entry(),
    ...overrides,
  }
}

describe('entriesURL', () => {
  it('省掉空值，只拼有效查询参数', () => {
    const url = entriesURL({ q: 'GB/T 7714', packId: undefined, scenario: '', pageSize: 50 })
    expect(url).toContain('q=GB%2FT+7714')
    expect(url).toContain('pageSize=50')
    expect(url).not.toContain('packId')
    expect(url).not.toContain('scenario')
  })

  it('没有参数时返回裸路径', () => {
    expect(entriesURL({})).toBe('/api/v1/norms/entries')
  })
})

describe('adoptionIndex', () => {
  it('按 entryId 建索引，供列表勾选态直接查', () => {
    const index = adoptionIndex([adoption(), adoption({ entryId: 'e2', state: 'disabled' })])
    expect(index.get('e1')?.state).toBe('enabled')
    expect(index.get('e2')?.state).toBe('disabled')
    expect(index.get('missing')).toBeUndefined()
  })
})

describe('adoptionSummary', () => {
  it('区分已采用与已启用，并统计有更新的条目', () => {
    const summary = adoptionSummary([
      adoption(),
      adoption({ entryId: 'e2', state: 'disabled' }),
      adoption({ entryId: 'e3', hasNewerVersion: true, latestVersion: 3 }),
    ])
    expect(summary.total).toBe(3)
    expect(summary.enabled).toBe(2)
    expect(summary.outdated).toBe(1)
    expect(summary.categories).toContain('参考文献')
  })

  it('没有采用任何规范时给出零值而不是报错', () => {
    expect(adoptionSummary([])).toEqual({ total: 0, enabled: 0, outdated: 0, categories: [] })
  })
})
