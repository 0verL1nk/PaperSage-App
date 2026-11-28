import { describe, expect, it } from 'vitest'
import { paragraphAt, sectionPathAt } from './paragraph'

const SAMPLE = `\\chapter{绪论}
\\section{研究背景}

人工智能正在改变科研写作方式，但通用助手常常给出缺少依据的建议。

\\subsection{问题定义}
现有工具很少约束建议必须锚定原文。

\\begin{equation}
E = mc^2
\\end{equation}
`

function offsetOf(text: string, needle: string): number {
  const index = text.indexOf(needle)
  if (index < 0) throw new Error(`missing needle: ${needle}`)
  return index + 1
}

describe('latex paragraph focus', () => {
  it('extracts the paragraph under the cursor and its section path', () => {
    const focus = paragraphAt(SAMPLE, offsetOf(SAMPLE, '人工智能'))
    expect(focus).not.toBeNull()
    expect(focus?.paragraphText).toBe('人工智能正在改变科研写作方式，但通用助手常常给出缺少依据的建议。')
    expect(focus?.sectionPath).toBe('绪论 / 研究背景')
  })

  it('uses the nearest subsection after a heading', () => {
    const focus = paragraphAt(SAMPLE, offsetOf(SAMPLE, '现有工具'))
    expect(focus?.sectionPath).toBe('绪论 / 研究背景 / 问题定义')
  })

  it('returns null when the cursor is on a heading or blank line', () => {
    expect(paragraphAt(SAMPLE, offsetOf(SAMPLE, '研究背景'))).toBeNull()
    expect(paragraphAt(SAMPLE, 0)).toBeNull()
  })

  it('does not treat math environments as prose paragraphs', () => {
    expect(paragraphAt(SAMPLE, offsetOf(SAMPLE, 'E = mc^2'))).toBeNull()
  })

  it('keeps the paragraph text as an exact substring of the source', () => {
    const focus = paragraphAt(SAMPLE, offsetOf(SAMPLE, '人工智能'))
    expect(SAMPLE).toContain(focus!.paragraphText)
  })

  it('returns empty section path before any heading', () => {
    expect(sectionPathAt('正文第一段。', 2)).toBe('')
  })
})
