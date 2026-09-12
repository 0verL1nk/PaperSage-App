# Tasks: 后端 Go 迁移与单二进制交付

> 并行开发原则：在 `server/` 完成 E2E 验收（第 9 组）之前，`backend/` 与相关 CI 保持原样不动。

## 1. Go 模块骨架

- [x] 1.1 初始化 `server/go.mod`（module `papersage/server`），建目录骨架（cmd/internal/migrations/web）
- [x] 1.2 `internal/config`：env 配置加载（caarlos0/env + godotenv），变量名与 Python 版 1:1，含校验
- [x] 1.3 `internal/httpapi`：chi 装配、信封 `{code,data,info}` 序列化、错误 handler 逐字段复刻（404/405/422/500 + panic 兜底；注：Python 校验处理器实际返回固定中文文案，不暴露 detail 数组）
- [x] 1.4 日志（slog 结构化日志）、`/health` 端点、优雅停机（SIGTERM → 排空）
- [x] 1.5 `server/web/assets/.keep` 占位 + `go:embed all:assets` 编译通过（`web/embed.go`）
- [x] 1.6 新增 `.github/workflows/server-ci.yml`（gofmt 检查 / go vet / golangci-lint / go test，SQLite 免 CGO）

## 2. 数据层

- [x] 2.1 GORM 模型 ×8（users、user_settings、project_groups、projects、folders、documents、latex_files、compile_jobs），字段/约束对齐 SQLAlchemy 定义
- [x] 2.2 golang-migrate 集成：迁移 SQL embed、启动自动 up、Alembic `alembic_version` 交接逻辑（design D3）
- [x] 2.3 baseline 迁移：从 Alembic head 生成 `0001_baseline`（以模型为 schema 真相生成；Alembic 链含 SQLite 形状迁移无法在 PG 干净重放，与现库 diff 验证挪到 9.5）
- [x] 2.4 `internal/cache`：Redis（go-redis v9）/ 内存 TTL 双实现 + 健康检查语义
- [x] 2.5 DTO 层时间/UUID 序列化对齐（ISO 格式与现网逐字段比对，写快照测试）

## 3. 认证域

- [x] 3.1 `internal/auth`：golang-jwt v5 签发/校验（HS256、同 claims、access/refresh 双类型）
- [x] 3.2 bcrypt 校验（与存量哈希兼容的回归测试：用 Python 生成的哈希在 Go 侧验证）
- [x] 3.3 POST /auth/login、/auth/register（表单体）、/auth/refresh（轮换）、/auth/logout（缓存黑名单）
- [x] 3.4 确认邮箱验证未接线任何路由（grep 证实），email 包为休眠基础设施 — 移交切换阶段一并处理
- [x] 3.5 `get_current_user` 中间件（对应 api/dependencies.py）
- [x] 3.6 测试：对齐 test_auth 相关用例（含错误形状断言）

## 4. 项目与分组域

- [x] 4.1 projects CRUD + 分组（groups CRUD）+ trash 恢复/删除端点
- [x] 4.2 folders 域（含 rename、层级查询）
- [x] 4.3 项目创建时 LaTeX 模板初始化（对齐 test_project_template）
- [x] 4.4 测试：对齐 test_projects_routes / test_folder_service

## 5. 文档与存储域

- [x] 5.1 `internal/storage`：aws-sdk-go-v2 S3 客户端（自定义 endpoint、path-style、预签名、并行拉取）
- [x] 5.2 documents 域：上传（multipart）、列表/筛选、预览、备注、移动、删除（trash 语义）
- [x] 5.3 wordcount 等派生端点
- [x] 5.4 测试：契约测试用注入的内存 Storage 替身；S3 真实协议验证在 E2E（MinIO）覆盖

## 6. 设置域

- [x] 6.1 user_settings 服务与端点（GET 全量/单分类、PATCH 合并、PUT 替换、DELETE 重置，默认值合并语义）
- [x] 6.2 测试：对齐 test_settings_* 三件套

## 7. LaTeX 域

- [x] 7.1 latex_files CRUD（`/{file_path:*}` 通配路由、rename/move）
- [x] 7.2 `internal/latex/clsi`：CLSI HTTP 客户端移植（编译、输出 zip、cache 清理）
- [x] 7.3 编译队列：goroutine 信号量池（并发 3）、compile_jobs 状态机、启动恢复、过期清理
- [x] 7.4 sync/pdf、sync/code 增量同步端点
- [x] 7.5 测试：httptest 假 CLSI（对齐 test_clsi_client / test_compile_* / test_latex_*）

## 8. 前端嵌入与单二进制

- [x] 8.1 构建联动：Makefile `make build`（pnpm build → 复制 dist → go build）
- [x] 8.2 `server/Dockerfile`：三阶段（pnpm deps/build → go build with embed → distroless），产出单镜像
- [x] 8.3 静态服务：embed FS 路由、`/assets/*` immutable 缓存、index.html no-cache、SPA fallback、Content-Disposition expose
- [x] 8.4 同源模式验证：不设 VITE_API_BASE_URL，全站走同源相对路径
- [x] 8.5 交叉编译验证 CGO_ENABLED=0 单目标产物；多平台 release 附带移交发布流程

## 9. E2E 与验收

- [x] 9.1 `server/cmd/seed`：移植 seed_e2e_user.py（用户 + MinIO 种子文件）
- [x] 9.2 e2e.yml 直接构建并运行 Go 二进制（auth/projects 套件零改动通过；1 个用例在前端拦截器层失败，Python 侧同样失败，与后端无关）
- [x] 9.3 `e2e.yml` 增加对 Go 二进制的矩阵（或切换），MinIO/CLSI 步骤复用
- [x] 9.4 存量 token 兼容：python-jose 签发固件在 Go 单测互验通过（internal/auth/token_test.go）
- [x] 9.5 Alembic 交接实测：带数据的 SQLite/PG 库被 Go 启动接管，数据完好

## 10. 切换与收尾

- [x] 10.1 删除 `backend/`、`frontend/Dockerfile`；`docker.yml` 改单镜像构建推送
- [x] 10.2 `deploy/docker-compose.yml`：backend+frontend 合并为 `papersage` 单服务（clsi/db/redis 保留）
- [x] 10.3 Makefile：backend/frontend 目标合并为 `make server` / `make dev`（Vite dev + Go 后端）
- [x] 10.4 更新 CLAUDE.md、docs/（PROJECT.md 技术概览、backend 设计文档归档说明）
- [x] 10.5 删除 backend CI 工作流，README/部署文档更新
- [x] 10.6 归档本 OpenSpec 变更

## 追加约束（2026-09-05 用户要求）

- [x] 测试覆盖率 ≥80%（CI 门槛；当前 83.9%）
- [x] 依赖注入便于测试（Service 构造注入 db/cache/storage/cfg；testutil 内存替身）
- [x] 测试须为真单元测试：断言基于 Python 契约（行为规格）而非 Go 实现输出；已删除同义反复用例（TestBaselineVersion/TestUUIDDataType）
