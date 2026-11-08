import { describe, expect, it } from 'vitest'
import { classifyCompileFailure, compileFailureMessage } from './compile-error'

describe('classifyCompileFailure', () => {
  it('把 catalog/S3 404 归为资源缺失', () => {
    expect(classifyCompileFailure('<Error><Code>NoSuchKey</Code></Error>')).toBe('resource')
    expect(classifyCompileFailure('资源加载失败：ctex.sty（HTTP 404）')).toBe('resource')
    expect(classifyCompileFailure('some engine error', null, true)).toBe('resource')
  })

  it('把 503 与超时归为服务不可用', () => {
    expect(classifyCompileFailure('Request failed with status code 503')).toBe('service')
    expect(classifyCompileFailure('xetex compilelatex 超过 10 分钟未完成')).toBe('service')
    expect(classifyCompileFailure('Failed to fetch')).toBe('service')
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
    expect(compileFailureMessage('service')).toBe('编译服务暂时不可用，请稍后重试')
    expect(compileFailureMessage('latex')).toBe('LaTeX 编译失败，请查看日志定位行号')
    expect(compileFailureMessage('unknown')).toBe('编译失败，请查看日志定位问题')
  })
})
