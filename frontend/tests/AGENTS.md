# `frontend/tests/`

Test suites for the frontend. Two flavors live here: component/unit (Vitest) and E2E (Playwright).

## STRUCTURE

```
frontend/tests/
├── e2e/
│   ├── helpers.ts                 # shared Playwright helpers (auth, mocks, URLs)
│   ├── auth/                      # auth flow E2E (login, register)
│   └── features/                  # feature E2E (projects, compile, file-tree)
├── setup files live in src/test/  # Vitest jsdom + cleanup
```

Vitest spec files live next to their components (`**/__tests__/*.test.tsx` and `*.test.ts` co-located under `src/`). E2E specs are the only files under `tests/`.

## PLAYWRIGHT CONVENTIONS

### Run against a specific dev server

`playwright.config.ts` honors `PLAYWRIGHT_BASE_URL` (default `http://localhost:5173`). When 5173 is busy (e.g. another project occupies it), start PaperSage on a free port and pass the env var:

```bash
cd frontend && pnpm dev --port 5178 &
PLAYWRIGHT_BASE_URL=http://localhost:5178 pnpm exec playwright test
```

Vite auto-bumps the port if `--port` is busy; the env var keeps the spec in sync.

### Shared helpers

Import from `../helpers` (sibling of `auth/` and `features/`). Two required for the writing page:

- `seedAuthTokens(page)` — sets `papersage_access_token` / `papersage_refresh_token` in localStorage. **Must run before the first navigation to a protected route** (call it after `page.goto('/')`).
- `mockWritingPageAPIs(page)` — installs `page.route` mocks for `/auth/me`, `/projects/{id}`, and `/projects/{id}/latex-files/`. **Must run before `page.goto(WRITING_URL)`** so the mocks intercept the page's initial fetches.

If a test needs extra mocks (e.g. catch a DELETE), register them AFTER `mockWritingPageAPIs` and BEFORE `page.goto`. The most recently registered matching route wins; use `route.fallback()` to defer to the earlier mock.

### Glob pattern gotcha

`page.route('**/api/v1/projects/*/latex-files/*', ...)` matches BOTH the list endpoint (`.../latex-files/`, trailing slash) AND file paths (`.../latex-files/main.tex`). To catch only the latter, check the URL inside the handler:

```typescript
if (route.request().url().endsWith('/')) {
  await route.fallback()
  return
}
```

### Destructive actions are confirmed

Destructive keyboard shortcuts (Delete on a file/folder row) trigger a `confirm-dialog` BEFORE the API is hit. E2E tests must:

1. Press the shortcut, assert the confirm dialog appears
2. Assert the API was NOT called yet
3. Click the confirm button
4. Assert the API call landed

If you see a "确定删除 X 吗？此操作不可撤销。" dialog, that's the expected pattern. The first version of `file-tree.spec.ts` got this wrong and the test timed out — the API never got called because nobody clicked "删除".

### Selectors: prefer `data-testid` over text/role

File-tree components carry `data-testid` attributes added in 2026-06:

| Element | Testid | Notes |
|---------|--------|-------|
| Search input | `file-tree-search-input` | Inside `data-testid="file-tree-search"` root |
| Search clear button | `file-tree-search-clear` | Only present when value is non-empty |
| File row | `file-node` (with `data-file-path`, `data-active`) | `data-active="true"` when selected |
| Folder row | `folder-node` (with `data-folder-path`, `data-open`) | |
| Inline rename input | `inline-rename-input` | Visible only when `node.isEditing` |

Text-based selectors like `page.getByText('main.tex')` are fragile (changes with i18n). Use `getByTestId('file-node').filter({ hasText: 'main.tex' })` when you need to find a specific row by its visible label.

### Keyboard events need a focused element

After clicking a row, focus may stay on the row. The Delete/F2 handler in `writing-file-tree.tsx` is attached to a `tabIndex={0}` container. If `page.keyboard.press(...)` does nothing, focus the container first:

```typescript
await mainFile.evaluate((el) => {
  let cur: HTMLElement | null = el
  while (cur && cur.tabIndex < 0) cur = cur.parentElement
  if (cur) (cur as HTMLElement).focus()
})
await page.keyboard.press('Delete')
```

## VITEST CONVENTIONS (summary)

- Co-locate `*.test.ts(x)` files in `__tests__/` next to the component
- Use `@testing-library/react` for components, `vi.mock` for services
- `src/test/setup.ts` registers `@testing-library/jest-dom/vitest` and runs `cleanup()` in `afterEach`
- For Radix UI Select in jsdom: mock `@/components/ui/select` with a test-friendly version that exposes options as buttons (see `GeneralSettings.test.tsx` for the pattern)
- See `frontend/src/services/AGENTS.md` for the Web Crypto / `crypto.subtle` mock pattern

## TODO (future cleanup)

- The 4 pre-existing specs (auth/login, auth/register, features/compile, features/projects) inline the same `seedAuthTokens` + `mockWritingPageAPIs` logic. Refactor them to import from `tests/e2e/helpers.ts` to match the new convention.
- Add a `webServer` entry to `playwright.config.ts` so `pnpm test:e2e` boots vite automatically instead of requiring a manual `pnpm dev`.
