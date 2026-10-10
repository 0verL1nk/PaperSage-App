import { defineConfig, devices } from '@playwright/test'
import { dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

/** Run direct browser WASM compiler acceptance without the authenticated app setup. */
export default defineConfig({
  testDir: dirname(fileURLToPath(import.meta.url)),
  testMatch: '*.spec.ts',
  workers: 1,
  timeout: 15 * 60_000,
  reporter: 'list',
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:5188',
    ...devices['Desktop Chrome'],
  },
})
