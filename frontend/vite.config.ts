/// <reference types="vitest/config" />
import { execFileSync } from 'node:child_process'
import { fileURLToPath, URL } from 'node:url'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { defineConfig, loadEnv } from 'vite'
import { resolveBuildVersion } from './src/lib/build-version'

const rootDir = fileURLToPath(new URL('.', import.meta.url))

function getGitValue(args: string[]) {
  try {
    const value = execFileSync('git', args, {
      cwd: rootDir,
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'ignore'],
    }).trim()
    return value || undefined
  } catch {
    return undefined
  }
}

const config = defineConfig({
  plugins: [
    react(),
    tailwindcss(),
  ],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:5478',
        changeOrigin: true,
      },
      '/health': {
        target: 'http://localhost:5478',
        changeOrigin: true,
      },
      '/texlive': {
        target: 'http://localhost:5478',
        changeOrigin: true,
      },
    },
  },
  build: {
    // Release builds ship minified code without source maps; never embed secrets.
    sourcemap: false,
    minify: true,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) {
            return
          }

          if (id.includes('/react/') || id.includes('/react-dom/')) {
            return 'vendor-react'
          }

          if (id.includes('/@tanstack/')) {
            return 'vendor-tanstack'
          }

          if (
            id.includes('/react-hook-form/') ||
            id.includes('/@hookform/') ||
            id.includes('/zod/')
          ) {
            return 'vendor-forms'
          }

          if (
            id.includes('/radix-ui/') ||
            id.includes('/sonner/') ||
            id.includes('/lucide-react/') ||
            id.includes('/class-variance-authority/')
          ) {
            return 'vendor-ui'
          }

          if (
            id.includes('/axios/') ||
            id.includes('/zustand/') ||
            id.includes('/clsx/') ||
            id.includes('/tailwind-merge/')
          ) {
            return 'vendor-utils'
          }
        },
      },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: './src/test/setup.ts',
    css: true,
    // Keep JSDOM/React suites from saturating CI hosts with many logical CPUs;
    // high worker counts caused otherwise trivial component tests to exceed 5s.
    maxWorkers: 4,
    // userEvent + jsdom on slow self-hosted ARM runners needs more than the 5s
    // default; a real regression still shows up as a failure, just later.
    testTimeout: 20_000,
  },
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
})

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, rootDir, 'VITE_')
  const gitCommit = getGitValue(['rev-parse', '--verify', 'HEAD'])
  const commit = env.VITE_GIT_COMMIT?.trim() || process.env.VERCEL_GIT_COMMIT_SHA?.trim() || gitCommit?.slice(0, 12)
  const version = resolveBuildVersion({
    explicit: env.VITE_APP_VERSION,
    tags: gitCommit ? getGitValue(['tag', '--points-at', 'HEAD']) : undefined,
    commit,
    gitAvailable: Boolean(gitCommit),
  })
  return {
    ...config,
    define: {
      __APP_VERSION__: JSON.stringify(version),
      __GIT_COMMIT__: JSON.stringify(commit || 'unknown'),
      __BUILD_DATE__: JSON.stringify(env.VITE_BUILD_DATE?.trim() || new Date().toISOString()),
    },
  }
})
