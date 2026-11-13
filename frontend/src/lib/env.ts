/**
 * 运行时环境解析。
 *
 * 构建期只提供两个「目标」的地址：
 *   - online：`VITE_API_BASE_URL`（部署时注入；留空表示同源，即前端由后端二进制一起提供）
 *   - local ：`VITE_LOCAL_API_BASE_URL`（默认 http://localhost:5478）
 *
 * 选中的目标存在 localStorage，**在请求时解析**而不是构建期固化——
 * 否则部署出去的静态前端无法切回本地后端。切换后需要整页刷新：
 * 令牌与 React Query 缓存都绑定在原来的后端上。
 */

const rawApiBaseUrl = import.meta.env.VITE_API_BASE_URL?.trim() ?? ''
const rawLocalApiBaseUrl =
  import.meta.env.VITE_LOCAL_API_BASE_URL?.trim() || 'http://localhost:5478'

export type ApiTarget = 'online' | 'local' | 'custom'

const API_TARGET_KEY = 'papersage_api_target'
const CUSTOM_API_BASE_URL_KEY = 'papersage_custom_api_base_url'

// Set by the desktop bootstrap once the supervised backend reports its URL.
let desktopLocalApiBaseUrl = ''

export function setDesktopLocalApiBaseUrl(value: string) {  const candidate = value.trim()
  if (!candidate) {
    desktopLocalApiBaseUrl = ''
    return
  }
  try {
    const url = new URL(candidate)
    if (!['http:', 'https:'].includes(url.protocol)) return
    desktopLocalApiBaseUrl = url.href.replace(/\/+$/, '')
  } catch {
    // Ignore malformed desktop URLs; keep the build-time local address.
  }
}

export const appEnv = Object.freeze({
  apiBaseUrl: rawApiBaseUrl,
  localApiBaseUrl: rawLocalApiBaseUrl,
  mode: import.meta.env.MODE,
  isDevelopment: import.meta.env.DEV,
  isProduction: import.meta.env.PROD,
  isTest: import.meta.env.MODE === 'test',
})

export function getDesktopLocalApiBaseUrl(): string {
  return desktopLocalApiBaseUrl
}

/** 当前选中的目标；未设置或值非法时回落。桌面端首次启动默认用托管后端。 */
export function getApiTarget(): ApiTarget {
  if (typeof localStorage === 'undefined') return desktopLocalApiBaseUrl ? 'local' : 'online'
  const target = localStorage.getItem(API_TARGET_KEY)
  if (target === 'local') return 'local'
  if (target === 'custom' && getCustomApiBaseUrl()) return 'custom'
  if (!target && desktopLocalApiBaseUrl) return 'local'
  return 'online'
}

export function setApiTarget(target: ApiTarget) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(API_TARGET_KEY, target)
}

export function getCustomApiBaseUrl(): string {
  if (typeof localStorage === 'undefined') return ''
  return localStorage.getItem(CUSTOM_API_BASE_URL_KEY) ?? ''
}

export function normalizeApiBaseUrl(value: string): string {
  const candidate = value.trim()
  if (!candidate) throw new Error('请输入后端地址')
  if (!/^https?:\/\//i.test(candidate)) {
    throw new Error('请输入包含 http:// 或 https:// 的完整地址')
  }
  let url: URL
  try {
    url = new URL(candidate)
  } catch {
    throw new Error('请输入包含 http:// 或 https:// 的完整地址')
  }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new Error('后端地址只支持 http(s) URL，且不能包含账号、密码、查询参数或片段')
  }
  return url.href.replace(/\/+$/, '')
}

export function setCustomApiBaseUrl(value: string) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(CUSTOM_API_BASE_URL_KEY, normalizeApiBaseUrl(value))
}

/** 目标对应的 base URL；online 为空串表示同源。 */
export function getApiBaseUrlFor(target: ApiTarget) {
  if (target === 'local') return desktopLocalApiBaseUrl || rawLocalApiBaseUrl
  if (target === 'custom') return getCustomApiBaseUrl()
  return rawApiBaseUrl
}

export function getOptionalApiBaseUrl() {
  return getApiBaseUrlFor(getApiTarget())
}

export function getRequiredApiBaseUrl() {
  return getOptionalApiBaseUrl()
}

export function assertRequiredAppEnv() {
  if (appEnv.isProduction) {
    getRequiredApiBaseUrl()
  }
}
