import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import '@/i18n'
import { PageContainer } from '@/components/shared/page-container'
import { PageHeader } from '@/components/shared/page-header'
import { SectionCard } from '@/components/shared/section-card'
import { PanelToolbar } from '@/components/shared/panel-toolbar'
import { EmptyState, ErrorState, LoadingState } from '@/components/shared/state-block'
import { StatusBadge } from '@/components/shared/status-badge'
import { SegmentedControl } from '@/components/shared/segmented-control'

describe('PageContainer', () => {
  it('renders children with the requested width', () => {
    render(
      <PageContainer size="narrow">
        <p>内容</p>
      </PageContainer>,
    )
    expect(screen.getByText('内容').parentElement).toHaveClass('max-w-3xl')
  })
})

describe('PageHeader', () => {
  it('renders title, description, eyebrow and actions', () => {
    render(
      <PageHeader
        title="项目"
        description="说明"
        eyebrow="工作区"
        actions={<button>新建</button>}
      />,
    )
    expect(screen.getByRole('heading', { level: 1, name: '项目' })).toBeInTheDocument()
    expect(screen.getByText('说明')).toBeInTheDocument()
    expect(screen.getByText('工作区')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '新建' })).toBeInTheDocument()
  })
})

describe('SectionCard', () => {
  it('renders header and content', () => {
    render(
      <SectionCard title="设置" description="账号设置" actions={<button>保存</button>}>
        <p>正文</p>
      </SectionCard>,
    )
    expect(screen.getByText('设置')).toBeInTheDocument()
    expect(screen.getByText('账号设置')).toBeInTheDocument()
    expect(screen.getByText('正文')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存' })).toBeInTheDocument()
  })
})

describe('PanelToolbar', () => {
  it('renders start, title, subtitle and end', () => {
    render(
      <PanelToolbar
        start={<button>左</button>}
        title="文件"
        subtitle="12 个"
        end={<button>右</button>}
      />,
    )
    expect(screen.getByRole('button', { name: '左' })).toBeInTheDocument()
    expect(screen.getByText('文件')).toBeInTheDocument()
    expect(screen.getByText('12 个')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '右' })).toBeInTheDocument()
  })
})

describe('StatusBadge', () => {
  it('applies semantic tone classes and an optional dot', () => {
    const { container } = render(
      <StatusBadge tone="success" dot>
        已完成
      </StatusBadge>,
    )
    const badge = container.firstElementChild as HTMLElement
    expect(badge).toHaveTextContent('已完成')
    expect(badge.className).toContain('text-success')
    expect(badge.querySelector('[aria-hidden]')).not.toBeNull()
  })
})

describe('state components', () => {
  it('EmptyState renders title and description', () => {
    render(<EmptyState title="暂无项目" description="创建第一个项目" />)
    expect(screen.getByText('暂无项目')).toBeInTheDocument()
    expect(screen.getByText('创建第一个项目')).toBeInTheDocument()
  })

  it('ErrorState shows details and retries', async () => {
    const onRetry = vi.fn()
    const user = userEvent.setup()
    render(
      <ErrorState title="加载失败" description="请稍后重试" details="timeout" onRetry={onRetry} />,
    )
    expect(screen.getByText('timeout')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '重试' }))
    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('LoadingState exposes status role and one skeleton per row plus the header line', () => {
    const { container } = render(<LoadingState title="加载中" rows={2} />)
    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(container.querySelectorAll('[data-slot="skeleton"]')).toHaveLength(3)
  })
})

describe('SegmentedControl', () => {
  it('renders options and reports value changes', async () => {
    const onValueChange = vi.fn()
    const user = userEvent.setup()
    render(
      <SegmentedControl
        items={[
          { value: 'all', label: '全部' },
          { value: 'mine', label: '我的' },
        ]}
        value="all"
        onValueChange={onValueChange}
        ariaLabel="项目筛选"
      />,
    )
    expect(screen.getByRole('radio', { name: '全部' })).toHaveAttribute('aria-checked', 'true')
    await user.click(screen.getByRole('radio', { name: '我的' }))
    expect(onValueChange).toHaveBeenCalledWith('mine')
  })
})
