import axios from 'axios'

/** 传输层失败的用户文案：client.ts 的全局 toast 与 normalizeApiError 共用。 */
export const NETWORK_ERROR_MESSAGE = '网络连接失败，请检查网络后重试。'
export const UNKNOWN_ERROR_MESSAGE = '发生未知错误，请稍后重试。'

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
  'Verification email could not be sent. Please try again shortly.': '验证邮件发送失败，请稍后重试；账号已保留，重试注册会重新发送。',
  'Verification email is cooling down; please try again shortly.': '发送过于频繁，请稍后再试。',
  'Unable to check verification status. Please try again shortly.': '暂时无法确认邮箱验证状态，请稍后重试。',
  'Unsupported file type: ': '不支持的文件类型：',
  '文件大小不能超过': '文件大小不能超过',
  '邮箱域名不在允许列表内': '邮箱域名不在允许列表内',
  'Invalid or expired refresh token': '登录已过期，请重新登录',
  'Refresh token has been revoked': '登录已失效，请重新登录',
  'Invalid or expired verification link': '验证链接无效或已过期，请重新发送验证邮件',
  'Invalid or expired password reset link': '密码重置链接无效、已过期或已使用，请重新申请',
  'Login code email could not be sent. Please try again shortly.': '验证码邮件发送失败，请稍后重试。',
  'Too many verification code requests. Please try again later.': '验证码请求过于频繁，请稍后再试。',
  'Invalid or expired verification code': '验证码错误或已过期，请重新获取。',
  'Account is disabled': '账号已被停用',
  'This email is linked to an unavailable account. Please contact support.': '该邮箱关联的账号不可用，请联系管理员。',
  'Current password is incorrect': '当前密码不正确',
  'Username already exists': '该账号已被注册，请换一个账号名',
  'Too many requests': '操作过于频繁，请稍后再试',
  'Too many attempts, try later': '尝试次数过多，请稍后再试',
  'Invalid refresh token': '登录已失效，请重新登录',
  'Payload too large': '提交的内容过大，请精简后重试',
  // 资源不存在：后端 NotFound 的英文契约串（HELE-22）
  'Resource not found': '请求的内容不存在或已被删除',
  'Source version not found': '来源版本不存在或已被删除',
  'Revision not found': '修订版本不存在，请刷新后重试',
  'Artifact not found': '研究产物不存在或已被删除',
  'Norm pack not found': '规范包不存在或已被删除',
  'Norm entry not found': '规范条目不存在或已被删除',
  'Adopted norm not found': '该规范尚未被采用或已移除',
  'Compile diagnostic not found': '编译诊断记录不存在或已过期',
  'Conversation not found': '会话不存在或已被删除，请新建会话',
  'Run not found': '运行记录不存在或已失效',
  'Turn not found': '对话记录不存在或已失效',
  'Proposal not found': '这条写作建议不存在或已失效',
  'Tool call not found': '工具调用记录不存在或已失效',
  'Group not found or does not belong to user': '分组不存在或不属于当前账号',
  'Not Found': '请求的接口不存在，请刷新页面或升级客户端后重试',
  'Method Not Allowed': '当前客户端不支持该请求方式，请刷新页面后重试',
  // 设置 / 同步 / 编译
  'Failed to load settings': '设置加载失败，请稍后重试',
  'Failed to reset settings': '设置重置失败，请稍后重试',
  'base_url and api_key are required': '请填写服务地址和 API Key',
  'streaming unsupported': '当前环境不支持流式输出，请稍后重试',
  'Import key conflicts with existing content or resource': '导入内容与现有文件或资料冲突',
  'Compile job is already complete or cancelled': '编译任务已结束，无法再操作',
  'Compile job is no longer running': '编译任务已结束',
  'Unable to cancel compile job': '取消编译失败，请稍后重试',
  'Unable to save the browser PDF to object storage; retry the upload': '编译结果保存失败，请重试',
  'PDF was uploaded but compile status could not be confirmed; retry the same result': '编译结果已上传，但状态确认失败，请重试同一结果',
  'Server-side LaTeX compilation is disabled. Start compilation from the project editor to use the browser engine.': '服务端编译已停用，请在项目编辑器中用浏览器编译。',
  'Browser compilation supports only pdflatex and xelatex': '浏览器编译仅支持 pdflatex 和 xelatex 两种编译器',
  'Requested range not satisfiable': '请求的文件范围无效，请刷新后重试',
}

/** 后端带动态剩余次数的错误（逐条翻译整句，避免中英混排） */
const CODE_ATTEMPTS_LEFT = /^Incorrect verification code\. (\d+) attempts left\.$/
const CODE_ATTEMPTS_EXHAUSTED = 'Too many incorrect verification codes. Request a new one.'

/** 前缀匹配（带动态后缀的串，如文件大小/类型） */
const INFO_ZH_PREFIX: Array<[string, string]> = [
  ['Unsupported file type: ', '不支持的文件类型：'],
  ['File type not allowed: ', '不支持的文件类型：'],
  ['Unknown settings category: ', '未知的设置分类：'],
  ['Invalid compiler. Must be one of', '不支持的编译器（可选 pdflatex / xelatex / lualatex）'],
]

export function translateBackendInfo(info: string | undefined): string | undefined {
  if (!info) return info
  const attempts = CODE_ATTEMPTS_LEFT.exec(info)
  if (attempts) return `验证码不正确，还可尝试 ${attempts[1]} 次`
  if (info === CODE_ATTEMPTS_EXHAUSTED) return '验证码错误次数过多，请重新获取验证码。'
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

/**
 * 面向用户的错误文案：中文原样返回；已知传输层错误转成可行动提示。
 * 页面错误态统一用它，避免把 "Network request failed." 直接甩给用户。
 */
export function friendlyErrorMessage(error: unknown): string {
  const raw = error instanceof Error ? error.message : String(error ?? '')
  if (/[一-鿿]/.test(raw)) return raw
  if (/network|fetch|timeout|econn|socket/i.test(raw)) return '网络连接失败，请检查网络后重试。'
  return '暂时无法读取数据，请稍后重试。'
}

/** 后端没给 info/detail 时的兜底文案，避免把 axios 的英文错误直接甩给用户。 */
const HTTP_STATUS_ZH: Record<number, string> = {
  400: '请求参数有误，请检查后重试。',
  401: '请先登录后再操作。',
  403: '没有执行该操作的权限。',
  404: '请求的内容不存在或已被删除。',
  405: '当前版本不支持该操作，请刷新页面后重试。',
  409: '数据已在其他位置更新，请刷新后重试。',
  413: '提交的内容过大，请精简后重试。',
  422: '请求参数验证失败，请检查输入。',
  429: '操作过于频繁，请稍后再试。',
}

function statusFallback(status: number): string {
  return HTTP_STATUS_ZH[status] ?? (status >= 500 ? '服务开小差了，请稍后重试。' : '请求失败，请稍后重试。')
}

export function normalizeApiError(error: unknown) {
  if (isFrontendApiError(error)) {
    return error
  }

  if (axios.isAxiosError(error)) {
    if (error.response) {
      const data = error.response.data
      // 信封错误用 info；路由级 404/405 是 FastAPI 风格的 { detail }，同样要翻译。
      const raw =
        typeof data === 'object' && data !== null
          ? 'info' in data
            ? data.info
            : 'detail' in data
              ? data.detail
              : undefined
          : undefined
      const info = raw == null ? undefined : String(raw)

      return new FrontendApiError(translateBackendInfo(info) || statusFallback(error.response.status), {
        kind: 'http',
        status: error.response.status,
        info,
        details: data,
        cause: error,
      })
    }

    if (error.request) {
      return new FrontendApiError(NETWORK_ERROR_MESSAGE, {
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

  return new FrontendApiError(UNKNOWN_ERROR_MESSAGE, {
    kind: 'unknown',
    details: error,
  })
}
