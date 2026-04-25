## Context

PaperSage 当前已经有前端 LaTeX 编辑探索和若干设计文档，但后端没有可用的多文件 LaTeX 编译服务。现有后端只实现了认证、项目与占位文档路由，无法支持真实模板工程的 PDF 编译、日志获取、词数统计和 SyncTeX。

参考实现 `.ref/overleaf/services/clsi` 已经具备完整的多文件 LaTeX 编译能力，并且在未开启 `SANDBOXED_COMPILES` 时可以走宿主机 `LocalCommandRunner`，直接执行本机 `latexmk`。这使得 PaperSage 可以在本地开发阶段不依赖 Docker，而在生产环境将同一 vendor 服务独立部署。约束在于：`clsi` 不是单一模块，而是依赖 Overleaf monorepo 的 workspace 包，因此需要按 vendor 服务整体引入，而不是只抄编译入口文件。

该改动跨越 vendor 代码、开发脚本、FastAPI 配置、服务层、API 路由和后续前端契约，属于典型的跨模块架构变更，需要在实现前先把边界和责任分清。

## Goals / Non-Goals

**Goals:**
- 将 Overleaf `clsi` 及其必要依赖以 vendor 形式引入仓库，并保留完整编译、输出产物、词数统计和 SyncTeX 能力。
- 支持开发者在 Ubuntu/Debian 和 macOS 上通过宿主机安装依赖运行本地 `clsi`，不要求本地 Docker。
- 为 FastAPI 增加稳定的 LaTeX 业务边界，前端只调用 PaperSage API，不直接依赖 vendor 路由。
- 支持多文件 LaTeX 工程，包括嵌套目录、BibTeX/Biber、图片资源和可选编译引擎。
- 为生产独立部署 vendor `clsi` 服务保留清晰的配置与调用方式。

**Non-Goals:**
- 本次不重写或重构 `clsi` 内部编译逻辑。
- 本次不引入本地 Docker 开发工作流作为默认路径。
- 本次不直接实现前端 LaTeX 页面完整交互，仅定义后端与服务边界。
- 本次不完成去 Overleaf 化清理，只为后续渐进式清理留出边界。

## Decisions

### 1. 采用“完整 vendor 服务 + 外层适配”，而不是自研编译服务

- **Decision**: 将 `services/clsi` 连同所需 `libraries/*`、workspace 根配置和 patches 一起 vendor 到 `latex_vender/`。
- **Why**: `clsi` 已覆盖多文件编译、输出文件、词数统计、SyncTeX 和编译控制。如果只借鉴 API 再自研实现，仍然要重写大部分复杂编译链。
- **Alternative considered**: 仅复制少量 `clsi` 文件到 PaperSage 后端或重写精简版编译服务。
- **Why not**: 依赖链不完整，且会导致重新发明复杂行为，风险和工作量都更高。

### 2. FastAPI 保持系统边界，前端不直接调用 vendor 服务

- **Decision**: 前端只访问 `/api/v1/projects/{project_id}/latex/...` 形式的 PaperSage API，由后端转调 `clsi`。
- **Why**: 这样可以复用项目权限控制、后端统一响应 envelope，并且以后更换或清理 vendor 服务不会影响前端契约。
- **Alternative considered**: 前端直连 `clsi`。
- **Why not**: 会把 vendor 路由、错误格式和输出 URL 暴露到前端，后续迁移成本更高。

### 3. 本地开发采用宿主机模式，生产采用外部部署模式

- **Decision**: 本地不启用 `SANDBOXED_COMPILES`，通过 `LocalCommandRunner` 直接执行宿主机 `latexmk`；生产环境由独立部署的 vendor `clsi` 服务承载。
- **Why**: 满足本地不用 Docker 的要求，同时避免在仓库内耦合生产部署方式。
- **Alternative considered**: 本地和生产都统一要求 Docker。
- **Why not**: 不符合当前开发体验要求，也会抬高日常开发门槛。

### 4. PaperSage 后端负责资源组装与权限控制，vendor 服务只负责编译

- **Decision**: 后端新增 `resource_builder` 和 `clsi_client`。`resource_builder` 负责将项目文件映射成 `resources[]`，`clsi_client` 负责与 vendor 服务交互。
- **Why**: 这样业务模型与第三方协议解耦，能够明确区分“项目访问”和“编译执行”两个责任面。
- **Alternative considered**: 路由直接读取项目资源并直接发 HTTP 请求到 `clsi`。
- **Why not**: 会把复杂映射和错误处理散落在路由层，难以测试和复用。

### 5. 保留完整 capability，而不是收缩到单次 PDF 编译 MVP

- **Decision**: 在 proposal/spec 阶段就把 compile、output、stop/clear-cache、wordcount、SyncTeX 都纳入要求。
- **Why**: 这次选择 vendor 整体引入，本身就是为了获得完整能力；若人为裁掉能力，会造成接口和部署形态反复调整。
- **Alternative considered**: 先只做“输入 tex 返回 pdf”。
- **Why not**: 与本次变更目标不一致，也不满足真实模板和后续前端交互需求。

## Risks / Trade-offs

- [Vendor 依赖体积大] → 先按最小可运行 workspace 引入，并将所有 PaperSage 特定逻辑放在 vendor 外层，后续逐步替换公共库。
- [宿主机模式缺少 Docker 隔离] → 明确仅限本地开发使用；生产环境必须通过外部独立部署服务承载。
- [跨平台安装复杂] → 将 `install` 与 `check` 分离，脚本优先验证命令存在与版本可用，而不是过度依赖单一包名。
- [多文件资源映射容易破坏路径语义] → 资源组装层必须完整保留相对路径和目录结构，并通过真实多文件 fixture 验证。
- [前后端直接依赖 vendor 错误格式] → 后端统一规范状态与响应 envelope，对前端屏蔽 vendor 细节。
- [后续去 Overleaf 化容易失控] → 先通过 `latex_vender` 与 `backend/services/latex` 边界固定责任，再逐步替换内部依赖。

## Migration Plan

1. 将 Overleaf `clsi` 所需最小工作区 vendor 到 `latex_vender/`，保证 `npm ci` 可运行。
2. 增加 `scripts/clsi/` 与 `Makefile` 目标，完成 Ubuntu/Debian 与 macOS 的本地宿主机安装、检查和开发启动流程。
3. 扩展后端配置，加入 `latex_clsi_base_url` 等设置，并默认指向本地开发服务地址。
4. 实现 `backend/services/latex/`，完成项目资源组装、vendor 服务调用和错误映射。
5. 新增 `backend/api/routes/latex.py` 与 schemas，暴露完整 PaperSage LaTeX API。
6. 使用真实多文件工程 fixture 验证编译、输出文件、词数统计和 SyncTeX 路由。
7. 更新部署和开发文档，明确本地与生产拓扑。

**Rollback strategy:**
- 若 vendor 导入导致主仓库构建受阻，可先移除 `latex_vender` 和相关 `Makefile`/脚本入口，保留 docs 与 OpenSpec 工件。
- 若 FastAPI 代理层不稳定，可通过配置关闭 LaTeX 能力入口，不影响现有认证、项目和上传模块。
- 若宿主机安装脚本不稳定，可先保留 `check` 和手动安装指引，再逐步恢复自动安装步骤。

## Open Questions

- PaperSage 项目文件当前的持久化模型是否已经足够承载 LaTeX 源文件、二进制图片和编译输出映射，还是需要单独的 LaTeX 文件模型。
- 编译输出文件是长期由 FastAPI 代理流式转发，还是后续落到对象存储并返回签名 URL。
- 是否需要在第一版中暴露基于 `user_id` 的隔离编译容器语义，还是先只支持项目级编译上下文。
