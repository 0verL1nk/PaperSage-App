## Why

PaperSage 需要一个可用于真实论文模板的 LaTeX 编译能力，而不是前端本地演示级别的编辑器预览。当前仓库缺少后端多文件 LaTeX 编译服务，无法支撑模板工程、BibTeX/Biber、图片资源、日志输出、PDF 产物获取以及后续的 SyncTeX 集成。

Overleaf 的 `clsi` 已经解决了多文件 LaTeX 工程编译、输出文件管理、词数统计和 SyncTeX 等核心问题。与其从零实现一套编译服务，不如把 `clsi` 作为 vendor 服务引入 PaperSage，在本地开发环境使用宿主机 TeX Live 运行，在生产环境单独部署，再由 PaperSage 后端提供稳定的业务边界。

## What Changes

1. Vendor Overleaf `clsi` 服务及其必要 workspace 依赖
   - 将 `services/clsi` 与其依赖的 `libraries/*`、根工作区配置一起引入仓库
   - 保持其完整编译、输出文件、词数统计和 SyncTeX 能力
   - 为后续去 Overleaf 化保留渐进清理空间

2. 为本地开发增加宿主机模式运行能力
   - 提供 `make install` / `make install-clsi` 安装 Node、TeX Live 与 `clsi` 所需依赖
   - 支持 Ubuntu/Debian 与 macOS
   - 提供 `make check-clsi` 与 `make clsi-dev`，在不使用 Docker 的前提下启动本地 `clsi`

3. 为 PaperSage 后端增加 LaTeX 编译代理能力
   - 增加后端配置，指向本地或外部部署的 `clsi`
   - 增加多文件工程资源组装逻辑，将 PaperSage 项目文件映射为 `clsi` 的 `resources[]`
   - 暴露 PaperSage 自己的 LaTeX API，而不是让前端直接依赖 vendor 服务接口

4. 为完整 LaTeX 工作流增加后端 API
   - 编译接口
   - 输出文件与 ZIP 获取接口
   - 停止编译与清理缓存接口
   - 词数统计接口
   - SyncTeX 正反向同步接口

5. 明确本地开发与生产部署边界
   - 本地开发使用宿主机 `latexmk` / TeX Live
   - 生产环境单独部署 vendor `clsi` 服务
   - FastAPI 作为前端唯一依赖的服务边界

## Capabilities

### New Capabilities
- `latex-compilation-service`: 基于 vendor `clsi` 提供多文件 LaTeX 工程编译、输出产物、编译控制与词数统计能力
- `latex-synctex-service`: 提供源码与 PDF 之间的 SyncTeX 双向同步能力
- `latex-local-dev-bootstrap`: 提供跨平台本地安装、检测和启动 LaTeX 编译服务的开发者工作流
- `latex-backend-api`: 为前端提供稳定的 PaperSage LaTeX 编译与产物访问 API

### Modified Capabilities
- 无

## Impact

- **vendor 代码**：新增 `latex_vender/`，包含 Overleaf `clsi` 及其必要 workspace 依赖
- **构建与开发脚本**：新增 `scripts/clsi/` 与 `Makefile` 目标，用于本地安装、检查与启动
- **后端配置**：扩展 `backend/app/config.py`，增加 `clsi` 服务配置
- **后端服务层**：新增 `backend/services/latex/`，负责资源组装、vendor 客户端调用、错误映射
- **后端 API**：新增 `backend/api/routes/latex.py` 与对应 schemas/tests
- **运维与文档**：更新部署与本地开发说明，明确本地宿主机模式和生产外部服务模式
