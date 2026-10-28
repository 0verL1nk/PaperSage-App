import { apiClient, type ApiRequestConfig } from '@/services/client'

export type FrontendBuildInfo = Readonly<{
  version: string
  commit: string
  buildTime: string
}>

export type BackendBuildInfo = Readonly<{
  status: string
  version: string
  commit: string
  buildTime: string
  goVersion: string
}>

export const frontendBuildInfo: FrontendBuildInfo = Object.freeze({
  version: __APP_VERSION__,
  commit: __GIT_COMMIT__,
  buildTime: __BUILD_DATE__,
})

/** /health is intentionally outside the API envelope. */
export async function fetchBackendBuildInfo(): Promise<BackendBuildInfo> {
  const config: ApiRequestConfig = {
    skipGlobalLoading: true,
    skipAuthRefresh: true,
    validateStatus: () => true,
    timeout: 5_000,
  }
  const response = await apiClient.get<BackendBuildInfo>('/health', config)
  if (
    response.status < 200 || response.status >= 300 || !response.data ||
    typeof response.data.status !== 'string' || typeof response.data.version !== 'string' ||
    typeof response.data.commit !== 'string' || typeof response.data.buildTime !== 'string' ||
    typeof response.data.goVersion !== 'string'
  ) {
    throw new Error('Backend version unavailable')
  }
  return response.data
}
