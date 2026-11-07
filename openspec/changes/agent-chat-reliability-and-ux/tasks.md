# Tasks — agent-chat-reliability-and-ux

> 依赖顺序：先补后端转录/版本/流语义，再接前端动线与 UX，最后做真实模型验收。
> 本变更只解决单实例部署下的可靠性与体验；多实例调度、跨实例取消不在此范围。

## 0. 规格与基线

- [x] 0.1 对照 `agent-run-architecture` 的现有 requirement，冻结本变更差异：忠实转录、版本自愈、断线续传、取消/重试、孤儿 Run、阶段上下文；确认不引入并行子 agent 与多实例编排。
- [ ] 0.2 用当前真实 provider 录制一次基线：流式文本、工具调用、取消、刷新恢复、断线；逐项标注通过/失败/未执行，不先修代码。
- [x] 0.3 固定验收矩阵：provider（opencode / OpenAI 兼容）、场景（本科毕设 / 小论文）、阶段（准备 / 写作 / 检查）、网络条件（正常 / 断流 / 超时 / 限流）。

## 1. 转录与历史恢复

- [x] 1.1 在 `server/internal/agent/protocol.go` 固化 part 契约：`text` / `tool` / `proposal` / `activity` / `error` 的 id、status、ordinal、meta；实时事件与历史转录共用同一映射。
- [x] 1.2 在 Run 终态从 `agent_events` + `agent_tool_calls` + `agent_change_proposals` 重建 assistant parts，替换 `createAssistantMessage` 的单 text + 聚合 tool（`runtime.go:281-305`）；补重建纯函数测试。
- [x] 1.3 工具 part 保留 `toolCallId` / `toolName` / 状态 / 耗时 / 错误，同名工具多次调用不合并；建议 part 只存 `proposalId` 与展示快照，历史读取时 join `agent_change_proposals` 取当前状态，应用/拒绝后刷新不显示旧状态。
- [x] 1.4 取消/失败的 Run 持久化已产生的部分文本与工具记录，消息状态标记 `cancelled` / `failed`，不得伪装 `completed`。
- [x] 1.5 新增 `GET /agent/conversations/:id` 返回 `{id,title,threadVersion}`；`GET .../messages` 保持数组并返回消息状态、Run 状态与原始 part 类型；补契约测试：建议卡、同名工具两次调用、取消态。
- [x] 1.6 前端 `messagesFromApi` 保留 part 类型与 meta，不再把未知类型降级为 text（`chat-store.ts:206`）；打开会话时调用已存在的 `listProposals`，与历史 part 去重合并。
- [x] 1.7 回归测试：刷新后建议卡、工具卡、取消态、证据入口与流式时一致，不出现“工具/completed”占位。

## 2. 版本与冲突自愈

- [x] 2.1 会话详情接口返回 `threadVersion`；`open` 成功后 `versionKnown=true`，不再以“未知版本”省略 CAS（`chat-store.ts:35,257`）。
- [x] 2.2 所有发送都携带 `expectedThreadVersion`；后端明确 nil 的语义（缺前置条件 428 / 冲突 409），不得把 nil 当作跳过校验（`runtime.go:129-131`）。
- [x] 2.3 409 响应解析最新 `threadVersion` 并写回 store；提供“基于最新版本重试”动作，不静默重发、不产生第二个 Run。
- [x] 2.4 重试发送复用同一 `clientRequestId`；双击、网络重试、页面重发只产生一个 Turn/Run。
- [ ] 2.5 补前端单测（409 → 更新版本 → 重试成功）与多标签页契约测试：后发标签 409 后自愈，不覆盖先发内容。

## 3. 事件流与断线续传

- [x] 3.1 SSE 响应写标准 `id: <sequence>`；前端记录 `lastEventId`，重连时传 `afterSeq`（`agent-api.ts:146`）。
- [x] 3.2 `streamRun` 接受 `AbortSignal`；切换会话、组件卸载、取消时 abort，旧连接不得继续读取（`agent-api.ts:143`）。
- [x] 3.3 断线后按 `afterSeq` 自动重连（退避），重放按 part id 幂等；增加心跳/停滞检测，超时显示“连接不稳定，正在重连”，断流不得当作成功。
- [x] 3.4 补测试：跨 chunk、断流重连、afterSeq 补发、旧流事件隔离；区分“Run 仍在执行”与“连接断开”的 UI 状态。

## 4. 取消、重试与孤儿 Run

- [x] 4.1 取消状态改为 `cancelling → cancelled`；重复 cancel 幂等；取消请求失败可见并可重试（`chat-store.ts:285-289`）。
- [x] 4.2 前端停止按钮显示“正在停止”，期间禁用发送；取消后保留已生成内容并标记“已停止”，不显示为完成。
- [x] 4.3 接入 `retryRun` 处理失败/取消的 Run；重试携带 `clientRequestId`，后端按会话幂等，连点不重复计费；对 `running` Run 的重试必须拒绝（`runroutes.go:151-217`）。
- [x] 4.4 服务启动时收敛本进程启动前遗留的 `running` Run 为 `failed`（单实例假设写入 design），并补启动恢复测试；订阅已终止 Run 先推终态再关闭。
- [x] 4.5 补齐 `toolSpecs`：`save_writing_preparation` / `wiki_read` / `wiki_write` 的 riskLevel 与审批语义，审计记录真实风险，不再按 read 处理（`toolcalls.go:57-66`）。

## 5. 前端会话动线

- [x] 5.1 “新对话”后保持空白，不因 `activeConversationId` 回退而自动重开第一条会话；用户发送或显式选择后才加载历史（`ai-panel.tsx:67,71-75`）。
- [x] 5.2 `open` 增加请求序号或 AbortController，快速切会话时旧 `listMessages` 响应不得覆盖当前会话（`chat-store.ts:230-234`）。
- [ ] 5.3 （可选增强）会话列表与消息历史改为游标分页（按 `created_at,id`），支持“加载更早”；当前全量返回已满足不截断，本项不阻塞本变更。
- [x] 5.4 会话项显示运行中/未完成状态；打开未完成 Run 时重新订阅；切会话/新建会话时清理旧流、旧错误、旧建议。

## 6. UX 与可访问性

- [x] 6.1 仅当用户接近底部时自动滚动；上翻时停止自动滚动并提供“回到最新”（`ai-panel.tsx:77-79`）。
- [x] 6.2 工具状态中文化（运行中/成功/失败/已拒绝），展示耗时与错误摘要；工具卡保留可折叠详情。
- [x] 6.3 流式文本加 `aria-live="polite"`；停止按钮补 `aria-label`；补焦点管理与键盘可达。
- [x] 6.4 错误分类（鉴权/额度/限流/超时/断线/上下文超限）给出可执行下一步；建议应用失败给 toast 与原因，stale 建议提供“重新生成”（`ai-panel.tsx:488-500`）。

## 7. 阶段上下文与检查页

- [x] 7.1 系统提示注入场景（本科毕设 / 小论文）与当前阶段（准备 / 写作 / 检查）及研究蓝图摘要；细节仍由 `get_writing_state` 读取，不灌全文（`service.go:25-40`）。
- [x] 7.2 `ReviewPage` 增加 `AiPanel`（review 模式）或记录明确不做的理由；若增加，提供“解释批注 / 补材料”快捷入口。
- [ ] 7.3 三阶段 E2E：准备页形成并保存蓝图、写作页产出并应用建议、检查页解释问题与补材料。

## 8. 真实模型验收

- [ ] 8.1 用真实 provider 完成流式文本、工具调用、取消、刷新恢复、断线重连的端到端记录；模拟 SSE 不计入。
- [ ] 8.2 覆盖鉴权失败、超时、限流、上下文超限的真实失败分类与用户提示。
- [x] 8.3 provider HTTP 层增加空闲/响应头超时配置，不把 30 分钟 `runTimeout` 当作唯一兜底（`model.go:158-168`）。
- [ ] 8.4 从生产 Vercel 页面复验一次，确认前端/API 版本匹配并保存证据。

## 9. 文档与收口

- [x] 9.1 更新 agent 域文档与 `CLAUDE.md` 的运行时、恢复、取消语义。
- [x] 9.2 更新 `docs/verification/research-writing/release-readiness.md`，区分已验收、失败与未执行。
- [ ] 9.3 校验 proposal/spec/tasks，完成后归档本变更；同步论文/答辩材料中的真实模型对话证据与局限。
