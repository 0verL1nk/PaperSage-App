const rawApiBaseUrl = import.meta.env.VITE_API_BASE_URL?.trim() ?? 'http://localhost:5478'

export const appEnv = Object.freeze({
  apiBaseUrl: rawApiBaseUrl,
  mode: import.meta.env.MODE,
  isDevelopment: import.meta.env.DEV,
  isProduction: import.meta.env.PROD,
  isTest: import.meta.env.MODE === 'test',
})

export function getOptionalApiBaseUrl() {
  return appEnv.apiBaseUrl
}

export function getRequiredApiBaseUrl() {
  if (appEnv.apiBaseUrl) {
    return appEnv.apiBaseUrl
  }

  throw new Error(
    `Missing required environment variable "VITE_API_BASE_URL" for mode "${appEnv.mode}".`,
  )
}

export function assertRequiredAppEnv() {
  if (appEnv.isProduction) {
    getRequiredApiBaseUrl()
  }
}
