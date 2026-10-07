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

export type ApiTarget = 'online' | 'local'

const API_TARGET_KEY = 'papersage_api_target'

export const appEnv = Object.freeze({
  apiBaseUrl: rawApiBaseUrl,
  localApiBaseUrl: rawLocalApiBaseUrl,
  mode: import.meta.env.MODE,
  isDevelopment: import.meta.env.DEV,
  isProduction: import.meta.env.PROD,
  isTest: import.meta.env.MODE === 'test',
})

/** 当前选中的目标；未设置或值非法时回落 online。 */
export function getApiTarget(): ApiTarget {
  if (typeof localStorage === 'undefined') return 'online'
  return localStorage.getItem(API_TARGET_KEY) === 'local' ? 'local' : 'online'
}

export function setApiTarget(target: ApiTarget) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(API_TARGET_KEY, target)
}

/** 目标对应的 base URL；online 为空串表示同源。 */
export function getApiBaseUrlFor(target: ApiTarget) {
  return target === 'local' ? rawLocalApiBaseUrl : rawApiBaseUrl
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
