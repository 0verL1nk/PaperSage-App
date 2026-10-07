# PaperSage 前端

React 19 + TypeScript + Vite 单页应用。页面和路由位于 `src/views/`、`src/routes/`，API 调用位于 `src/services/`，客户端状态位于 `src/store/`。

## 开发

```bash
pnpm install
pnpm dev
```

Vite 默认在 `http://localhost:5173` 提供页面。独立开发时需启动 Go 后端（默认 `http://localhost:5478`）；代理及 API 基址配置可从 `vite.config.ts` 与 `src/` 中对应配置核对。

## 命令

```bash
pnpm typecheck  # TypeScript 类型检查
pnpm lint       # ESLint
pnpm test       # Vitest 单元/组件测试，不含 E2E
pnpm test:e2e   # Playwright 端到端测试
pnpm build      # 类型检查并生成生产静态文件
```

Playwright 用例在 `tests/e2e/`，运行前需准备对应的后端及依赖服务。部署时前端构建产物由 Go 服务嵌入并同源提供；具体全栈结构见 [项目概览](../docs/PROJECT.md)。
