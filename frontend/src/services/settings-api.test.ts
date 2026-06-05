import { describe, expect, it, vi, beforeEach } from 'vitest'
import { getJson, putJson, requestApi } from './client'
import {
  getSettings,
  getCategorySettings,
  updateCategory,
  replaceCategory,
  resetCategory,
} from './settings-api'
import { SETTINGS } from './api-routes'

vi.mock('./client', () => ({
  getJson: vi.fn(),
  putJson: vi.fn(),
  requestApi: vi.fn(),
}))

describe('settings api', () => {
  beforeEach(() => {
    vi.mocked(getJson).mockReset()
    vi.mocked(putJson).mockReset()
    vi.mocked(requestApi).mockReset()
  })

  describe('getSettings', () => {
    it('calls GET on the all-settings route and unwraps .settings', async () => {
      vi.mocked(getJson).mockResolvedValue({ settings: { foo: 1 } })
      const result = await getSettings()
      expect(getJson).toHaveBeenCalledWith(SETTINGS.all)
      expect(result).toEqual({ foo: 1 })
    })
  })

  describe('getCategorySettings', () => {
    it('calls GET on the category route', async () => {
      vi.mocked(getJson).mockResolvedValue({ category: 'ui', settings: { theme: 'dark' } })
      const result = await getCategorySettings('ui')
      expect(getJson).toHaveBeenCalledWith(SETTINGS.category('ui'))
      expect(result).toEqual({ theme: 'dark' })
    })
  })

  describe('updateCategory', () => {
    it('calls PATCH on the category route with wrapped settings', async () => {
      vi.mocked(requestApi).mockResolvedValue({ category: 'ui', settings: { theme: 'dark' } })
      await updateCategory('ui', { theme: 'dark' })
      expect(requestApi).toHaveBeenCalledWith({
        method: 'patch',
        url: SETTINGS.category('ui'),
        data: { settings: { theme: 'dark' } },
      })
    })
  })

  describe('replaceCategory', () => {
    it('calls PUT on the category route with wrapped settings', async () => {
      vi.mocked(putJson).mockResolvedValue({ category: 'ui', settings: { theme: 'dark' } })
      const payload = { theme: 'dark' as const, language: 'en-US', sidebar_collapsed: false }
      await replaceCategory('ui', payload)
      expect(putJson).toHaveBeenCalledWith(SETTINGS.category('ui'), { settings: payload })
    })
  })

  describe('resetCategory', () => {
    it('calls DELETE on the category route', async () => {
      vi.mocked(requestApi).mockResolvedValue({ category: 'ui', settings: {} })
      await resetCategory('ui')
      expect(requestApi).toHaveBeenCalledWith({
        method: 'delete',
        url: SETTINGS.category('ui'),
      })
    })
  })
})
