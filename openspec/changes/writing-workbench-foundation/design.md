## Context

- 仓库现状：React 19 + TypeScript 前端（`frontend/`）与 Go gin+GORM 后端（`server/`，单二进制、前端 embed）。已有文档上传/文件夹、LaTeX 编辑编译、用户设置；**没有**论文场景与项目级写作状态。
- 产品主线（`docs/project/12-ai-writing-design.md` §4、`docs/project/13-thesis-product-model.md` §4/§5.1）：两场景（本科毕业论文 / 入门级学术小论文）× 三阶段（写作准备 / 内容撰写 / 检查完善）；研究蓝图、课题要求、阅读与比较、章节计划等产物跨阶段持续保存、按需组合，不设强制编辑门槛。
- 已确认硬约束（2026-10-04）：三阶段交叉流转——撰写中可完善研究目标与资料库、检查中可回退重写；切换阶段不清空、不锁定；阶段完成度不得由聊天轮数推断。
- 前序变更：`2026-09-13-ai-agent-module` 交付聊天后端与（待完成）AI 面板；本变更不改动其协议，只提供后续 AI 任务的「场景 / 阶段 / 写作状态」上下文挂载点。
- 工程约定：后端按域分包 + 构造函数注入；SQL-first（`server/schema/tables/*.sql` 真相源）+ golang-migrate（pg/mysql 双份）+ SQLite AutoMigrate；前端 shadcn/ui + Tailwind 4 语义 token；服务层 URL 集中在 `api-routes.ts`。

## Goals / Non-Goals

**Goals:**
- 项目场景字段与创建时选择；存量项目与缺省请求兼容默认值。
- `ProjectWritingState` 持久化契约：GET 默认合成、PUT 乐观锁保存（revision）。
- 三阶段导航（准备 / 撰写 / 检查 + 资料库）与跨阶段交叉流转：无门槛、不清空、不锁定。
- 写作准备工作台（研究蓝图 + 课题要求 + 材料概览 + 待补项）与检查页骨架。
- 新页面与新导航严格沿用现有设计语言；测试与 e2e 证据齐全。

**Non-Goals:**
- 场景切换与差异预览（后续独立变更）。
- 阅读记录 / 文献比较、章节计划 / 章节摘要、规范库、建议与确认流、运行记录。
- AI 澄清向导（本变更只提供人工可编辑状态；AI 对话由 agent 变更承载）。
- 稿件版本快照与恢复、Word/多格式、跨章节检查的实际能力。

## Decisions

### D1 场景放在 `projects` 表
- 决策：`projects.scenario varchar(32) NOT NULL DEFAULT 'undergraduate_thesis'`，枚举校验在应用层。
- 理由：创建时即需确定；列表/详情查询与展示直接可用；写作状态 API 同时返回 scenario，前端单一数据源。
- 备选：放进 writing-state JSON → 创建时尚无状态行、列表需 JOIN；否。

### D2 写作状态独立表 + JSON 列
- 决策：新表 `project_writing_states`（`project_id` 主键/外键级联、`brief` json、`requirements` json、`phases` json、`current_phase` varchar、`revision` int、BaseModel 时间戳与软删除）。
- 理由：一项目一行；产物字段会随设计演进，JSON 列 + 读取端默认合并可向后兼容；软删除符合全表约定。
- 备选：全部关系化（字段尚不稳定、查询需求弱，过度设计）；并入 projects 表（膨胀且并发写放大）；否。

### D3 读写语义与乐观锁
- GET：行不存在 → 合成默认状态（`revision = 0`），不写库。
- PUT：body 携带完整状态 + `revision`；`revision = 0` 且无行 → 插入（1）；有行且匹配 → 更新（+1）；不匹配 → 409 + 服务端最新状态。
- 用整数 `revision` 而非 `updated_at` 比较：规避时钟精度与序列化格式歧义。
- 备选：PATCH 片段更新（字段多、并发语义复杂），推迟到确有需要。

### D4 三阶段模型
- `current_phase` ∈ {`prepare`,`write`,`review`}；`phases` 为三键映射（`not_started` | `in_progress` | `done`），默认 `prepare = in_progress`、其余 `not_started`。
- 切换阶段只更新 `current_phase`（随下次状态保存一并提交）；**不**改变 `phases` 的完成状态。
- 完成度 = 用户维护的 `phases`；禁止由聊天轮数/访问次数推断；UI 不显示百分比。

### D5 前端信息架构与路由
- 项目内导航（`WorkspaceLayout`）：资料库（保留 `/project/:id/uploading`）→ 写作准备（新 `/project/:id/prepare`）→ 内容撰写（保留 `/project/:id/writing`）→ 检查完善（新 `/project/:id/review`）。
- 项目根 `/project/:id` 默认重定向从 `uploading` 改为 `prepare`（阶段优先；资料库仍一键可达）。
- 移除 `classifying` / `logic-chain` / `generating` 占位路由，统一重定向到 `/project/:id/prepare`；移除 `ComingSoon` 用法与对应 i18n 占位词条。
- 备选：保留占位 tab 到后续变更（与主线 IA 冲突、答辩演示混乱）；否。

### D6 写作状态 JSON 形状（v1）
```json
{
  "brief": {
    "topic": "", "background": "", "questions": [],
    "goals": [], "scope_out": [],
    "objects_and_data": "", "methods": "",
    "key_chapters": [], "milestones": [], "constraints": [],
    "known_facts": [], "assumptions": [], "unverified": []
  },
  "requirements": [{ "id": "", "text": "", "source": "", "note": "" }],
  "phases": { "prepare": "in_progress", "write": "not_started", "review": "not_started" },
  "current_phase": "prepare"
}
```
- 读取时逐键合并默认值（容忍旧数据缺键）；PUT 时服务端校验枚举与体积上限（整状态 ≤ 256KB、数组 ≤ 200 条、单字符串 ≤ 20k 字符），非法 → 422。

### D7 材料概览与待补项
- 材料概览 = 现有 documents 列表 API 的只读汇总（计数、最近更新、进入资料库入口）；不在准备页做上传/确认（资料库已有）。
- 待补项 = 前端派生：brief 关键字段为空（topic/questions/goals/methods）、requirements 为空、项目无已确认文档；只提示、不阻断。
- 不落库、不参与 revision。

### D8 UI 设计语言（延续 + 参考）
- 沿用现有 token 与组件（`bg-background/card`、`border-border/40`、`rounded-2xl`、micro 标题 `text-[11px] font-bold uppercase tracking-widest`、lucide 图标、shadcn 原语）；不引入新依赖。
- 准备页布局：「检查清单（左，待补项/分组锚点）+ 编辑区（右，研究蓝图 / 课题要求 / 材料概览卡片）」；窄屏折叠为单列。
- 保存反馈复用「保存中 / 已保存 / 失败重试」micro 状态模式（与 `WritingPage` 的 `SavingStatus` 同语义），自动保存防抖约 800ms。
- 参考成熟实现（不照搬、不引依赖）：Vercel `ai-elements` 的组合式信息卡与状态徽标；Notion/Linear 式内联编辑与保存指示。取其「简洁信息密度 + 明确保存状态」，不引入组件库。

### D9 冲突处理 UX
- 409 时保留表单本地输入，顶部横幅提示「内容已在其他位置更新」，提供两键：
  - 「加载最新版本」丢弃本地输入；
  - 「基于最新版本覆盖保存」以 409 返回的 revision 重放本地内容。
- 服务端不做静默合并；冲突路径可测试。

### D10 迁移与数据
- `server/schema/tables/projects.sql` 增列；新增 `server/schema/tables/project_writing_states.sql`（MySQL 方言真相源）。
- `server/migrations/postgres/0011_writing_workbench.{up,down}.sql` 与 `server/migrations/mysql/0011_writing_workbench.{up,down}.sql`。
- `store.AllModels()` 注册新模型（SQLite AutoMigrate 路径）。
- DAO：`internal/store/query` 由 `make generate-dao SCHEMA_DSN=...` 生成（CI/开发机执行）；本变更多数读写可在服务层用 GORM 表达。

### D11 测试策略
- 后端契约测试：writingstate（默认合成 / 递增 / 409 载荷 / 404 越权 / 422 枚举与体积 / 旧数据逐键合并）；projects 补 scenario（创建、默认、返回）。
- 前端：`writing-state-api` 单测（URL、请求体、错误归一化）；准备页组件测试（编辑保存、待补项、409 冲突 UI）；导航测试（阶段跳转、重定向、根路径）。
- e2e：新建带场景项目 → 准备页编辑保存 → 刷新持久 → 撰写 ↔ 准备交叉跳转 → 检查页骨架可见。

## Risks / Trade-offs

- [移除占位路由破坏书签/e2e 断言] → 旧路径统一重定向到 prepare；同步检查 `tests/e2e/navigation/` 等断言。
- [默认落地页改为 prepare 改变既有动线] → 资料库 tab 一键直达；e2e 断言同步更新。
- [JSON 状态演进] → 读取端逐键默认合并，新增字段不需数据迁移。
- [双轨迁移（AutoMigrate vs golang-migrate）遗漏] → 任务清单硬性包含两处登记；PG 集成测试（TEST_PG_DSN）覆盖。
- [乐观锁在多标签下频繁 409] → 明示冲突 UI + 保留输入；单用户低并发下可接受。
- [阶段状态被质疑可伪造] → 不显示百分比、不自动推断；页面与文档口径一致（辅助导航，非验收依据）。

## Migration Plan

1. 迁移 0011 上线（加列 + 建表，无破坏性变更）。
2. 旧版本回滚时新表/列保留，旧代码忽略之；无需数据回填。
3. 前端路由重定向兜底随发布生效。

## Open Questions

- 待评审：移除三个占位 tab 与「默认落地写作准备页」是否接受（proposal 已标注 BREAKING / 动线变化）。
- 材料概览是否需要「确认入库」快捷操作（当前设计仅只读 + 跳转资料库）。
- 阶段状态是否允许三阶段均为 done 后继续编辑（本设计：允许，无锁）。
