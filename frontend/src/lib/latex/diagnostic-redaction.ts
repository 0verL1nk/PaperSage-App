/**
 * 编译诊断脱敏：与 `server/internal/compilediag/redact.go` 同一套白名单规则
 * （两端各自测试）。客户端先跑一遍，保证论文正文不出浏览器；服务端会再强制跑一遍。
 *
 * 只保留 TeX 诊断行；源码回显（`l.NN` 之后的内容）、`(./chapters/x.tex)` 文件展开、
 * `\argument` 展开内容一律丢弃。
 */

const MAX_LOG_EXCERPT_BYTES = 32 * 1024
const MAX_MESSAGE_CHARS = 500

// eslint-disable-next-line no-control-regex -- 这些控制字符正是要剥离的目标
const ANSI_CSI = /\x1b\[[0-9;?]*[ -/]*[@-~]/g
// eslint-disable-next-line no-control-regex -- 同上：保留 \n \t，其余控制字符丢弃
const CONTROL_CHARS = /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]/g
const LINE_NUMBER = /^l\.\d+/
const PACKAGE_DIAG = /^Package \S+ (Error|Warning)\b/
const ENGINE_BANNER = /^This is (pdfTeX|XeTeX|LuaTeX|e-TeX)/

const DIAGNOSTIC_PREFIXES = [
  '!',
  'LaTeX Error',
  'LaTeX Warning',
  'LaTeX Font Warning',
  'Overfull',
  'Underfull',
  'Emergency stop',
  'Fatal error',
  'Runaway argument',
  'Output written',
  'No pages of output',
]

function redactLine(line: string): string | null {
  if (!line) return null
  const lineNumber = LINE_NUMBER.exec(line)
  if (lineNumber) return lineNumber[0]
  if (DIAGNOSTIC_PREFIXES.some((prefix) => line.startsWith(prefix))) return line
  if (PACKAGE_DIAG.test(line) || ENGINE_BANNER.test(line)) return line
  return null
}

/** 返回只含诊断行的日志片段，按行边界截断到 32 KiB。 */
export function redactDiagnosticLog(raw: string, projectFiles: string[] = []): string {
  if (!raw) return ''
  let cleaned = raw.replace(ANSI_CSI, '').replace(CONTROL_CHARS, '').replace(/\r\n/g, '\n')
  // 先把项目已知路径替换掉，避免它们出现在保留下来的诊断行里。
  const knownPaths = projectFiles.filter(Boolean).sort((a, b) => b.length - a.length)
  for (const path of knownPaths) cleaned = cleaned.split(path).join('<project-file>')

  const encoder = new TextEncoder()
  let bytes = 0
  const kept: string[] = []
  for (const line of cleaned.split('\n')) {
    const redacted = redactLine(line.trim())
    if (!redacted) continue
    const size = encoder.encode(redacted).length + 1
    if (bytes + size > MAX_LOG_EXCERPT_BYTES) break
    bytes += size
    kept.push(redacted)
  }
  return kept.join('\n')
}

/** 错误摘要 ≤500 字符；latex 类优先首条 `!` 行。 */
export function redactDiagnosticMessage(kind: string, message: string): string {
  let text = message
  if (kind === 'latex') {
    const bang = message.split(/\r?\n/).map((line) => line.trim()).find((line) => line.startsWith('!'))
    if (bang) text = bang
  }
  return Array.from(text.trim()).slice(0, MAX_MESSAGE_CHARS).join('')
}
