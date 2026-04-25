## 1. Vendor CLSI Workspace

- [x] 1.1 将 `.ref/overleaf` 中运行 `clsi` 所需的根 `package.json`、`package-lock.json`、`patches/`、`libraries/` 与 `services/clsi/` vendor 到 `latex_render/`
- [x] 1.2 为 `latex_render/` 添加 PAPERSAGE_VENDOR.md，记录来源、引入范围和后续去 Overleaf 化策略
- [x] 1.3 运行 `cd latex_render && npm ci`，验证 vendor workspace 可以完成依赖安装
- [x] 1.4 以宿主机模式启动一次 `latex_render/services/clsi/app.js`，验证基础服务可启动（需 patch import.meta.main 兼容 Node 22）

## 2. 本地开发安装与启动流程

- [x] 2.1 在 `Makefile` 中新增 `install-clsi`、`check-clsi`、`clsi-dev` 目标，并将 `make install` 串联到 LaTeX 服务安装流程
- [x] 2.2 创建 `scripts/clsi/install.sh`，支持 Ubuntu/Debian 安装或校验 Node、TeX Live 与宿主机工具依赖
- [x] 2.3 创建 `scripts/clsi/install.sh` 的 macOS 分支，支持 Homebrew 与 TeX 发行版依赖安装或校验
- [x] 2.4 创建 `scripts/clsi/check.sh`，校验 `node`、`npm`、`latexmk`、TeX 引擎、`biber`、`qpdf`、`texcount`、`inkscape` 等命令可用
- [x] 2.5 创建 `scripts/clsi/env.sh` 与 `scripts/clsi/dev.sh`，在不启用 `SANDBOXED_COMPILES` 的情况下启动本地 `clsi`

## 3. 后端配置与 Vendor 客户端

- [x] 3.1 扩展 `backend/app/config.py`，增加 `LATEX_CLSI_ENABLED`、`LATEX_CLSI_BASE_URL`、`LATEX_CLSI_TIMEOUT_SECONDS` 等配置
- [x] 3.2 创建 `backend/services/latex/clsi_client.py`，封装 compile、stop、clear-cache、wordcount、sync、output-zip 接口调用
- [x] 3.3 为 `clsi_client.py` 编写测试，验证请求序列化、状态透传和 vendor 错误映射

## 4. 多文件资源组装服务

- [x] 4.1 创建 `backend/services/latex/resource_builder.py`，将项目文件组装为 `clsi` `resources[]`
- [x] 4.2 在资源组装中保留嵌套路径、主文件选择和二进制资源处理策略
- [x] 4.3 创建 `backend/services/latex/service.py`，串联项目权限检查、资源组装和 vendor 客户端调用
- [x] 4.4 为资源组装和服务层编写测试，覆盖多文件模板、编译引擎选择与缺失主文件错误

## 5. PaperSage LaTeX API

- [x] 5.1 创建 `backend/schemas/latex.py`，定义 compile、wordcount、SyncTeX 与输出文件响应模型
- [x] 5.2 创建 `backend/api/routes/latex.py`，实现项目级 compile、stop、clear-cache、wordcount、sync、output-zip 路由
- [x] 5.3 在 `backend/api/__init__.py` 中注册新的 LaTeX 路由，并保持现有 API envelope 风格一致
- [x] 5.4 为 LaTeX 路由编写鉴权测试，验证无权访问的请求不会被转发到 vendor 服务

## 6. 输出文件与 SyncTeX 能力接入

- [x] 6.1 实现后端管理的输出 ZIP 代理接口（CLSI 不提供单文件端点，改用 output.zip）
- [x] 6.2 实现 SyncTeX code-to-PDF 路由与服务调用
- [x] 6.3 实现 SyncTeX PDF-to-code 路由与服务调用
- [x] 6.4 实现词数统计接口

## 7. 端到端验证

- [x] 7.1 在 `backend/tests/fixtures/` 下添加一个真实多文件 LaTeX 模板工程 fixture
- [x] 7.2 编写集成测试，验证单文件和多文件模板可经 CLSI 成功编译并返回 `output.pdf`
- [x] 7.3 编写集成测试，验证 wordcount、output.zip 和 cache 接口在成功编译后可用
- [x] 7.4 运行后端测试与本地 `clsi` 联调，40/40 测试全部通过

## 8. 文档与部署边界

- [x] 8.1 更新 `docs/backend/DEPLOYMENT.md`，说明本地宿主机模式与生产独立部署模式
- [x] 8.2 更新 `backend/.env.example`，补充 LaTeX 服务相关配置说明
- [x] 8.3 补充本地开发说明，明确 `make install`、`make check-clsi`、`make clsi-dev` 的使用方法
- [x] 8.4 记录第一阶段保留的 Overleaf 依赖边界，为后续渐进清理建立清单
