import { describe, expect, it } from 'vitest'
import { buildBooktabs, escapeLatex, formatCell, sanitizeLabel } from './booktabs'
import type { BuildOptions, ExperimentTable } from './types'

const options: BuildOptions = {
  caption: '不同方法在测试集上的性能对比',
  label: 'tab:results',
  precision: 2,
  bold: ['none', 'max', 'max'],
  includeFloat: true,
}

const table: ExperimentTable = {
  headers: ['方法', '准确率 (%)', 'F1'],
  rows: [
    ['Baseline', '88.1', '0.842'],
    ['Ours', '92.5', '0.881'],
  ],
}

describe('escapeLatex', () => {
  it('转义 booktabs 单元格里的全部特殊字符', () => {
    expect(escapeLatex('100% & a_b #1 $x$ {y} ~z ^2 \\')).toBe(
      '100\\% \\& a\\_b \\#1 \\$x\\$ \\{y\\} \\textasciitilde{}z \\textasciicircum{}2 \\textbackslash{}',
    )
  })

  it('转义不会二次处理自身插入的命令', () => {
    expect(escapeLatex('\\')).toBe('\\textbackslash{}')
  })
})

describe('sanitizeLabel', () => {
  it('只保留 LaTeX 引用安全字符，空值回退默认 label', () => {
    expect(sanitizeLabel('tab:results-1.2')).toBe('tab:results-1.2')
    expect(sanitizeLabel('表格 1')).toBe('1')
    expect(sanitizeLabel('   ')).toBe('tab:results')
  })
})

describe('formatCell', () => {
  it('用十进制四舍五入，不经过浮点', () => {
    expect(formatCell('1.005', 2)).toBe('1.01')
    expect(formatCell('2.5', 0)).toBe('3')
    expect(formatCell('0.842', 2)).toBe('0.84')
  })

  it('保留百分号与科学计数法', () => {
    expect(formatCell('92.5%', 2)).toBe('92.50%')
    expect(formatCell('1.2e3', 1)).toBe('1200.0')
  })

  it('非数值单元格原样保留；精度为 null 时不改任何值', () => {
    expect(formatCell('N/A', 2)).toBe('N/A')
    expect(formatCell('88.10', null)).toBe('88.10')
  })
})

describe('buildBooktabs', () => {
  it('生成规范三线表、数值列右对齐、最优值加粗', () => {
    expect(buildBooktabs(table, options)).toBe(`\\begin{table}[htbp]
  \\centering
  \\caption{不同方法在测试集上的性能对比}
  \\label{tab:results}
  \\begin{tabular}{lrr}
    \\toprule
    方法 & 准确率 (\\%) & F1 \\\\
    \\midrule
    Baseline & 88.10 & 0.84 \\\\
    Ours & \\textbf{92.50} & \\textbf{0.88} \\\\
    \\bottomrule
  \\end{tabular}
\\end{table}`)
  })

  it('并列最优全部加粗，空值与非数值不参与比较', () => {
    const output = buildBooktabs(
      { headers: ['方法', '得分'], rows: [['A', '90'], ['B', ''], ['C', '90'], ['D', 'N/A']] },
      { ...options, caption: '', bold: ['none', 'max'], precision: null },
    )
    expect(output).toContain('A & \\textbf{90} \\\\')
    expect(output).toContain('C & \\textbf{90} \\\\')
    expect(output).toContain('B &  \\\\')
    expect(output).toContain('D & N/A \\\\')
  })

  it('最小模式加粗最小值', () => {
    const output = buildBooktabs(
      { headers: ['方法', '损失'], rows: [['A', '0.9'], ['B', '0.1']] },
      { ...options, caption: '', bold: ['none', 'min'], precision: null, includeFloat: false },
    )
    expect(output).toContain('B & \\textbf{0.1} \\\\')
    expect(output).not.toContain('A & \\textbf')
  })

  it('关闭浮动环境时只生成 tabular，便于插入已有 table', () => {
    const output = buildBooktabs(table, { ...options, includeFloat: false })
    expect(output.startsWith('\\begin{tabular}{lrr}')).toBe(true)
    expect(output).not.toContain('\\caption')
    expect(output).not.toContain('\\begin{table}')
  })

  it('转义 caption 与表头特殊字符', () => {
    const output = buildBooktabs(
      { headers: ['方法_1', '准确率 (%)'], rows: [['A', '90']] },
      { ...options, caption: '结果 & 对比', bold: ['none', 'none'] },
    )
    expect(output).toContain('\\caption{结果 \\& 对比}')
    expect(output).toContain('方法\\_1 & 准确率 (\\%) \\\\')
  })

  it('空表头生成默认列名', () => {
    const output = buildBooktabs({ headers: ['方法', ''], rows: [['A', '1']] }, { ...options, bold: ['none', 'none'] })
    expect(output).toContain('方法 & 列 2 \\\\')
  })
})
