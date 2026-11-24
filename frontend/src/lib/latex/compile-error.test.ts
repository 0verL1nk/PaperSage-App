import { describe, expect, it } from 'vitest'
import { classifyCompileFailure, compileFailureMessage } from './compile-error'

describe('classifyCompileFailure', () => {
  it('把 catalog/S3 404 归为资源缺失', () => {
    expect(classifyCompileFailure('<Error><Code>NoSuchKey</Code></Error>')).toBe('resource')
    expect(classifyCompileFailure('资源加载失败：ctex.sty（HTTP 404）')).toBe('resource')
    expect(classifyCompileFailure('some engine error', null, true)).toBe('resource')
  })

  it('把 503 归为服务不可用', () => {
    expect(classifyCompileFailure('Request failed with status code 503')).toBe('service')
    expect(classifyCompileFailure('Failed to fetch')).toBe('service')
  })

  it('把超时/无回传归为超时，而不是服务故障', () => {
    expect(classifyCompileFailure('XeLaTeX 单步编译超过 10 分钟没有返回结果，已停止')).toBe('timeout')
    expect(classifyCompileFailure('浏览器编译超过 5 分钟没有回传结果，任务已自动结束')).toBe('timeout')
    expect(classifyCompileFailure('timeout of 60000ms exceeded')).toBe('timeout')
  })

  it('把 LaTeX 日志归为编译失败，且不把行号 503 当成服务故障', () => {
    expect(classifyCompileFailure('! Undefined control sequence.\nl.503 \\foo')).toBe('latex')
    expect(classifyCompileFailure('LaTeX Error: File `article.cls\' not found.')).toBe('latex')
  })

  it('未命中时返回 unknown', () => {
    expect(classifyCompileFailure('something odd happened')).toBe('unknown')
  })

  it('日志里的 catalog/timeout 字样不能把 LaTeX 错误误判为资源或服务问题', () => {
    expect(classifyCompileFailure('LaTeX Error: Undefined control sequence.', 'loading catalog timeout')).toBe('latex')
    expect(classifyCompileFailure('engine crashed', '! Emergency stop.')).toBe('latex')
  })
})

describe('compileFailureMessage', () => {
  it('每种原因都有固定的人话文案', () => {
    expect(compileFailureMessage('resource')).toBe('编译资源尚未准备好，请稍后重试或联系管理员')
    expect(compileFailureMessage('timeout')).toBe('编译超过预期时间仍未完成，已自动停止；请重试编译，若反复超时请检查网络或换用性能更好的设备')
    expect(compileFailureMessage('service')).toBe('编译服务暂时不可用，请稍后重试')
    expect(compileFailureMessage('latex')).toBe('LaTeX 编译失败，请查看日志定位行号')
    expect(compileFailureMessage('unknown')).toBe('编译失败，请查看日志定位问题')
  })
})
