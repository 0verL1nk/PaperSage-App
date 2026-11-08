export type CompileFailureKind = 'resource' | 'service' | 'latex' | 'unknown'

const RESOURCE_RE = /NoSuchKey|HTTP\s*404|ResourceNotFound|资源不存在|catalog/i
// 只认 HTTP/status 形式的 503，避免把 LaTeX 日志里的 "l.503" 行号误判成服务故障。
const SERVICE_RE = /HTTP\s*503|status(?:\s+code)?\s*503|503\s+Service|timeout|timed out|超时|超过\s*10\s*分钟|ECONNREFUSED|Failed to fetch|NetworkError/i
const LATEX_RE = /^\s*!|LaTeX Error|Undefined control sequence|Emergency stop|Fatal error|Runaway argument|File `[^']+' not found|LaTeX 引擎没有生成输出文件|Extra \}/im

/**
 * 把浏览器编译的原始报错归成三类可读原因。
 * `hasMissingResource` 来自编译过程中上报的资源事件：404 资源在引擎内部
 * 只标记 missing，不一定出现在最终异常文本里。
 */
export function classifyCompileFailure(
  raw: string | null | undefined,
  logs?: string | null,
  hasMissingResource = false,
): CompileFailureKind {
  const error = raw ?? ''
  // 资源/服务判断只看抛出的错误：日志里出现 "catalog"、"timeout" 这类词
  // 往往只是宏包名或源码内容，不能据此把真实 LaTeX 错误归错类。
  if (hasMissingResource || RESOURCE_RE.test(error)) return 'resource'
  if (SERVICE_RE.test(error)) return 'service'
  if (LATEX_RE.test(error) || LATEX_RE.test(logs ?? '')) return 'latex'
  return 'unknown'
}

export function compileFailureMessage(kind: CompileFailureKind): string {
  switch (kind) {
    case 'resource':
      return '编译资源尚未准备好，请稍后重试或联系管理员'
    case 'service':
      return '编译服务暂时不可用，请稍后重试'
    case 'latex':
      return 'LaTeX 编译失败，请查看日志定位行号'
    default:
      return '编译失败，请查看日志定位问题'
  }
}
