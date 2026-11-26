import { isDesktop } from '@/lib/desktop'
import { getApiTarget, getRequiredApiBaseUrl } from '@/lib/env'

/**
 * Origin that serves `/texlive/...` to browser engines. In desktop local mode
 * the supervised backend proxies it on the app origin; worker XHRs to
 * 127.0.0.1 are blocked as private-network requests.
 */
export function browserResourceOrigin(): string {
  // Throws LocalBackendUnavailableError before any worker starts; the compile
  // must fail fast instead of silently using the app origin.
  const apiBaseUrl = getRequiredApiBaseUrl()
  if (isDesktop() && getApiTarget() === 'local') return window.location.origin
  return apiBaseUrl || window.location.origin
}

/**
 * The backend embeds the profile, so clients ask for it instead of baking it
 * into a build and having to redeploy the frontend on every profile upgrade.
 */
const PROFILE_TIMEOUT_MS = 15_000

export async function discoverResourceProfile(origin: string, signal?: AbortSignal): Promise<string> {
  const timeout = AbortSignal.timeout(PROFILE_TIMEOUT_MS)
  const requestSignal = signal && typeof AbortSignal.any === 'function'
    ? AbortSignal.any([signal, timeout])
    : timeout
  let response: Response
  try {
    response = await fetch(new URL('/texlive/profile', origin).href, {
      headers: { Accept: 'application/json' },
      signal: requestSignal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
    if (error instanceof DOMException && error.name === 'TimeoutError') {
      throw new Error('无法获取 LaTeX 资源目录版本：服务 15 秒内没有响应，请检查网络或资源服务状态后重试')
    }
    throw new Error('无法获取 LaTeX 资源目录版本：网络不可用，请检查连接后重试')
  }
  if (!response.ok) {
    throw new Error(`无法获取 LaTeX 资源目录版本：服务返回 ${response.status}`)
  }
  const payload: unknown = await response.json().catch(() => null)
  const profile = payload && typeof payload === 'object' ? (payload as { profile?: unknown }).profile : undefined
  if (typeof profile !== 'string' || profile === '') {
    throw new Error('无法获取 LaTeX 资源目录版本：服务响应缺少 profile')
  }
  return profile
}
