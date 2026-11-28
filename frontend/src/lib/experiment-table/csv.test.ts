import { describe, expect, it } from 'vitest'
import { CsvParseError, parseDelimited } from './csv'

describe('parseDelimited', () => {
  it('解析逗号分隔并原样保留数值字符串', () => {
    const { table, warnings } = parseDelimited('方法,准确率,F1\nBaseline,88.10,0.842\nOurs,92.50,0.881')
    expect(table.headers).toEqual(['方法', '准确率', 'F1'])
    expect(table.rows).toEqual([
      ['Baseline', '88.10', '0.842'],
      ['Ours', '92.50', '0.881'],
    ])
    expect(warnings).toEqual([])
  })

  it('识别 Excel 复制出来的制表符分隔', () => {
    const { table } = parseDelimited('方法\t准确率\nBaseline\t88.1')
    expect(table.headers).toEqual(['方法', '准确率'])
    expect(table.rows).toEqual([['Baseline', '88.1']])
  })

  it('引号内的逗号、换行与双引号不拆列', () => {
    const { table } = parseDelimited('方法,备注\n"Ours, v2","第一行\n第二行"\n"引号 ""内嵌""",ok')
    expect(table.headers).toEqual(['方法', '备注'])
    expect(table.rows[0]).toEqual(['Ours, v2', '第一行 第二行'])
    expect(table.rows[1]).toEqual(['引号 "内嵌"', 'ok'])
  })

  it('剥离 BOM 并兼容 CRLF', () => {
    const { table } = parseDelimited('﻿方法,准确率\r\nA,90\r\n')
    expect(table.headers).toEqual(['方法', '准确率'])
    expect(table.rows).toEqual([['A', '90']])
  })

  it('列数少于表头时补空并给出带行号的警告', () => {
    const { table, warnings } = parseDelimited('方法,准确率,F1\nA,90')
    expect(table.rows).toEqual([['A', '90', '']])
    expect(warnings[0]).toContain('第 2 行')
  })

  it('列数多于表头时报错并指出行号，不静默扩列', () => {
    expect(() => parseDelimited('方法,准确率\nA,90,多余')).toThrow(CsvParseError)
    expect(() => parseDelimited('方法,准确率\nA,90,多余')).toThrow(/第 2 行有 3 列，表头只有 2 列/)
  })

  it('空输入与只有表头都明确报错', () => {
    expect(() => parseDelimited('   ')).toThrow(/没有可解析的内容/)
    expect(() => parseDelimited('方法,准确率')).toThrow(/只有表头/)
  })

  it('无表头模式生成列名并把首行当数据', () => {
    const { table } = parseDelimited('A,90\nB,91', { hasHeader: false })
    expect(table.headers).toEqual(['列 1', '列 2'])
    expect(table.rows).toEqual([
      ['A', '90'],
      ['B', '91'],
    ])
  })
})
