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
export async function discoverResourceProfile(origin: string, signal?: AbortSignal): Promise<string> {
  let response: Response
  try {
    response = await fetch(new URL('/texlive/profile', origin).href, {
      headers: { Accept: 'application/json' },
      signal,
    })
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw error
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
