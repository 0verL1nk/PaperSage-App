## Why

2026-10 数据层评审发现一批缺陷，与「AI 写作工作台」共用同一批表，故先修地基再建功能。

1. **逻辑删除与唯一约束冲突（已回归复现）**。`latex_files` 声明 `UNIQUE (project_id, file_path)` 且走软删除（`FilesService.DeleteFile` → GORM 置 `deleted_at`），而判重用的 `findByPath` 经 GORM 软删过滤后看不到已删行，随后 `Create` 撞唯一键。回归用例 `TestDeleteThenRecreateSamePath` / `TestRepeatedDeleteAndRecreate`（`server/internal/latex/files_test.go`）在实现前已跑红：**删除文件后重建同名文件返回 400**；错误经 `files.go` 的 `errText(400, err.Error())` 直出，**响应体里是原始数据库约束错误文本**。`users.username`、`users.email`、`agent_wiki_pages (project_id, title)` 属同一类问题。
   MySQL/TiDB 不支持部分唯一索引（`WHERE deleted_at IS NULL`）；把 `deleted_at` 纳入唯一键在 MySQL 下因唯一索引中 NULL 不去重而失效。故引入 `deleted_seq`。
2. **删除语义三层不一致**。`agent_conversations` / `agent_messages` 表有 `deleted_at`，Go 模型未嵌 `BaseModel`（无该字段），而 `deleteConversation` 用裸 SQL 硬删——表有列、模型没有、代码硬删，三层各说各话。
3. **时间戳三方言不一致**。应用层 `UPDATE agent_conversations SET updated_at = now()` 在 SQLite 下静默失败（`now()` 不是 SQLite 函数），错误被 `_ =` 丢弃，会话排序在开发环境等于没更新。
4. **索引与查询模式不匹配**。会话列表查询 `WHERE project_id AND user_id ORDER BY updated_at DESC` 只有 `project_id` 单列索引。
5. **RAG 索引无代次**。`rag_chunks` 没有 generation 列，`docs/project/12-ai-writing-design.md` §6.3 要求的「建立新索引代次—校验—切换活动代次」无从表达；`§6.2` 已登记的 RRF 融合缺陷也没有可回归的索引版本维度。
6. **`agent_wiki_pages` 三方漂移，`wiki_write` 在生产不可用**。Go 模型声明并使用 `meta` 列，而 PG/MySQL 迁移与 `schema/tables` 都没有该列 → `UpsertWiki` 在 PG/MySQL 上必然命中「列不存在」。反向地，`schema/tables` 声明了迁移与模型都没有的 `deleted_at`。同时 SQLite 路径下 `rag_chunks` / `agent_wiki_pages` **根本不会被创建**（`store.AllModels()` 未注册 rag 模型，`store.Open` 的 SQLite 分支只 AutoMigrate 它）。测试全绿是因为测试用例直接按 **Go 模型** `AutoMigrate`，掩蔽了与迁移的差异。

## What Changes

- **唯一约束引入 `deleted_seq`**：`latex_files(project_id, file_path)` 与 `agent_wiki_pages(project_id, title)` 的唯一键改为包含 `deleted_seq`。活跃行取 `0`，删除时写入单调非零值（unix 微秒），使删除后同名资源可重建，且可反复删建。机制由自定义软删类型在**同一条 UPDATE** 内同时写 `deleted_at` 与 `deleted_seq`，保持原子性。
  `users(username)` / `users(email)` **不在本变更内**：GORM 的 `uniqueIndex` tag 中一个字段只能属于一个索引，而这两个索引都需包含 `deleted_seq`，AutoMigrate 路径无法表达；且当前无任何删除用户的代码路径，该陷阱属假设。改为在表上注释记录，待用户注销功能立项时解决。
- **删除语义分层并统一**：项目 / 文件 / 文档域 = 软删除（回收站语义）；会话与消息 = 软删除且**保留运行记录**（评审决定：删除会话不销毁实验与审计证据）；消息随会话在同一事务内软删，不依赖 FK 级联（软删不触发级联）。
- **时间戳统一**：一律由应用层以 UTC 写入，移除裸 `now()`；数据库默认值仅作兜底，不作为业务时钟。
- **索引补齐**：`agent_conversations(project_id, user_id, updated_at)`、`agent_messages(conversation_id, created_at)`。
- **RAG 索引代次**：`rag_chunks` 增 `generation`；新增 `rag_index_generations` 记录代次状态、嵌入模型与维度。

## Capabilities

### New Capabilities
- `data-integrity`: 逻辑删除下的唯一性约束、删除语义分层、跨方言时间戳一致性、与查询模式匹配的索引。
- `rag-index-lifecycle`: 检索索引代次的建立、校验与活动代次切换。

### Modified Capabilities
<!-- 无：现有 spec 未约束逻辑删除的唯一性行为与索引代次 -->

## Impact

- 后端：`server/schema/tables/`（users / latex_files / agent_wiki_pages / rag_chunks 修改，新增 rag_index_generations）、`server/migrations/{postgres,mysql}/NNNN_data_integrity_*`、`server/internal/store`（`BaseModel` 软删机制、`AgentConversation`/`AgentMessage` 模型、`AllModels()`）、`server/internal/agent/routes.go`（会话删除）、`server/internal/rag`（代次写入与检索过滤）。
- 数据：唯一键重建 + 新列 + 新表；存量数据无破坏性变更（存量行 `deleted_seq` 默认 0；已软删行在迁移中回填非零值）。
- 前端：无（`agent_wiki_pages` / 会话软删对现有接口返回不可见）。
- 文档：`CLAUDE.md` 软删除约定段落、`docs/project/12-ai-writing-design.md` §6.3 状态同步。
- 迁移号：`0011` 已由 `writing-workbench-foundation` 占用，本变更按落地顺序取后续号（默认 `0012`）。
