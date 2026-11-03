## Why

PaperSage 已有项目工作台、LaTeX 编辑/浏览器编译、DeepAgent 与确认式改稿，但部署尚未正式切换，长文处理、稿件版本、证据新鲜度和科研写作产物仍有缺口。2026-10-29 用户要求把项目盘点中的 P0/P1/P2 工作全部纳入 OpenSpec 并完成，不能只增加聊天能力或把测试替身当成线上验收。

## What Changes

以一个总控变更管理五个有依赖、可独立交付的子变更；具体契约仅在子变更维护，本变更负责覆盖范围与整体验收：

| 子变更 | 负责的盘点事项 |
|---|---|
| `production-release-readiness` | 迁移历史、CI/CD、正式版本与服务器、DNS/HTTPS/SSE、真实账号/模型/R2/邮件与向量连接验收 |
| `versioned-content-and-evidence` | 完整正文与续读、来源版本/定位、索引代次/撤权、不可变稿件修订/CAS/恢复/编译快照 |
| `research-organization-artifacts` | 带依据的阅读记录、文献比较、可编辑章节计划、版本化章节摘要 |
| `evidence-aware-writing-review` | 锚定批注/候选修改、实际依赖复查、受控事件触发、规范资源及多维跨章节审阅 |
| `research-writing-quality-closure` | 两场景三阶段验收、提示/检索评价、性能、必要交互整合、文档和存量 PR/规格收口 |

保留 React/Vite、Go/Gin/GORM、Eino DeepAgent、现有 Turn/Run/SSE、云数据库和 R2；前端留 Vercel，小服务器仅运行 API 与 Caddy。不新增独立工作流平台或必需中间件。

**BREAKING（分阶段交付）**：版本化稿件写入将要求基准修订；旧写客户端须升级，旧 API 写入口须停写后才能启用强制 CAS。兼容步骤和失败语义由 `versioned-content-and-evidence` 定义，不允许静默无条件覆盖。

## Capabilities

### New Capabilities
- `research-writing-workflow`: 统一追踪五个子变更、两场景写作闭环、生产证据及“完成/受阻/未验收”的真实完成判定。

### Modified Capabilities
- 无。本总控不重复定义子变更中的领域规格；子变更分别声明其新增/修改的 capability。

## Impact

- 后端：复用现有领域服务与 Agent 运行时，逐步增加来源/稿件修订、组织产物、批注、规范和审阅数据及 API。
- 前端：在现有准备/撰写/检查页面补齐产物编辑、依据定位、版本冲突/恢复、批注与覆盖范围，不另建一套工作台。
- 数据库：SQL truth、PG/MySQL 迁移、Go 模型同步；SQLite 用于开发/测试。新增 DDL 不绕过独立迁移和生产发布审查。
- 运维：先收口当前部署，再随阶段发布；真实凭据、模型费用、邮件收件和生产切换设置明确验收门，不泄露秘密或伪造结果。
- 文档：沿用 `docs/project/12-ai-writing-design.md`、`13-thesis-product-model.md`、`15-event-driven-writing-and-review-model.md` 的已确认原则；本变更覆盖本次盘点事项，不把未承诺的 Graph RAG、SSH 执行、模型训练、Word 双向编辑等并入首版。
