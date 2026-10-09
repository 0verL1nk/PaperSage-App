## Why

任务书四个核心业务模块中「分场景写作工作台」尚无落地形态：当前项目没有论文场景概念（本科毕业论文 / 入门级学术小论文），三阶段（写作准备 → 内容撰写 → 检查完善）没有持久化状态与导航载体；项目内现有三个占位 tab（内容分类 / 逻辑链条 / 报告生成）与当前产品主线不一致。

后续所有 AI 能力（研究任务澄清、文献阅读与比较、章节论证计划、规范条目、建议与确认流）都需要挂在「持续保存的项目写作状态」上。并且三阶段不是线性关卡：撰写阶段会回来完善研究目标与资料库，检查阶段会回退重写——底座必须在数据与交互上支持跨阶段交叉流转，切换阶段不清空、不设门槛、不锁定。

## What Changes

- 项目新增场景字段 `scenario`（`undergraduate_thesis` | `intro_paper`）：创建项目时选择；列表与工作台展示；存量项目默认本科毕设。
- 新增 `ProjectWritingState`（每项目一条，服务端权威存储）：
  - 研究蓝图 `brief`：研究主题、背景、核心研究问题、目标与预期成果、不超出范围、研究对象与数据/实验条件、方法与技术路线、关键章节、时间节点与学校/导师约束、已知事实/假设/未证实事项；
  - 课题要求 `requirements`：条目化（内容、来源、适用说明）；
  - 三阶段状态 `phases`（`not_started` | `in_progress` | `done`）与当前阶段 `current_phase`；完成度由用户维护，**不得**由聊天轮数或页面访问推断；
  - 修订号 `revision` 做乐观锁。
- 新增 API：`GET|PUT /api/v1/projects/:project_id/writing-state`；PUT 携带 `revision`，冲突返回 409 并附服务端最新状态。
- 前端信息架构改为阶段优先：项目内导航 = 写作准备 / 内容撰写 / 检查完善（+ 资料库）；原三个占位 tab 移除，旧路径重定向到新阶段路由。
- 新增「写作准备」工作台：研究蓝图与课题要求编辑、材料概览（复用现有文档模块）、基于缺失字段与材料状态的待补项提示；保存失败可见可重试，版本冲突不丢用户输入。
- 「检查完善」本变更先交付页面骨架与阶段状态维护；跨章节检查等能力由后续变更实现（页面如实说明边界，不伪造功能）。
- **BREAKING（仅前端路由层）**：移除 `/project/:projectId/classifying`、`logic-chain`、`generating` 三个占位路由，重定向到新的阶段路由。

## Capabilities

### New Capabilities
- `writing-workbench`: 项目场景、三阶段导航、项目写作状态的持久化与编辑、跨阶段交叉流转与待补项提示。

### Modified Capabilities
<!-- 无：现有 spec 未约束被移除的占位路由与工作台导航结构 -->

## Impact

- 后端：`server/internal/projects`（scenario 字段与 DTO）、新域 `server/internal/writingstate`、`server/schema/tables/`（projects.sql 修改 + project_writing_states.sql 新增）、`server/migrations/{postgres,mysql}/0011_*`、`store` 模型与 `AllModels()`（SQLite AutoMigrate）、`bootstrap` 装配、契约测试。
- 前端：`services/api-routes.ts` + 新 `services/writing-state-api.ts`、`types/projects.ts`、`app/router.tsx`、`views/layout/WorkspaceLayout.tsx`、新 PreparePage / ReviewPage、CreateProjectDialog、i18n、组件与 e2e 测试。
- 数据：projects 表加列 + 新表 `project_writing_states`；无破坏性数据变更（存量项目回退默认场景）。
- 文档：`CLAUDE.md` 架构小节、`docs/PROJECT.md` 状态段落随实现同步；后续变更（阅读与比较、章节计划与摘要、规范库、建议确认、场景切换）在实施时各自立项。
