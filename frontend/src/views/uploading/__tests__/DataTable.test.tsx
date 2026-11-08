import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { DataTable } from '../DataTable'

const actions = {
  onMove: vi.fn(),
  onRemark: vi.fn(),
  onDelete: vi.fn(),
  onPreview: vi.fn(),
}

describe('DataTable empty states', () => {
  it('空库提示上传，而不是无匹配结果', () => {
    render(<DataTable data={[]} actions={actions} totalCount={0} onClearFilters={vi.fn()} />)

    expect(screen.getByText(/还没有资料/)).toBeInTheDocument()
    expect(screen.queryByText('没有匹配的结果')).not.toBeInTheDocument()
  })

  it('有数据但被筛选掉时提供清除筛选', async () => {
    const onClearFilters = vi.fn()
    render(<DataTable data={[]} actions={actions} totalCount={3} onClearFilters={onClearFilters} />)

    expect(screen.getByText('没有匹配的结果')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '清除搜索和筛选' }))
    expect(onClearFilters).toHaveBeenCalledOnce()
  })
})
