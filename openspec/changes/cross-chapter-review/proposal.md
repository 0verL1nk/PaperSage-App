## Why

检查页原先只有 LaTeX 结构 lint，不评估章节目标与结论是否一致、论断是否有证据、章节之间是否重复或矛盾。它是中期演示与真实写作闭环里唯一没有实现的 P0 语义能力，且不能用硬编码规则冒充。

## What Changes

- 新增 `chapter_review` 科研组织产物：报告作为不可变修订持久化，带 `run_id`、项目隔离与历史版本，不建新表、不迁移。
- 四个语义维度全部由真实模型基于三源上下文推理：已采用学术规范（HELE-25 接口，未接入时显式不可用）、项目背景/章节计划、各章正文与已读资料。**不使用关键词/正则/模板规则生成语义结论。**
- Agent 工具新增 `get_chapter_review_context` 装配三源上下文；`create_research_artifact` / `add_research_artifact_revision` 在落库前逐字核对引文、计算行号、核对资料确实被读过，并用实际读取记录覆盖模型自报的 `sourcesRead`。
- 检查页复用现有 Review 页：基础结构检查（本地确定性规则）与 AI 跨章节审阅分区呈现；覆盖不完整、证据不足、模型未配置或失败时明确说明，不显示「全部通过」。
- 报告历史可回看并与上一份对比（新增 / 已消失 / 仍存在）；异步进度、取消、失败重试复用现有 Run/SSE 链路。
- 计划编辑器补 `filePath` 选择，使「计划 → 正文」有确定性映射。
- 种子数据新增本科毕设固定案例：4 章正文（植入样本量矛盾、跨章重复、论断缺证据）+ 已采用章节计划 + 可读样本说明资料。

## Capabilities

### New Capabilities
- `cross-chapter-review`: 带定位、依据、下一步与诚实覆盖状态的多维跨章节审阅。

### Modified Capabilities
- `writing-workbench`: 检查页从纯结构检查扩展为「基础结构检查 + AI 语义审阅」，保留模型不可用时结构检查仍可用的边界。

## Impact

- 复用 `research_artifacts` / 修订 / 采用 / CAS、Agent Run/SSE/取消/重试、`AiPanel`、`useLatexFiles`、现有 5 项结构检查。
- 后端新增 `server/internal/norms` 输入边界；`server/internal/agent/review_tools.go` 负责上下文装配与引文核对；`server/internal/research` 增加 payload 校验。
- 前端新增 `cross-chapter-review.ts`、`review-prompt.ts` 并扩展 `ReviewPage`；不新建导航、不新增依赖。
- 不做查重、AI 检测、知识图谱、`writing_comments` 全量状态机与规范库本身（HELE-25）。
