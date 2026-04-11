import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'
import { clearSessionTokens, resetGlobalLoading } from '@/shared/stores'

afterEach(() => {
  cleanup()
  clearSessionTokens()
  resetGlobalLoading()
  window.localStorage.clear()
})
