## Context

`agent-run-architecture` 已建立 Turn/Run、AI SDK UI Message Stream、Part 消息、工具状态机、三类幂等键与确认式改稿。运行时骨架可用，但恢复路径和真实模型验收没有闭环：

- `agent_events` 是事件真相，`agent_message_parts` 是 UI 转录；当前终答只写一条 text + 一条聚合 tool，两者不等价。
- 前端 reducer 已按 part id 幂等，但历史接口返回的 part 丢失类型与身份，刷新后无法还原。
- 线程版本只在新建会话/收到 Run 句柄后已知；重开会话时被清空，CAS 被绕过。
- SSE 有 `afterSeq` 后端能力，但协议未暴露序号，前端也未重连。
- 部署为单实例 API + Caddy；多实例不是本变更目标。

## Goals / Non-Goals

**Goals:**
- 刷新、重开、切会话、断网、取消、服务重启后，用户看到的对话与真实执行一致。
- 线程版本始终可校验；冲突可解释、可重试，不静默覆盖。
- 取消、重试、重连都有明确、可见、幂等的语义。
- 真实模型上的流式文本、工具调用、取消与恢复有可复现证据。

**Non-Goals:**
- 多实例调度与跨实例取消；本变更按单实例部署收敛孤儿 Run。
- 并行子 agent 与专用 Run 树视图。
- WebSocket、消息全文搜索、Markdown 渲染库替换。
- 自动改稿；修改建议仍须用户确认。

## Decisions

### D1 转录在 Run 终态重建，而不是增量写 part

**决策**：Run 进入终态后，服务端从该 Run 的 `agent_events`（按 sequence）、`agent_tool_calls`（按 seq）、`agent_change_proposals` 重建 assistant 消息的 parts，一次事务写入 `agent_message_parts`。

**理由**：事件已经是事实源，实时流也由同一映射函数产生；增量写 part 会在每个 delta 上放大写库，且取消/失败路径容易留下半条 part。终态重建保证 UI 转录与事件日志同源。

**映射**：text 事件按 part id 聚合为一条 `text`；每个 `toolCallId` 一条 `tool` part（保留 input/output/state/耗时）；`data-proposal` 一条 `proposal` part；`error` 一条 `error` part。取消/失败时保留已产生的文本与工具结果，消息状态写 `cancelled` / `failed`。

**建议状态以建议表为准**：`proposal` part 只保存 `proposalId` 与展示快照；历史读取时 join `agent_change_proposals` 取当前状态（pending/applied/rejected/stale），避免应用/拒绝后 part 中的旧状态被重新展示。子 agent 的 silent 事件只写子 Run，不进入主消息转录。

**兼容**：旧消息仍按现有 parts 读取；重建只影响新 Run，不做历史回填。

### D2 历史接口返回可恢复的完整状态

新增 `GET /agent/conversations/:id`，返回 `{id, title, threadVersion}`；现有 `GET /agent/conversations/:id/messages` 保持消息数组形态，但每条消息增加 `status`、`runId` 与原始 `parts`（类型与状态原样）。未决建议由前端单独 `GET /projects/:id/agent/proposals?status=pending` 合并，或在会话详情中一并返回。

消息与建议必须能独立于实时流恢复；不得要求用户重新触发一次 Run 才能看到建议卡。

### D3 线程版本始终参与 CAS

- 打开会话时从历史接口取得 `threadVersion`，`versionKnown=true`。
- 每次提交都携带 `expectedThreadVersion`；不再用 `versionKnown=false` 省略。
- 409 响应体带最新 `threadVersion`；前端更新本地版本，展示“会话已更新”，提供一次显式重试（复用同一 `clientRequestId`）。
- 新建会话的第一次提交允许以 0/最新版本作为基准；创建成功后立即写入返回的版本。

### D4 SSE 用标准 `id:` 暴露序号，支持 `afterSeq`

- 每个 SSE 事件写 `id: <agent_events.sequence>`，part payload 不变。
- 客户端记录 `lastEventId`；重连调用 `GET /runs/:id/events?afterSeq=<last>`。
- 服务端先补 DB 中 `sequence > afterSeq` 的事件，再跟随实时事件；已结束 Run 补完后关闭。
- 断线只停止推送，不取消 Run；UI 明确显示“连接中断，正在重连”，与“Run 已取消/失败”区分。

### D5 取消语义

- `POST /runs/:id/cancel` 幂等：已终态返回当前状态，不重复写终态。
- 前端进入 `cancelling`：停止按钮变为“正在停止”，发送禁用；成功/失败都给出可见反馈。
- 取消只终止执行，不撤销已生效的修改建议；UI 分别展示 Run 状态与稿件/建议状态。
- 单实例下继续使用进程内 cancel map；多实例取消不在本变更范围。

### D6 重试幂等

`POST /runs/:id/retry` 接受 `clientRequestId`，在会话内唯一；重复请求返回首次创建的新 Run，不重复调用模型。前端只在 `failed` / `cancelled` 的 Run 上展示重试入口，重试期间禁用重复点击。

### D7 启动时收敛孤儿 Run

服务启动时把 `started_at` 早于本进程启动时间的 `running` Run 标为 `failed`，错误写“服务重启，执行已中断”，并补齐终态事件，使订阅端收到终止。该行为以单实例部署为前提；若未来多实例，需改为租约/心跳后再启用。

### D8 前端状态机

```
idle → sending → streaming → (completed | failed | cancelled | reconnecting)
cancelling 是 streaming 的子状态，仅影响按钮与输入
```

- `open` 使用请求序号/AbortController；过期响应丢弃。
- 新对话设置 `newConversation=true`，禁止自动重开历史；用户主动选择会话才清除。
- 自动滚动仅在用户接近底部时生效；上翻后显示“回到最新”。
- 发送失败保留输入内容与 `clientRequestId`，提供“重试发送”；不得生成新 Run。

### D9 阶段上下文与检查页入口

- 系统提示注入：场景（本科毕设/入门论文）、阶段（准备/撰写/检查）、研究蓝图摘要、当前项目名；完整状态仍由 `get_writing_state` 按需读取。
- `AiPanel` 增加 `mode="review"`，在 `ReviewPage` 提供入口；审阅模式下快速提示面向“解释检查结果、定位问题、补材料”，不自动改稿。这是补齐“三阶段都有助手”的产品闭环，复用现有面板，不新建页面。
- 不把整份稿件塞进系统提示；按任务用工具读取。

### D10 工具审计与错误分类

- `toolSpecs` 补齐 `save_writing_preparation`、`wiki_read`、`wiki_write`；写类工具标 `RiskWrite`，是否 `RequiresApproval` 按现有产品语义明确声明。
- 未登记工具按 `RiskWrite` 处理，而不是默认 read。
- provider 错误映射为：未配置、鉴权失败、限流、超时、上下文超限、网络中断、服务端 5xx；前端显示可操作提示（去设置/稍后重试/缩短上下文/检查网络）。

### D11 分页（增强，不阻塞）

会话列表与消息当前全量返回且没有截断，不构成本变更的缺陷。游标分页（`created_at,id` / `ordinal`）列为后续增强：客户端传 `limit`，服务端只做资源上限保护，不设产品级固定条数。本变更只要求不恢复按条数静默截断。

## Gap → Decision 对照

| 现象 | 根因 | 决策 |
|---|---|---|
| 刷新后建议卡消失 | 终答只写 text+聚合 tool | D1、D2 |
| 工具卡丢失 toolCallId | 历史 part 无身份 | D1、D2 |
| 409 后持续冲突 | 不更新版本 | D3 |
| 重开会话绕过 CAS | versionKnown=false | D3 |
| 新对话被重开 | activeConversationId 回退 | D8 |
| 切会话串消息 | open 无请求序号 | D8 |
| 断线无续传 | 未用 afterSeq | D4 |
| 取消无反馈 | 无 cancelling 状态 | D5 |
| 重试重复计费 | retry 无幂等键 | D6 |
| 重启后 Run 悬挂 | 无孤儿收敛 | D7 |
| 写工具被记 read | toolSpecs 缺失 | D10 |
| 阶段提示单一 | 未注入上下文 | D9 |

## Risks

- 转录重建必须与实时映射保持同一套 part 语义，否则历史与实时再次分叉；用共享 reducer + 契约测试约束。
- 子 agent 的 silent 事件只写子 Run，重建主转录时必须排除，否则子 agent 原文会泄漏到主回答。
- 心跳事件不得写 `id:`，否则会推进客户端 `afterSeq`，造成断点续传跳过真实事件。
- 重建只覆盖新 Run，旧会话仍可能缺少建议卡；在 UI 中如实显示，不回填伪造历史。
- 启动收敛会终止真实在跑的 Run；仅在单实例部署启用，并在部署文档中写明。
- 游标分页会改变现有“全量返回”契约，前端需同时升级；旧客户端不应被静默截断。

## Rollout

1. 后端先落转录重建与历史接口字段（向后兼容新增字段）。
2. 前端接版本、重连、取消、重试与新对话修正。
3. 再补阶段上下文、检查页入口与可访问性。
4. 最后用真实 provider 跑验收矩阵，更新 `docs/verification/research-writing/` 记录。
