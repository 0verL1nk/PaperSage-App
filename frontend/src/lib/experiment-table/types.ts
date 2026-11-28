/** 实验数据表格：浏览器内确定性解析与生成，全程不经过模型。 */
export interface ExperimentTable {
  headers: string[]
  rows: string[][]
}

export type BoldMode = 'none' | 'max' | 'min'

export interface BuildOptions {
  caption: string
  label: string
  /** null = 保留原始数值；0–4 = 固定小数位（只影响表格显示，不改源数据）。 */
  precision: number | null
  /** 与 headers 等长；每列按「最大/最小」加粗最优值。 */
  bold: BoldMode[]
  /** true 生成完整 table 浮动环境；false 只生成 tabular。 */
  includeFloat: boolean
}

export interface ParseResult {
  table: ExperimentTable
  warnings: string[]
}
