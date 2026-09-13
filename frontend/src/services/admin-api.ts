import { getJson, putJson } from './client'

export interface SystemSettings {
  allowed_email_domains: string[]
}

export function getSystemSettings() {
  return getJson<SystemSettings>('/api/v1/admin/settings')
}

export function putSystemSettings(kv: Partial<SystemSettings>) {
  return putJson<SystemSettings, Partial<SystemSettings>>('/api/v1/admin/settings', kv)
}
