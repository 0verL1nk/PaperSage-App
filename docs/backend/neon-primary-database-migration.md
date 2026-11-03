# 主业务库迁移到 Neon PostgreSQL

## 当前状态（2026-10-08）

生产主库已切换到用户新建的 Neon PostgreSQL `papersage` database，旧 TiDB 数据不迁移。Vercel Production 的 `DATABASE_URL` 使用 pooled endpoint，`DATABASE_MIGRATION_URL` 使用同一 database 的 direct endpoint；本机 `server/.env` 也采用相同配置。首次 Go Function 部署时业务表尚未创建，注册因此返回 500；已通过 `go run ./cmd/migrate` 对 Neon 执行迁移，命令成功返回 `schema is up to date`。迁移完成后将 Vercel Production 的 `RUN_DB_MIGRATIONS` 明确设为 `false`，避免多个 Function 冷启动并发修改 schema。

2026-10-08 的生产日志还发现 pgx 预编译语句与 Neon 池化连接不兼容（`prepared statement name is already in use`）。Go PostgreSQL dialector 已切换为 `PreferSimpleProtocol`，随本次后续部署上线后再对注册链路复测。

## 当前架构与延迟线索

- Go 主库通过 `DATABASE_URL` 选择 PostgreSQL、MySQL/TiDB 或 SQLite；生产 `DATABASE_URL` 已切换为 Neon `papersage` database。
- `NEON_DATABASE_URL` 仍供独立 RAG/pgvector 能力使用，不与主业务 schema 混用。
- Neon 主库与 Vercel Go Function 均位于 `ap-southeast-1` / `sin1`；生产部署已确认 Function 区域为 `sin1`。
- PostgreSQL 运行时连接使用 pooler；独立迁移命令通过 `DATABASE_MIGRATION_URL` 走 direct endpoint。
- Go SQL 连接池原来硬编码每实例 50 条连接，Vercel 环境已有的 `DB_MAX_OPEN_CONNS`/`DB_MAX_IDLE_CONNS` 设置未生效。已改为读取环境变量，默认每实例最多 10 条、空闲 2 条，并限制连接存活 30 分钟，供 Neon Pooler 复用。

## 发现的迁移阻断项

1. `server/migrations/postgres/0009_embedding_binary.up.sql` 使用 PostgreSQL `bytea` 保存二进制嵌入；不与 RAG 库的 pgvector schema 复用。
2. 不要把业务主库和当前 RAG PostgreSQL 指向同一个 database。业务迁移的 `rag_chunks.embedding` 是二进制 `bytea`，独立 RAG 则使用 pgvector，复用会产生表结构冲突。历史版本的 `cmd/neoninit` 包含 `DROP TABLE IF EXISTS rag_chunks`，可能删掉已有数据；当前工作树已移除该语句，但尚未在真实 PG 验证或发布，切勿据此运行历史版本。
3. SQL 迁移原先随应用启动执行。现在可用 `RUN_DB_MIGRATIONS=false` 关闭请求实例上的迁移，并通过 `DATABASE_MIGRATION_URL` 指定直连地址、单独运行 `go run ./cmd/migrate`；应用运行时仍使用 Neon pooled URL。
4. 不导入 TiDB 历史数据；新业务库从空 schema 初始化，旧账号、项目和文档元数据不保留。

## 建议切换流程

1. 在当前 Neon Project/Branch 新建独立 PostgreSQL database（例如 `papersage_main`），保留现有 RAG database 和 `NEON_DATABASE_URL` 不变。Neon 支持一个项目包含多个数据库。
2. 在本机环境中设置新库的直连 `DATABASE_MIGRATION_URL`，执行 `go run ./cmd/migrate`；再用只含测试 schema 的直连 `TEST_PG_DSN` 运行 `go test -tags=integration ./internal/store/`，验证全新库迁移与重复迁移。不要对现有 RAG database 执行 `cmd/neoninit`。
3. 不导入 TiDB 历史数据，直接在新的业务 database 初始化空 schema；现有 Neon RAG database 和 `NEON_DATABASE_URL` 保持不变。
4. 用独立 Preview 环境设置新库连接，验证注册/登录、刷新令牌、项目与文件 CRUD、设置、Agent、上传下载和 RAG；检查 Vercel Function 到数据库的 p50/p95 延迟、连接池等待和冷启动。
5. 更新 Vercel Production 的 `DATABASE_URL` 指向新业务库，并部署；完成健康检查、注册/登录、项目和文件读写验证后开放使用。按用户决定，不设置回滚到旧 TiDB 的流程；旧库不再作为应用数据源。

## 当前验证

- `go run ./cmd/migrate`：2026-10-08 在新 Neon `papersage` database 成功完成；再次运行返回 schema up to date。
- `go test ./...`：通过。Go 连接池读取 `DB_MAX_OPEN_CONNS`、`DB_MAX_IDLE_CONNS` 和 `DB_CONN_MAX_LIFETIME_SECONDS`；迁移使用 direct URL，Function 请求连接使用 pooled URL。
- Vercel Production 的 `RUN_DB_MIGRATIONS` 已设为 `false`；schema 由单独迁移命令更新，避免冷启动竞争。
- 生产 `/api/v1/auth/email-domains` 返回 200。注册 500 的生产错误已定位为缺少业务表及池化连接上的 prepared statement 冲突；迁移与简单协议修复已完成，仍需部署新版 Function 后验证注册成功。

## 资料

- [Neon 连接字符串与连接池](https://neon.com/docs/connect/connection-pooling)：应用运行时使用 pooled URL；迁移等需要会话状态的维护任务应使用直接连接。
- [Neon 项目对象结构](https://neon.com/blog/neon-object-hierarchy)：一个 Neon Project 可包含多个 branch 和 database。
- [Neon 区域间延迟演示](https://neon.com/demos/regional-latency)：展示数据库区域与 Vercel Function 区域对查询往返的影响。
- [Vercel 区域配置](https://vercel.com/docs/regions)：Functions 默认区域为 `iad1`，Function 应尽量接近数据库；Hobby 可选单一区域。
