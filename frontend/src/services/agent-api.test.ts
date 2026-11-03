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
    const events = await collect([full.slice(0, 20), full.slice(20), 'data: [DONE]\n\n'])
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

  it('中途断流保留已收到事件并报告未完成', async () => {
    const events: unknown[] = []
    vi.stubGlobal('fetch', vi.fn(async () => sseResponse([
      'data: {"type":"start","messageId":"m1"}\n\n',
      'data: {"type":"text-delta","id":"t1","delta":"已有回答"}\n\n',
      'data: {"type":"text-delta","delta":"未完成',
    ])))
    await expect(streamRun('run-1', { onEvent: (e) => events.push(e) })).rejects.toThrow(/中断|未完成/)
    expect(events).toEqual([
      { type: 'start', messageId: 'm1' },
      { type: 'text-delta', id: 't1', delta: '已有回答' },
    ])
  })

  it('支持跨分片 CRLF 分隔符、心跳及同一分片多个事件', async () => {
    const events = await collect([
      ': ping\r\n\r',
      '\ndata:{"type":"start","messageId":"m1"}\r\n\r\ndata: {"type":"text-delta","delta":"a"}\r\n\r',
      '\ndata: [DONE]\r\n\r\n',
    ])
    expect(events).toEqual([{ type: 'start', messageId: 'm1' }, { type: 'text-delta', delta: 'a' }])
  })

  it.each(['finish', 'abort'])('服务端 %s 帧是终态', async (type) => {
    expect(await collect([`data: {"type":"${type}"}\n\n`])).toEqual([{ type }])
  })

  it('服务端终态已经到达后，后续网络故障不应撤销终态', async () => {
    let reads = 0
    vi.stubGlobal('fetch', vi.fn(async () => new Response(new ReadableStream<Uint8Array>({
      pull(controller) {
        if (reads++ === 0) controller.enqueue(new TextEncoder().encode('data: {"type":"finish"}\n\n'))
        else controller.error(new TypeError('connection lost'))
      },
    }, { highWaterMark: 0 }), { headers: { 'Content-Type': 'text/event-stream' } })))
    const events: unknown[] = []
    await expect(streamRun('run-1', { onEvent: (e) => events.push(e) })).resolves.toBeUndefined()
    expect(events).toEqual([{ type: 'finish' }])
  })

  it('error 事件不是终态，后续断流不能被当成成功', async () => {
    const events: unknown[] = []
    vi.stubGlobal('fetch', vi.fn(async () => sseResponse(['data: {"type":"error","errorText":"模型错误"}\n\n'])))
    await expect(streamRun('run-1', { onEvent: (e) => events.push(e) })).rejects.toThrow(/中断|未完成/)
    expect(events).toEqual([{ type: 'error', errorText: '模型错误' }])
  })

  it('格式错误的完整事件不能被忽略为成功', async () => {
    await expect(collect([
      'data: {"type":"start"}\n\n',
      'data: {这不是合法 JSON}\n\n',
      'data: {"type":"finish"}\n\n',
    ])).rejects.toThrow()
  })

  it('error 后的服务端 finish 仍传递原错误并正常结束传输', async () => {
    expect(await collect([
      'data: {"type":"error","errorText":"模型错误"}\n\n' +
      'data: {"type":"finish"}\n\n' +
      'data: [DONE]\n\n',
    ])).toEqual([{ type: 'error', errorText: '模型错误' }, { type: 'finish' }])
  })

  it('用户中止保持 AbortError，不当成服务端错误或成功', async () => {
    const controller = new AbortController()
    const abortError = new DOMException('Aborted', 'AbortError')
    vi.stubGlobal('fetch', vi.fn(async (_url, init: RequestInit) => {
      expect(init.signal).toBe(controller.signal)
      throw abortError
    }))
    controller.abort()
    await expect(streamRun('run-1', { signal: controller.signal, onEvent: () => {} })).rejects.toBe(abortError)
  })

  it('消费事件的异常不得被解析器吞掉', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => sseResponse(['data: {"type":"start"}\n\n', 'data: [DONE]\n\n'])))
    await expect(streamRun('run-1', { onEvent: () => { throw new Error('consumer failed') } })).rejects.toThrow('consumer failed')
  })

  it('HTTP 失败时抛出，而不是静默返回空流', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => new Response(null, { status: 500 })),
    )
    await expect(streamRun('run-1', { onEvent: () => {} })).rejects.toThrow('500')
  })
})
