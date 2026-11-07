## Why

AI 助手运行时（Turn/Run、SSE、工具、修改建议）已按 `agent-run-architecture` 落地，但“真实模型流式对话、工具调用、取消与历史恢复”还没有形成可验收闭环。本轮端到端审计确认的问题不是“缺少一个聊天框”，而是恢复与状态一致性存在真实缺陷：

- **历史恢复退化**：助手终答只持久化一条 text 和一条聚合 tool（`server/internal/agent/runtime.go:281-305`），前端把非 text/tool/activity/error 的 part 一律映射成 text（`frontend/src/store/chat-store.ts:206`）。刷新后建议卡消失、工具卡丢失 `toolCallId` 与状态，与“刷新后可继续”的规格不符。
- **版本冲突不自愈**：409 只显示错误，不更新本地 `conversationVersion`（`chat-store.ts:266-278`）；重开会话时 `versionKnown=false`，提交省略 `expectedThreadVersion`（`chat-store.ts:35,257`），后端 nil 即跳过 CAS（`server/internal/agent/runtime.go:129-131`），多标签页可并发覆盖。
- **新对话与切会话竞态**：“新对话”重置后 `activeConversationId` 回退到 `conversations[0]`，effect 立刻重开第一条历史会话（`ai-panel.tsx:67,71-75`）；`open` 无请求序号，快速切换时旧 `listMessages` 可覆盖新会话（`chat-store.ts:230-234`）。
- **断线、取消、重试不可用**：`streamRun` 已接受 `signal`，但 store 没有创建/传入 `AbortSignal`，切会话后旧流仍继续读取；`afterSeq` 与 `retryRun` 已定义但未使用（`agent-api.ts:121-124,146`）；取消没有“正在停止”，失败静默；发送失败后输入框仍可再次发送，容易产生第二个 Run。
- **重启后 Run 悬挂**：服务启动不收敛上一个进程遗留的 `running` Run，订阅端会一直等待。
- **审计与上下文缺口**：`save_writing_preparation`、`wiki_read`、`wiki_write` 未登记 `toolSpecs`，被记成 read；系统提示未注入场景/阶段；检查页没有助手入口。

这些问题会让答辩演示在“刷新、切会话、断网、取消、重启”任一动作上露馅，因此需要单独收口，而不是继续堆功能。

## What Changes

- **忠实转录与历史恢复**：Run 终态时从 `agent_events`、`agent_tool_calls`、`agent_change_proposals` 重建 assistant 消息的 parts（text / tool（按 `toolCallId`）/ proposal / error），保留状态、顺序与身份；历史接口返回消息状态、Run 状态、`threadVersion` 与未决建议。
- **版本与冲突自愈**：会话打开即获得 `threadVersion`；每次提交都携带 `expectedThreadVersion`；409 更新本地版本并提供一次显式重试，不以缺失版本绕过 CAS。
- **事件流与断线续传**：SSE 使用标准 `id:` 携带事件序号；客户端用 `afterSeq` 断点续传、指数退避重连、按 part id 幂等归并；断开连接不等于取消 Run。
- **取消、重试与孤儿 Run**：取消有 `cancelling` 可见状态且幂等；重试接受 `clientRequestId` 并幂等；服务启动时收敛上一进程遗留的 `running` Run（单实例部署假设）。
- **会话动线**：新对话不再被自动重开；切会话请求有序；会话与消息改为游标分页，不固定截断。
- **阶段上下文与入口**：系统提示注入场景、阶段与研究蓝图摘要；检查页提供 review 模式助手入口。
- **工具审计**：所有写类工具声明风险级别；未登记工具不得默认成读。
- **真实模型验收**：在真实 provider 上验证流式文本、工具调用、取消、重连与历史恢复，并区分鉴权、限流、超时、上下文超限与网络中断。

## Capabilities

### Modified Capabilities

- **agent-conversation-runtime**：线程版本、事件流序号与断点续传、断线恢复、Run 终态收敛、转录持久化与历史接口。
- **ai-writing-assistant**：真实模型对话与工具调用的产品动线、取消/重试/重连、会话管理、阶段上下文与检查页入口、可访问性。

## Impact

- 后端：`server/internal/agent/`（runtime、runexec、service、routes、toolcalls）、`server/internal/store`（如新增取消/序号字段）、迁移与模型同步；不改变“助手不直接改稿”的边界。
- 前端：`services/agent-api.ts`、`store/chat-store.ts`、`views/projects/ai-panel.tsx`、`PreparePage.tsx`、`WritingPage.tsx`、`ReviewPage.tsx`；不新增必需依赖。
- 数据库：优先复用 `agent_events.sequence`、`agent_tool_calls`、`agent_change_proposals`；仅当取消需要跨进程可见时才新增字段。
- 验收：真实 provider 的流式/工具/取消/重连/历史恢复证据；模拟 SSE 不能替代。
- 非目标：多实例调度、并行子 agent、WebSocket、模型训练、完整消息全文搜索。
