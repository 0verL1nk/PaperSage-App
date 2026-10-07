import axios from 'axios'

export type ApiErrorKind = 'backend' | 'http' | 'network' | 'unknown'

export type FrontendApiErrorOptions = {
  kind: ApiErrorKind
  code?: number
  status?: number
  info?: string
  details?: unknown
  cause?: unknown
}

export class FrontendApiError extends Error {
  readonly kind: ApiErrorKind
  readonly code?: number
  readonly status?: number
  readonly info?: string
  readonly details?: unknown

  constructor(message: string, options: FrontendApiErrorOptions) {
    super(message)
    this.name = 'FrontendApiError'
    this.kind = options.kind
    this.code = options.code
    this.status = options.status
    this.info = options.info
    this.details = options.details
    this.cause = options.cause
  }
}

export function createBackendEnvelopeError(payload: {
  code: number
  info?: string
  details?: unknown
}) {
  return new FrontendApiError(translateBackendInfo(payload.info) || '请求失败，请稍后重试。', {
    kind: 'backend',
    code: payload.code,
    info: payload.info,
    details: payload.details,
  })
}


/**
 * 后端错误文案 → 用户语言。后端契约串保持英文（API 兼容），展示层
 * 统一在这里翻译；未命中的串原样透出（新错误先可见，再补条目）。
 * 注意：「账号或密码不正确」合并账号不存在与密码错误——区分二者
 * 会被用于探测已注册账号（OWASP 用户枚举）。
 */
const INFO_ZH: Record<string, string> = {
  'Invalid credentials': '账号或密码不正确',
  'Not authenticated': '请先登录',
  'Unauthorized': '请先登录',
  'Invalid token': '登录状态已失效，请重新登录',
  'User not found or inactive': '账号不可用或已被停用',
  'Please verify your email before logging in': '请先查收邮件完成邮箱验证后再登录',
  'Admin access required': '需要管理员权限',
  'Project not found': '项目不存在或已被删除',
  'Project not found in trash': '回收站中没有该项目',
  'Folder not found': '文件夹不存在或已被删除',
  'Parent folder not found': '目标父文件夹不存在',
  'Target folder not found': '目标文件夹不存在',
  'Document not found': '文档不存在或已被删除',
  'Document is not staged': '该文档不处于待入库状态',
  'Group not found': '分组不存在或已被删除',
  'Compile job not found': '编译任务不存在或已过期',
  'File not found': '文件不存在或已被删除',
  'File already exists': '同名文件已存在',
  'A file with that path already exists': '目标路径已有同名文件',
  'A folder with that name already exists': '同位置已有同名文件夹',
  'Path traversal is not allowed': '文件路径不合法',
  'Bad request': '请求参数有误',
  'Internal server error': '服务开小差了，请稍后重试',
  'Unsupported file type: ': '不支持的文件类型：',
  '文件大小不能超过': '文件大小不能超过',
  '邮箱域名不在允许列表内': '邮箱域名不在允许列表内',
  'Invalid or expired refresh token': '登录已过期，请重新登录',
  'Refresh token has been revoked': '登录已失效，请重新登录',
  'Invalid or expired verification link': '验证链接无效或已过期，请重新发送验证邮件',
  'Invalid or expired password reset link': '密码重置链接无效、已过期或已使用，请重新申请',
}

/** 前缀匹配（带动态后缀的串，如文件大小/类型） */
const INFO_ZH_PREFIX: Array<[string, string]> = [
  ['Unsupported file type: ', '不支持的文件类型：'],
  ['File type not allowed: ', '不支持的文件类型：'],
  ['Unknown settings category: ', '未知的设置分类：'],
  ['Invalid compiler. Must be one of', '不支持的编译器（可选 pdflatex / xelatex / lualatex）'],
]

export function translateBackendInfo(info: string | undefined): string | undefined {
  if (!info) return info
  const exact = INFO_ZH[info]
  if (exact) return exact
  for (const [prefix, zh] of INFO_ZH_PREFIX) {
    if (info.startsWith(prefix)) {
      const rest = info.slice(prefix.length)
      return zh + (rest.startsWith(' ') ? '' : ' ') + rest
    }
  }
  return info
}

export function isFrontendApiError(error: unknown): error is FrontendApiError {
  return error instanceof FrontendApiError
}

export function normalizeApiError(error: unknown) {
  if (isFrontendApiError(error)) {
    return error
  }

  if (axios.isAxiosError(error)) {
    if (error.response) {
      const info =
        typeof error.response.data === 'object' &&
        error.response.data !== null &&
        'info' in error.response.data
          ? String(error.response.data.info)
          : error.message

      return new FrontendApiError(translateBackendInfo(info) || '请求失败，请稍后重试。', {
        kind: 'http',
        status: error.response.status,
        info,
        details: error.response.data,
        cause: error,
      })
    }

    if (error.request) {
      return new FrontendApiError('Network request failed.', {
        kind: 'network',
        cause: error,
      })
    }
  }

  if (error instanceof Error) {
    return new FrontendApiError(error.message, {
      kind: 'unknown',
      cause: error,
    })
  }

  return new FrontendApiError('Unexpected API error.', {
    kind: 'unknown',
    details: error,
  })
}
