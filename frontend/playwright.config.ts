import { defineConfig, devices } from '@playwright/test'

const isCI = !!process.env.CI

export default defineConfig({
  testDir: './tests/e2e',
  // Re-enable parallel runs in CI; specs use unique resource names
  // (project id with UUID suffix) to avoid cross-spec clashes.
  fullyParallel: true,
  forbidOnly: isCI,
  // CI gets one retry to absorb transient browser/network flakes;
  // local runs fail fast so devs see the real error.
  retries: isCI ? 1 : 0,
  // CI runs 2 workers to shave ~40% wall time; local single-worker
  // keeps machine responsive.
  workers: isCI ? 2 : 1,
  timeout: 30_000,
  expect: { timeout: 5_000 },
  reporter: isCI
    ? [
        ['list'],
        ['github'],
        ['html', { outputFolder: 'playwright-report', open: 'never' }],
        ['json', { outputFile: 'playwright-results.json' }],
      ]
    : [
        ['html', { outputFolder: 'playwright-report' }],
        ['json', { outputFile: 'playwright-results.json' }],
      ],
  use: {
    baseURL: process.env.PLAYWRIGHT_BASE_URL ?? 'http://localhost:5173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 10_000,
    navigationTimeout: 30_000,
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
  // Auto-start the Vite dev server for local `pnpm test:e2e` runs.
  // In CI the workflow starts the server manually via nohup.
  webServer: !isCI
    ? {
        command: 'pnpm dev --port 5173',
        port: 5173,
        reuseExistingServer: true,
        timeout: 30_000,
      }
    : undefined,
})
