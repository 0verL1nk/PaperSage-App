# PaperSage

PaperSage 是面向科研新手的论文写作辅助系统，支持论文项目管理、LaTeX 写作与编译，并在持续建设 AI 写作辅助能力。当前代码由 React 19 + TypeScript 前端、Go + Gin 后端和独立的 LaTeX CLSI 服务组成；生产构建可将前端资源嵌入 Go 单二进制。

## 文档入口

- [项目概览、当前实现边界与文档索引](docs/PROJECT.md)
- [开发者仓库指南](AGENTS.md)
- [项目设计说明](DESIGN.md)
- [前端说明](frontend/README.md)
- [OpenSpec 变更流程](docs/agents/openspec-workflow.md)

`docs/project/`、`docs/backend/` 中保留了迁移前的设计资料；其中的旧 Python/FastAPI 描述不代表当前实现。以当前代码和 `docs/PROJECT.md` 的现状说明为准。

## 快速开始

需要 Node.js 与 pnpm、Go（版本见 `server/go.mod`），以及按具体功能配置的数据库、对象存储、模型服务或 CLSI。常用本地开发命令：

```bash
make install
make dev
```

前端默认使用 Vite 开发服务器（`http://localhost:5173`），Go API 默认监听 `5478`。也可以分别运行：

```bash
cd frontend && pnpm dev
cd server && go run ./cmd/papersage
```

需要哪些后端环境变量取决于启用的功能；请查看 `server/internal/config/` 和 `docs/PROJECT.md`，不要将开发默认值当作生产配置。

## 检查与测试

```bash
make check
make test
cd frontend && pnpm test:e2e
```

E2E 需先准备后端及其依赖服务。各 Make 目标的准确范围以根 `Makefile` 为准。

## 构建单二进制

```bash
make build-server
```

该命令先构建前端，再把产物放入 Go embed 目录并编译服务。LaTeX CLSI、数据库、Redis 和对象存储仍按部署配置运行。
