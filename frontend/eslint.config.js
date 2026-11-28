import js from '@eslint/js'
import globals from 'globals'
import reactHooks from 'eslint-plugin-react-hooks'
import reactRefresh from 'eslint-plugin-react-refresh'
import tseslint from 'typescript-eslint'
import { defineConfig, globalIgnores } from 'eslint/config'

export default defineConfig([
  globalIgnores(['dist']),
  {
    files: ['**/*.{ts,tsx}'],
    extends: [
      js.configs.recommended,
      tseslint.configs.recommended,
      reactHooks.configs.flat.recommended,
      reactRefresh.configs.vite,
    ],
    languageOptions: {
      ecmaVersion: 2020,
      globals: globals.browser,
    },
  },
  {
    files: ['src/components/ui/**/*.{ts,tsx}', 'src/app/router.tsx', 'src/routes/**/*.{ts,tsx}'],
    rules: {
      'react-refresh/only-export-components': 'off',
    },
  },
  // 用户可见文案禁用内部术语（口径见 docs/frontend/ux-copy.md）。
  // ReviewPage 仍由 HELE-70 第 2 批彻底替换，替换前保留豁免。
  {
    files: ['src/**/*.{ts,tsx}'],
    ignores: [
      'src/**/*.test.{ts,tsx}',
      'src/views/projects/ReviewPage.tsx',
    ],
    rules: {
      'no-restricted-syntax': ['error',
        { selector: 'Literal[value=/产物|会话|您|置信度|确定性/]', message: '界面文案避免内部术语，见 docs/frontend/ux-copy.md' },
        { selector: 'JSXText[value=/产物|会话|您|置信度|确定性/]', message: '界面文案避免内部术语，见 docs/frontend/ux-copy.md' },
        { selector: 'TemplateElement[value.raw=/产物|会话|您|置信度|确定性/]', message: '界面文案避免内部术语，见 docs/frontend/ux-copy.md' },
      ],
    },
  },
])
