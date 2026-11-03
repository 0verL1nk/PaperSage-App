import { describe, expect, it } from 'vitest'
import i18n from './index'

describe('document language', () => {
  it('matches the selected interface language', async () => {
    document.documentElement.lang = 'en'
    await i18n.changeLanguage('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')

    await i18n.changeLanguage('en-US')
    expect(document.documentElement.lang).toBe('en-US')
  })
})
