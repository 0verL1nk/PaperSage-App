import { sectionRanges } from './paragraph'

/**
 * 在 LaTeX 正文中定位一条批注的原文锚点。
 *
 * 纯前端字符串匹配：JS 的 offset 与 Monaco 的 UTF-16 坐标一致，
 * 不使用后端 Go 的 byte offset（中文/emoji 下会错位）。定位有歧义时
 * 明确返回 `ambiguous`，由卡片提示「无法定位」，绝不猜位置高亮。
 */

export type AnchorFailure = 'not_found' | 'ambiguous'

export interface AnchorLocation {
  start: number
  end: number
  locatable: boolean
  reason?: AnchorFailure
}

export interface AnchorProposal {
  oldText: string
  sectionPath?: string
}

function within(offset: number, start: number, end: number): boolean {
  return offset >= start && offset <= end
}

export function locateProposal(
  content: string,
  proposal: AnchorProposal,
  cursorOffset?: number,
): AnchorLocation {
  const needle = proposal.oldText
  if (!needle) return { start: -1, end: -1, locatable: false, reason: 'not_found' }

  const hits: number[] = []
  let from = 0
  for (;;) {
    const index = content.indexOf(needle, from)
    if (index < 0) break
    hits.push(index)
    from = index + Math.max(needle.length, 1)
  }
  if (!hits.length) return { start: -1, end: -1, locatable: false, reason: 'not_found' }

  const hit = (start: number): AnchorLocation => ({ start, end: start + needle.length, locatable: true })
  if (hits.length === 1) return hit(hits[0])

  const ranges = sectionRanges(content)
  let candidates = hits
  if (proposal.sectionPath && ranges.length) {
    const inSection = hits.filter((start) => {
      const range = ranges.find((r) => within(start, r.start, r.end))
      if (!range) return false
      return range.path === proposal.sectionPath
        || proposal.sectionPath!.startsWith(range.path)
        || range.path.startsWith(proposal.sectionPath!)
    })
    if (inSection.length) candidates = inSection
  }
  if (candidates.length === 1) return hit(candidates[0])

  if (typeof cursorOffset === 'number') {
    const atCursor = candidates.filter((start) => within(cursorOffset, start, start + needle.length))
    if (atCursor.length === 1) return hit(atCursor[0])
  }
  return { start: -1, end: -1, locatable: false, reason: 'ambiguous' }
}
