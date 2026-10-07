> 迁移号按落地顺序确定（默认 `0012`）；迁移树里尚无 `0011`。

## 1. 回归用例先行

- [x] 1.1 补 `latex_files` 回归用例：创建 → 删除 → 同名重建 → 断言成功（先跑红：重建返回 400）
- [x] 1.2 `agent_wiki_pages` 同类用例 **不再需要**：该表已定为硬删（design D6），不存在"已删行占位唯一键"的前提
- [x] 1.3 补 `agent` 会话删除用例：删除后消息与列表均不可见、越权删除被拒、且**行仍在**（软删而非物理删）

## 2. 软删机制与唯一约束

- [x] 2.1 `store.SoftDeleteTime`：DeleteClauses 在同一条 UPDATE 内写 `deleted_at` + `deleted_seq`
- [x] 2.2 加列 `deleted_seq`：`latex_files`（`agent_wiki_pages` 改硬删，不需要）
- [x] 2.3 唯一键重建：`latex_files(project_id, file_path, deleted_seq)`
- [x] 2.3a `users` 表加注释记录已知限制（删除路径立项时一并解决）
- [x] 2.4 迁移中回填存量已软删行的 `deleted_seq`（由 `deleted_at` 派生）
- [x] 2.5 `schema/tables/*.sql` 真相源同步 + `store` 模型标签更新
- [x] 2.6 单测：反复删建同一路径；活跃行唯一性仍生效（既有「重复创建 → 400」）

## 3. 删除语义分层

- [x] 3.1 `AgentConversation` / `AgentMessage` 补齐软删字段
- [x] 3.2 `deleteConversation` 改 GORM 软删 + 同事务软删消息（软删不触发 FK 级联）
- [x] 3.2a 修正裸表名查询绕过软删：`listConversations` / `listMessages` 由 `.Table(...)` 改走 `.Model(...)`
- [x] 3.3 `agent_messages` 加索引 `(conversation_id, created_at)`
- [x] 3.4 `agent_conversations` 加索引 `(project_id, user_id, updated_at)`
- [x] 3.5 契约测试：删除会话后列表不含该会话；消息不可见（同 1.3）
- [x] 3.6 `documents.deleteFolder` 修复：软删前显式把文档 `folder_id` 置空
  （原实现依赖 `ON DELETE SET NULL`，而软删是 UPDATE，外键永不触发 → 文档从 UI 消失）
- [x] 3.7 `folders.parent_id` 的 `ON DELETE CASCADE` 同样不会触发：`deleteFolder` 改为按层展开整棵子树，
  解绑各级文档后一并软删全部文件夹（文件夹无改父级接口，树不可能成环）；测试一并覆盖两层嵌套

## 4. 时间戳统一

- [x] 4.1 移除裸 `now()`（`internal/agent/service.go` 会话 `updated_at` 改应用层 UTC）
- [x] 4.2 全库审查并统一：`internal/auth` 五处写入 + 两处比较改 UTC；
  `gorm.Config.NowFunc` 固定 UTC（`autoCreateTime` / `autoUpdateTime` 默认取本地时间）
- [x] 4.3 **有意不做**：为「会话 `updated_at` 前进」写断言需依赖 sleep 的时序断言（脆弱）；
  而真正要防的是"裸方言 SQL 复现"，全库 grep 又会误伤 `rag/store.go` 中被方言判断
  合法保护的 `NOW(6)`。收益不足，改为在 design D3 记录约定 + 代码评审把关。

## 5. RAG 索引代次

- [x] 5.1 `rag_chunks` 加 `generation` + 索引 `(project_id, generation)`
- [x] 5.2 新增 `rag_index_generations`（`UNIQUE(project_id, generation)`、`INDEX(project_id, status)`）
- [x] 5.3 迁移为每个已有项目补 `generation = 1, status = 'active'` 的代次行
- [x] 5.4 写入侧：`WriteGeneration`（构建中优先，否则活动代次）+ `BeginGeneration` / `ActivateGeneration`（条件更新）/ `FailGeneration`
- [x] 5.5 检索侧：只命中活动代次（向量内存索引、BM25 索引、库端向量下推、BM25 行取数四条路径全部过滤）
- [x] 5.6 单测：代次生命周期——构建中不参与检索、切换后只命中新代次、旧代次可回查、重复切换报错、失败代次不影响活动代次

## 5b. RAG 表漂移修复（design D6）

- [x] 5b.1 `agent_wiki_pages` 三方对齐：补 `meta`；移除 `deleted_at`（改硬删）
- [x] 5b.2 `agent_wiki_pages` 保持 `UNIQUE(project_id, title)` 不变（硬删后无需 `deleted_seq`）
- [x] 5b.3 新增 `store.RegisterModels(...)` 注册口；`rag` 包在 `init()` 注册其模型
- [x] 5b.4 回归用例：全新 SQLite 库必须建出 `rag_chunks` / `agent_wiki_pages` / `rag_index_generations`
- [x] 5b.5 漂移检查测试：逐表比对 Go 模型与 `schema/tables/*.sql` 的列集合（首次运行即揪出 8 处真实漂移）
- [x] 5b.6 `cmd/neoninit` 的 `rag_chunks` DDL 补 `generation` 与索引

## 6. 迁移与验收

- [x] 6.1 双方言迁移 `{postgres,mysql}/0012_data_integrity.{up,down}.sql`
- [x] 6.2 `store.AllModels()` 覆盖新模型
- [x] 6.3 PG/MySQL 集成测试的业务表计数同步为 16（新增 `rag_index_generations`）
- [x] 6.4 验收（后端）：`make check-backend`（gofmt + vet + 编译）通过；`make test-backend` 全绿；
  `-tags=integration` 亦能编译。
  **覆盖率门槛未达标且非本次引入**：按 CI 同口径本地实测，HEAD 基线 **43.5%**、本次改动后 **43.8%**
  （略微提升），而 `.github/workflows/server-ci.yml` 卡的是 ≥80%。即 80% 门槛在本仓库 HEAD 上本就未满足，
  与本次改动无关；已用独立 worktree 取基线对照后确认。
  前端未改动，故未跑 `check-frontend`。
  另注：`CLAUDE.md` 文档的 `make check` / `make test` 目标并不存在（Makefile 只有 `check-backend` /
  `check-frontend` / `test-backend` / `test-frontend`），已同步修正文档。
- [x] 6.5 文档：`CLAUDE.md` 删除语义分层与软删陷阱约定；`docs/project/12-ai-writing-design.md` §6.3 落地状态同步

## 7. 对抗性审查（Workflow：5 维度发现 → 3 视角复核 → 完整性批评）

19 条发现、8 条被复核否定。确认项的处置：

- [x] 7.1 `listMessages` 的消息查询仍用 `.Table("agent_messages")`（同一 handler 的归属校验已改 `.Model`）→ 改为 `.Model(&store.AgentMessage{})`
- [x] 7.2 **SQLite 路径下本次修复实际不生效**（复核者已复现）：AutoMigrate 既不回填新列，也因索引同名而不重建旧的窄唯一键 → 新增 `store.sqliteRepair`（回填 + 重建索引，幂等）+ 针对老库的回归用例 `TestSQLiteRepairFixesLegacyUniqueIndex`
- [x] 7.3 down 迁移无法在用过新行为后执行（重复键）→ 两个方言的 down 均先删除已软删行（旧 schema 表达不了，已在注释说明）
- [x] 7.4 纯 Neon 模式（`NewStore(nil,…)`）下生成代次解引用空 DB 会 panic → `ActiveGeneration`/`WriteGeneration` 加 nil 守卫返回 0；Pg SQL 以 `(gen <= 0 OR generation = gen)` 容忍
- [x] 7.5 会话删除用例只断言行数，未证明消息被软删 → 补 `DeletedAt.Valid` 断言
- [x] 7.6 子文件夹悬挂（审查快照早于本次修复）→ 已由 3.7 覆盖
- [x] 7.10 迁移号空洞（0012 已用而 0011 空缺）的潜在风险 → 复核判定本变更不可达；已在 `writing-workbench-foundation/tasks.md` 记录其迁移号须在落地时顺延

**范围外登记（不是本变更的交付项，故不计入完成度）**：

- 生成 DAO 漂移：`latex_files.gen.go` 尚无 `DeletedSeq`。`query.LatexFile` **当前无任何业务调用**，非活 bug；`make generate-dao` 需真实 DB，留待下次带库执行时同步。
- `schema_drift_test` 只比对列名集合，不覆盖索引 / 唯一键 / 外键 / 默认值 / 类型——已知局限（本次仍据此揪出 8 处真实列漂移）。
- `rag` 的代次管理函数（`BeginGeneration` / `ActivateGeneration` / `FailGeneration`）暂无生产调用方：设计如此，触发调度（来源变更 → 重建）已在 §6.3 落地状态中标注为未做。
