# `frontend/src/services/` — Conventions

> 给人和 AI agent 的速查。修改这个目录前先读。

## 1. 模块结构

```
src/services/
├── client.ts              # axios 实例 + 拦截器 + envelope 处理
├── api-routes.ts          # 集中路由常量（*不要在 service 文件里写字符串 URL*）
├── contracts.ts           # ApiEnvelope 等共享类型
├── errors.ts              # FrontendApiError + 归一化
├── <domain>-api.ts        # 每个域一个文件：auth / projects / settings / ...
├── <domain>-api.test.ts   # 与 service 同级的 Vitest 单测
├── __tests__/             # 跨服务或更复杂的测试
├── token-refresh.test.ts  # 命名直陈，不带 -api 后缀
└── index.ts               # barrel export（@/services 入口）
```

**规则**：
- 加新域就新建 `<domain>-api.ts` + `<domain>-api.test.ts`，不要塞到现有文件
- URL 字符串统一在 `api-routes.ts` 拼装，service 文件只 `import { X } from './api-routes'`
- barrel 出口 `<domain>-api.ts` 必须被 `index.ts` 显式 re-export

## 2. Web Crypto 约束（重要）

> 来源：`auth-api.ts` 的 `js-sha256` fallback。2026-06 引入。

`window.crypto.subtle` 仅在 **secure context**（HTTPS 或 `localhost`）下可用。从局域网 IP 访问开发服务器时（HTTP），`crypto.subtle` 是 `undefined`，会抛 `TypeError`。

**模式：feature detect + 库 fallback**：

```typescript
import { sha256 as jsSha256 } from 'js-sha256'

async function sha256(message: string): Promise<string> {
  const msgBuffer = new TextEncoder().encode(message)
  if (crypto.subtle) {
    const hashBuffer = await crypto.subtle.digest('SHA-256', msgBuffer)
    const hashArray = Array.from(new Uint8Array(hashBuffer))
    return hashArray.map((b) => b.toString(16).padStart(2, '0')).join('')
  }
  // 非 secure context 下的 fallback：HTTP 经 IP 访问
  return jsSha256(message)
}
```

**两条不变量**：
1. 两条路径**必须产出相同的 hex 输出**——后端 bcrypt 拿到的就是 hex 字符串，路径差异不能改变哈希值
2. fallback 调用必须是**同步的纯函数**（js-sha256 满足），不能引入 await/Promise

**测试约束**（`auth-api.test.ts`）：
- 用 `vi.stubGlobal('crypto', { subtle: undefined })` 模拟非安全上下文
- 关键断言是"两条路径产出一致"，**不要**直接断言特定哈希值（会随 js-sha256 版本变化）
- 旁证：单独验证 `sha256WebCrypto(input) === jsSha256(input)`，确认库本身输出一致

**未来遇到同类约束**（`crypto.subtle` 之外的 Web API）：
- 任何只在 secure context 暴露的能力都要走同一 pattern：先 `if (crypto.X)`，fallback 写明原因
- 备选 fallback 库要选**纯 JS、零依赖、同步**的，避免 fallback 自身又依赖 `window`

## 3. API 响应信封

后端统一返回 `{code: number, data: T | null, info: string}`。**不要在 service 文件里手写 envelope 解包**——`client.ts` 的 `requestApi()` 已经做了。任何在 axios 直接 `await client.get(...)` 然后手 `.data.data` 的代码都是反模式。

**错误归一化**：所有抛出的错误都应该是 `FrontendApiError`（见 `errors.ts`）。`normalizeApiError()` 把 axios 错误、HTTP 错误、envelope code != 200 三类统一到同一形态，方便上层 `try/catch` 或 toast。

## 4. 测试约定

- **每个 service 文件必须有对应 `*.test.ts`**，位置同级
- mock 风格：和 `auth-api.test.ts` 一致，`vi.mock('@/services', async () => ({ ...actual, <fn>: vi.fn() }))`
- 不依赖真实后端：所有 HTTP 走 `vi.mock` 拦截
- 不依赖真实 localStorage：`setup.ts` 的 `afterEach` 会 `window.localStorage.clear()`
- **不要**用 `fetch` mock（项目统一 axios）；要测拦截器就直接 import `apiClient` 然后 `vi.spyOn`
- **不要**在测试里用真实的 sleep/timer——用 `vi.useFakeTimers()`

## 5. 提交时

- 加新 service 文件 → 同 commit 写测试 + 必要时更新本 AGENTS.md
- 修改 envelope 行为 → 同步检查所有 `*.test.ts` 里 envelope 形状的断言
- 加新路由 → 同步 `api-routes.ts` + 用 `pnpm test api-routes` 验证（routes 测试覆盖 URL 拼装）
