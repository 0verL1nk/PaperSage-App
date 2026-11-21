# 桌面客户端与 Web 的稳定契约

桌面端（Wails）只是 **打包壳 + 本地进程监管 + 原生能力**。Web 应用的功能开发默认不需要改动桌面代码；本文列出少数必须保持一致的地方，改动这些点时才需要同步桌面端。

## 1. 前端如何进入桌面包

`desktop/build/Taskfile.yml` 的构建任务会调用 `common:build:frontend`：执行前端 `pnpm build`，把 `frontend/dist` 复制到 `desktop/internal/assets/dist`。新增页面、路由、组件、静态资源都会自动进入下一次桌面构建，**无需修改桌面侧**。

CI 的 `Desktop` 工作流在 PR 阶段也会执行 `task package`，真实编译 NSIS 安装器，因此前端构建失败会在 PR 上暴露。

## 2. 桌面必须满足的运行时假设

| 契约 | 说明 |
| --- | --- |
| SPA fallback | 除 `/texlive/*` 外的未知路径都返回 `index.html`，所以新增客户端路由无需桌面配置。 |
| `/texlive/*` 同源代理 | 桌面把 `/texlive/*` 代理到本地监管的后端；Web 侧不要把这些请求改写为其它前缀。 |
| 后端地址来自绑定 | 桌面上使用 `DesktopService.AppInfo.localApiBaseUrl`（或 `LocalBackendURL`），不要用 `window.location` 拼 API 地址。`frontend/src/lib/env.ts` 已封装。 |
| 桌面检测 | 以 Wails 运行时（`window.runtime` / `Call.ByName`）为准；浏览器环境必须继续可用且不依赖 Vite 代理。 |
| 深链路由形状 | `papersage://project/<id>/<stage>` 映射到 `/project/<id>/<stage>`（`desktop/internal/app/app.go` 的 `deepLinkPath`）。修改项目路由结构时需同步这一处。 |
| 绑定方法名 | `DesktopService` 的方法名通过 `Call.ByName` 直接调用，属于契约；重命名需要同步桌面与前端。 |
| 事件名 | `papersage:open-update`、`desktop:open-update`、`desktop:update-error`、`desktop:update-progress`、`desktop:files-dropped`、`desktop:deeplink`、`desktop:sync-change`。 |

## 3. 新功能放在哪一侧

- 默认按 Web 实现；桌面端不做业务逻辑。
- 需要原生能力时，在 `desktop/internal/services/service.go` 增加方法，并在 Web 侧用 `isDesktop()` 提供浏览器降级路径。
- 桌面特有 UI（标题栏、更新对话框）放在 `frontend/src/components/desktop/`，通过 `lib/desktop.ts` 访问绑定，不直接散落 `Call.ByName`。

## 4. 验证清单

- Web：`pnpm typecheck`、`pnpm lint`、`pnpm test`。
- 桌面：`go -C desktop test ./internal/...`；`Desktop` 工作流会额外跑真实安装器打包。
- 涉及更新流程时：`desktop/internal/updater` 的契约测试覆盖版本门禁、安装包选择、句柄锁定与暂存目录位置。
