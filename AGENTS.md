# PaperSage 仓库指南

## 当前项目

PaperSage 是面向科研新手的论文写作辅助系统。当前主线为 React 19 + TypeScript + Vite 前端、Go + Gin + GORM 后端；生产构建将前端静态文件嵌入 Go 二进制。`latex_render/` 是独立运行的 Overleaf CLSI 派生服务。旧 Python/FastAPI 后端已经迁移，不是当前运行入口。

产品定位、现状和文档索引见 [docs/PROJECT.md](docs/PROJECT.md)。设计方案、课题材料与历史设计须区分，不要把计划描述成已经交付的功能。

## 目录导航

| 任务 | 位置 |
|---|---|
| 前端路由与页面 | `frontend/src/routes/`、`frontend/src/views/` |
| 前端 API 与状态 | `frontend/src/services/`、`frontend/src/store/` |
| Go 服务入口 | `server/cmd/papersage/` |
| 依赖装配 | `server/internal/bootstrap/` |
| API、中间件与静态站点 | `server/internal/httpapi/` |
| 领域模块 | `server/internal/`（auth、projects、documents、latex、settings、agent、rag 等） |
| 数据库 DDL 与迁移 | `server/schema/`、`server/migrations/` |
| LaTeX 编译服务 | `latex_render/` |
| 能力规格与变更提案 | `openspec/specs/`、`openspec/changes/` |
| 项目及课题文档 | `docs/` |

## 本地开发

仓库根目录 Makefile 提供常用命令：

```bash
make install       # 安装前端与 CLSI 依赖
make dev           # 启动本地服务
make stop          # 停止本地服务
make status        # 查看服务状态
make check         # 类型/静态检查
make test          # Go 单元测试与前端 Vitest
make build-server  # 构建前端并打包为 Go 二进制
```

单独运行前端：`cd frontend && pnpm dev`。单独运行后端：`cd server && go run ./cmd/papersage`。后端运行所需配置以 `server/internal/config/` 为准；AI、数据库、对象存储和 CLSI 等能力可能需要外部服务或环境变量。

## 项目约定

- API 通常使用 `{code, data, info}` 信封；认证沿用表单请求与 access/refresh 双 token。
- Go 依赖集中在 `internal/bootstrap.Build()` 装配，通过构造函数注入；新领域代码放入对应 `internal/<domain>/`。
- `server/schema/tables/` 是 SQL-first 表结构来源；数据库迁移放在 `server/migrations/`。
- 前端使用 TanStack Router/Query、Zustand、React Hook Form 与 Zod；修改前遵循目录内的 `AGENTS.md`。
- OpenSpec 用于较大功能变更：先读 `openspec/config.yaml` 与 `docs/agents/openspec-workflow.md`，将提案/规格与已实现行为分开。
- `docs/project/`、`docs/backend/` 中部分文档是迁移前的历史设计；阅读时看文档中的状态说明，并以当前代码为准。

## 验证

按改动范围运行对应命令。`make check` 和 `make test` 的实际目标定义在根 `Makefile`；E2E 用 `cd frontend && pnpm test:e2e`，需按 E2E 文档准备后端和依赖服务。不要将类型检查当成功能测试。
