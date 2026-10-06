import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamRun } from '@/services/agent-api'

/** 用给定分片构造一个 SSE 响应体。 */
function sseResponse(chunks: string[]): Response {
  const encoder = new TextEncoder()
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c))
      controller.close()
    },
  })
  return new Response(stream, { status: 200 })
}

function collect(chunks: string[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => sseResponse(chunks)),
  )
  const events: unknown[] = []
  return streamRun('run-1', { onEvent: (e) => events.push(e) }).then(() => events)
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('streamRun（UI Message Stream 解析）', () => {
  it('解析 data: 行并跳过终止标记 [DONE]', async () => {
    const events = await collect([
      'data: {"type":"start","messageId":"m1"}\n\n',
      'data: {"type":"text-delta","id":"t1","delta":"hi"}\n\n',
      'data: [DONE]\n\n',
    ])
    expect(events).toEqual([
      { type: 'start', messageId: 'm1' },
      { type: 'text-delta', id: 't1', delta: 'hi' },
    ])
  })

  it('跨 chunk 边界不丢事件：JSON 被切成两半也能拼回', async () => {
    const full = 'data: {"type":"text-delta","id":"t1","delta":"跨边界"}\n\n'
    const events = await collect([full.slice(0, 20), full.slice(20)])
    expect(events).toEqual([{ type: 'text-delta', id: 't1', delta: '跨边界' }])
  })

  it('一个 chunk 里含多个事件时全部解析', async () => {
    const events = await collect([
      'data: {"type":"start","messageId":"m1"}\n\n' +
        'data: {"type":"text-delta","id":"t1","delta":"a"}\n\n' +
        'data: {"type":"finish"}\n\n',
    ])
    expect(events.map((e) => (e as { type: string }).type)).toEqual(['start', 'text-delta', 'finish'])
  })

  it('单个 part 解析失败不中断整条流', async () => {
    const events = await collect([
      'data: {"type":"start"}\n\n',
      'data: {这不是合法 JSON}\n\n',
      'data: {"type":"finish"}\n\n',
    ])
    expect(events.map((e) => (e as { type: string }).type)).toEqual(['start', 'finish'])
  })

  it('HTTP 失败时抛出，而不是静默返回空流', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(null, { status: 500 })),
    )
    await expect(streamRun('run-1', { onEvent: () => {} })).rejects.toThrow('500')
  })
})
