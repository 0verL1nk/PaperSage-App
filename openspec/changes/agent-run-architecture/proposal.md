## Why

`2026-09-13-ai-agent-module` 交付了可用的 agent 后端（Eino ADK 的 ReAct 循环 + 6 个项目工具 + `search_references` 子 agent + RAG/wiki），但其数据与协议形态停留在 Demo 级：

- **协议是自造的**：SSE 只推 `meta|delta|tool_call|tool_result|done|error`，`tool_call` 事件不含调用 ID，前端只能按「同名工具队列」配对（同一轮两次 `read_latex_file` 会串味，见 handoff 备忘）。
- **没有 Run 实体**：一次执行的中间态（模型步骤、工具调用、用量、耗时、错误）全部压缩进一条 assistant 消息的 `tool_events` JSON 列。JSON 无法索引、无法聚合，而 `docs/project/12-ai-writing-design.md` §9.2 明确要求统计工具调用次数与计算量，§10 要求把「实际模型、提示版本、检索配置及降级情况」写入运行记录。
- **无法断线恢复**：刷新页面时若 run 仍在执行，既查不到状态也接不回事件流（§8 要求「断流不等于成功，取消也不等于已撤销」，现有实现无从表达）。
- **无幂等**：用户双击发送或网络重试会产生两条用户消息、两次模型调用。
- **写侧直接覆盖稿件**：`write_latex_file` 无确认、无版本校验，与 `docs/project/12-ai-writing-design.md` §10 P0「切断未经确认的稿件覆盖……避免将现有直接写工具直接暴露为可用功能」直接冲突。

参考架构确定采用《AI 对话应用前后端交互架构研究》的阶段 1–4，并完全对齐 Vercel AI SDK UI Message Stream Protocol 的 part 词汇。

## What Changes

**传输与协议**
- 引入 **Run 实体**：一次用户提交 = 一个 Turn，Turn 创建一个 Run；Run 独立于 HTTP 连接存在，可查询、可取消、可恢复。
- 事件流改为 **AI SDK UI Message Stream Protocol**（响应头 `x-vercel-ai-ui-message-stream: v1`），part 词汇完全对齐：`start` / `text-start|delta|end` / `tool-input-start|delta|available` / `tool-approval-request|response` / `tool-output-available|denied` / `source-url|source-document` / `data-*` / `start-step|finish-step` / `finish` / `abort` / `[DONE]`。
- **BREAKING**：移除 `POST /projects/:project_id/agent/chat` 的直返流模式（模式 A），改为模式 B——先建 Run，再订阅事件。

**消息模型**
- 消息由 **Part** 组成：`agent_messages` 去掉 `content` / `tool_events`，新增 `agent_message_parts`（`text` / `tool` / `activity` / `approval` / `citation` / `artifact` / `error`）。
- 前端不再拼接字符串，改为 Normalized Store + Event Reducer（按 part id 幂等归并）。

**工具与确认**
- 工具定义扩展：`riskLevel` / `requiresApproval` / `timeoutMs` / `retryPolicy` / `display`。
- 主 agent **摘除 `write_latex_file`**，改为 `propose_edit(path, old_text, new_text, reason)`：只产出带锚点的修改建议，不写盘。
- 建议挂在工具调用的**审批状态机**上（`input_streaming → ready → validating → awaiting_approval → approved/denied → executing → succeeded/failed`），审批绑定 `argumentsHash`——参数变化则旧审批失效。
- 用户在面板上确认后，由服务端校验权限、基准版本与幂等键，再执行替换并写入 LaTeX 文件（复用 latex 域的路径校验与存储）。

**多 Agent 架构**
- 确立 **Supervisor + 专业子 agent** 形态：子 agent 以工具挂载，用户始终面对单一 Assistant，子 agent 不成为顶层发言者。
- **Run 树**：子 agent 调用创建 `parent_run_id` 子 Run，拥有独立 `sequence`、events 与工具调用记录。
- **结果压缩契约**：子 agent 只向父 agent 返回 `{conclusion, evidence[], uncertainties[]}`，完整过程留在子 Run 供审计；父 Run 上下文不装载子 agent 完整历史。
- **展示语义**：子 agent 活动作为 `activity` part 嵌入 Assistant 消息，可展开为 Run 树；并行时呈现为任务面板而非多个发言者。
- v1 以现有 `search_references` 为唯一落地子 agent；`compare_literature` / `review_chapters` 按产品依赖后续增补——**增补子 agent 不改表、不改协议、不改前端**。

**可靠性与幂等**
- 三类幂等键：`clientRequestId`（防重复 Turn）、`tool idempotency_key`（防工具副作用重复）、`approvalId + argumentsHash`（防审批错配）。
- Thread 乐观锁：提交携带 `expectedThreadVersion`，不匹配返回 409 与最新版本。
- 事件持久化到 `agent_events`（`UNIQUE(run_id, sequence)`）作为内部事实记录与审计来源。

**数据**
- 新增：`agent_turns`、`agent_message_parts`、`agent_runs`、`agent_tool_calls`、`agent_events`、`agent_artifacts`、`agent_change_proposals`、`agent_evidence`、`agent_wiki_sources`。
- 修改：`agent_conversations`（+`version`）、`agent_messages`（改由 parts 承载内容）、`agent_wiki_pages`（+`source_run_id` / `stale_at`）。

**前端**
- `services/agent-api.ts`（SSE 用 fetch + ReadableStream 解析）+ `api-routes.ts` + 单测。
- AIPanel 实装：会话列表、消息流、工具卡片（状态机驱动）、审批/建议卡片（diff + 应用/拒绝 + 过期态）、未配置模型引导。
- 模型选择器回写 `settings.agent.active_provider_id`（当前选中值到不了后端，属既有缺陷）。
- 轻量联动 + 编译联动：建议应用后刷新文件树与编辑器；识别 `compile_project` 结果触发编译状态轮询。

## Capabilities

### New Capabilities
- `agent-conversation-runtime`: Turn / Run 生命周期、统一事件流、工具调用状态机、审批与幂等、断线恢复。
- `agent-edit-proposal`: 由 agent 产出的带锚点修改建议，及其确认、应用与失效语义。

### Modified Capabilities
<!-- 无 MODIFIED delta：`ai-writing-assistant` 只存在于未归档的 `2026-09-13-ai-agent-module`，
     尚未进入 openspec/specs/ 基线，交错引用会校验失败。其「项目内 AI 对话」与「会话管理」
     两条需求由本变更的 `agent-conversation-runtime` 取代（对话入口改为创建 Run + 订阅事件；
     消息改由 Part 组成），并在该变更归档时一并处理。 -->

## Impact

- 后端：`server/internal/agent`（runtime / 事件协议 / 工具 / 审批）、`server/internal/store`（9 张新表模型与 `AllModels()`）、`server/schema/tables/*.sql`、双方言迁移、`bootstrap` 装配、契约测试。
- 前端：`services/agent-api.ts`、`api-routes.ts`、`views/projects/WritingPage.tsx` 的 `AIPanel` 与新增 chat 组件、`store`（chat store + reducer）、i18n、组件测试与 e2e。
- 数据：9 张新表 + 3 张表改造；现有 `agent_messages.content` / `tool_events` 需迁移为 Part（迁移脚本一次性完成，无历史包袱：现有会话均为演示数据）。
- 协议：**BREAKING**——旧 SSE 事件格式与 `POST .../agent/chat` 废弃。前端面板尚未实现，此刻切换代价最低。
- 文档：`CLAUDE.md` agent 域小节、`docs/project/12-ai-writing-design.md` §7/§8 状态同步。
- 依赖：**不新增前端依赖**（不引入 `ai` / AI Elements）。协议与组件形态参考，代码自建。
