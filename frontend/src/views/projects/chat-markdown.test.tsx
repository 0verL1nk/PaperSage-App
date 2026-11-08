import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ComponentProps } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ChatMarkdown } from './chat-markdown'

const markdownMock = vi.hoisted(() => ({ fail: false }))

vi.mock('react-markdown', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-markdown')>()
  const { createElement } = await import('react')
  return {
    ...actual,
    default: (props: ComponentProps<typeof actual.default>) => {
      if (markdownMock.fail) throw new Error('parse failed')
      return createElement(actual.default, props)
    },
  }
})

beforeEach(() => {
  markdownMock.fail = false
})

describe('ChatMarkdown', () => {
  it('renders headings and lists as semantic nodes', () => {
    render(<ChatMarkdown text={'## 研究问题\n\n- 第一点\n- 第二点'} />)

    expect(screen.getByRole('heading', { level: 2, name: '研究问题' })).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(2)
    expect(screen.getByText('第一点')).toBeInTheDocument()
  })

  it('renders a fenced code block with a language label and copy action', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    render(<ChatMarkdown text={'```ts\nconst answer = 42\n```'} />)

    expect(screen.getByText('ts')).toBeInTheDocument()
    expect(screen.getByText('const answer = 42')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '复制代码' }))

    await waitFor(() => expect(writeText).toHaveBeenCalledWith('const answer = 42'))
    expect(await screen.findByText('已复制')).toBeInTheDocument()
  })

  it('shows a visible message when copying is unavailable', async () => {
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: undefined })
    render(<ChatMarkdown text={'```js\nlet x = 1\n```'} />)

    fireEvent.click(screen.getByRole('button', { name: '复制代码' }))

    expect(await screen.findByText('复制失败')).toBeInTheDocument()
  })

  it('renders GFM tables with headers and cells', () => {
    render(<ChatMarkdown text={'| 名称 | 值 |\n| --- | --- |\n| A | 1 |'} />)

    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByRole('columnheader', { name: '名称' })).toBeInTheDocument()
    expect(screen.getByRole('cell', { name: 'A' })).toBeInTheDocument()
  })

  it('opens links in a new window without an opener', () => {
    render(<ChatMarkdown text={'[示例](https://example.com)'} />)

    const link = screen.getByRole('link', { name: '示例' })
    expect(link).toHaveAttribute('href', 'https://example.com')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('never injects raw HTML from the model', () => {
    const alert = vi.spyOn(window, 'alert').mockImplementation(() => {})
    const { container } = render(
      <ChatMarkdown text={'正常文本\n\n<script>alert("xss")</script>\n\n<img src=x onerror="alert(1)">'} />,
    )

    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('img')).toBeNull()
    expect(alert).not.toHaveBeenCalled()
    expect(screen.getByText('正常文本')).toBeInTheDocument()
  })

  it('falls back to plain text when markdown rendering fails', () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    markdownMock.fail = true

    render(<ChatMarkdown text={'## 标题\n\n- 第一点'} />)

    expect(screen.getByText(/## 标题/)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { level: 2 })).not.toBeInTheDocument()
    consoleError.mockRestore()
  })

  it('shows a cursor only while streaming', () => {
    const { container, rerender } = render(<ChatMarkdown text="回答中" streaming />)
    expect(container.querySelector('[data-streaming-cursor]')).not.toBeNull()

    rerender(<ChatMarkdown text="回答完成" />)
    expect(container.querySelector('[data-streaming-cursor]')).toBeNull()
  })
})
