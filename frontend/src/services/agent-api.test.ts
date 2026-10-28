import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamRun } from '@/services/agent-api'
import * as env from '@/lib/env'

/** 用给定分片构造一个 SSE 响应体。 */
function sseResponse(chunks: string[]): Response {
  const encoder = new TextEncoder()
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const c of chunks) controller.enqueue(encoder.encode(c))
      controller.close()
    },
  })
  return new Response(stream, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
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
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('streamRun（UI Message Stream 解析）', () => {
  it.each(['https://api.paper.overlink.top/', 'http://localhost:5478', ''])('事件流跟随当前后端地址 %s', async (baseUrl) => {
    vi.spyOn(env, 'getOptionalApiBaseUrl').mockReturnValue(baseUrl)
    await collect(['data: [DONE]\n\n'])
    expect(fetch).toHaveBeenCalledWith(
      `${baseUrl.replace(/\/$/, '')}/api/v1/agent/runs/run-1/events`,
      expect.objectContaining({ headers: expect.objectContaining({ Accept: 'text/event-stream' }) }),
    )
  })

  it('前端 SPA 回退的 200 HTML 不得被当作成功的空回复', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<!doctype html><html></html>', {
      status: 200,
      headers: { 'Content-Type': 'text/html; charset=utf-8' },
    })))
    await expect(streamRun('run-1', { onEvent: () => {} })).rejects.toThrow('事件流')
  })
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
