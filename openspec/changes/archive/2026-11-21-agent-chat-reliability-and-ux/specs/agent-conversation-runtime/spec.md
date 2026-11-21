## MODIFIED Requirements

### Requirement: Turn 与 Run 生命周期
系统 SHALL 将一次用户提交建模为一个 Turn 与一个 Run；Run SHALL 独立于 HTTP 连接存在，可被查询、取消与重试。同一会话内相同 `clientRequestId` 的重复提交 MUST NOT 产生第二个 Turn 或第二次模型调用；提交 SHALL 携带 `expectedThreadVersion`，版本不匹配时 MUST 拒绝并返回最新版本。会话已存在时，缺失 `expectedThreadVersion` 的提交 MUST 被拒绝（428），MUST NOT 以缺失版本绕过 CAS。服务启动时 SHALL 收敛上一个进程遗留的 `running` Run，写入终态并让订阅端收到终止；多实例调度不是本变更目标。

#### Scenario: 提交创建 Turn 与 Run
- **WHEN** 用户在某会话提交一条消息
- **THEN** 系统创建 Turn 与 Run，返回 `turnId`、`runId`、`userMessageId` 与事件流地址
- **AND** 用户消息与其 parts 已持久化

#### Scenario: 重复提交被幂等吸收
- **WHEN** 用户以相同 `clientRequestId` 再次提交（双击或网络重试）
- **THEN** 系统返回首次创建的 Turn 与 Run，不创建新的 Turn，不重复调用模型

#### Scenario: 会话版本冲突
- **WHEN** 提交携带的 `expectedThreadVersion` 落后于服务端
- **THEN** 返回 409 与最新版本，且不产生 Turn 与 Run
- **AND** 客户端更新版本后可用同一 `clientRequestId` 重试

#### Scenario: 缺失版本
- **WHEN** 会话已存在但提交未携带 `expectedThreadVersion`
- **THEN** 返回 428，不创建 Turn 与 Run

#### Scenario: 取消之后的语义
- **WHEN** 用户取消一个进行中的 Run
- **THEN** Run 状态变为 `cancelled`
- **AND** 系统 MUST NOT 将断流视为执行成功，MUST NOT 将取消视为已撤销已生效的修改

#### Scenario: 服务重启后的遗留 Run
- **WHEN** 服务启动时发现上一个进程遗留的 `running` Run
- **THEN** 系统将其标记为 `failed` 并记录中断原因，持久化已产生的部分消息
- **AND** 该 Run 的订阅端收到终止事件，不永久挂起

### Requirement: 统一事件流
系统 SHALL 以 AI SDK UI Message Stream Protocol 推送执行事件，响应头为 `x-vercel-ai-ui-message-stream: v1`；事件 MUST 使用该协议的 part 词汇（`start`、`text-start|delta|end`、`start-step`、`finish-step`、`tool-input-start|delta|available`、`tool-approval-request|response`、`tool-output-available|denied`、`source-document`、`data-*`、`error`、`finish`、`abort`、`[DONE]`）。工具相关事件 MUST 携带 `toolCallId`；MUST NOT 依赖工具名进行调用与结果的配对。每条 SSE 事件 SHALL 使用标准 `id:` 字段携带 `agent_events.sequence`，供客户端断点续传；该序号 MUST NOT 出现在 part 业务载荷中。

#### Scenario: 流式文本
- **WHEN** 模型产生文本
- **THEN** 依次推送 `text-start`、若干 `text-delta`、`text-end`，三者共享同一 `id`

#### Scenario: 工具调用的完整生命周期
- **WHEN** 模型发起一次工具调用
- **THEN** 依次推送 `tool-input-start`、可选的 `tool-input-delta`、`tool-input-available`
- **AND** 执行完成后推送 `tool-output-available`，被拒绝时推送 `tool-output-denied`
- **AND** 每一事件都携带同一 `toolCallId`

#### Scenario: 流结束
- **WHEN** Run 执行完毕
- **THEN** 推送 `finish` 与 `[DONE]` 终止标记

#### Scenario: 模型未配置
- **WHEN** 无 per-user provider 且无服务端 env 兜底
- **THEN** 创建 Run 的请求返回 400 与引导信息，前端展示设置入口

#### Scenario: 事件携带序号
- **WHEN** 服务端推送任一事件
- **THEN** SSE 的 `id:` 等于该事件的持久化 sequence
- **AND** part 的 JSON 载荷不新增内部序号字段

### Requirement: 工具调用状态机与审批
工具定义 SHALL 包含 `riskLevel`、`requiresApproval` 与展示元数据；MUST NOT 声明没有执行点的 `timeoutMs` / `retryPolicy` 字段。工具调用 SHALL 只记录实际发生的状态迁移：普通工具 `executing → succeeded | failed`；需审批工具 `executing → awaiting_approval → approved | denied`。`denied` 与 `failed` 为终态。没有写入点的 `input_streaming` / `ready` / `validating` MUST NOT 被伪造记录。审批 SHALL 绑定参数哈希，参数与审批时不一致时该审批 MUST 失效。所有暴露给模型的工具 MUST 在工具登记表中声明风险级别；未登记工具 MUST NOT 被默认视为只读，MUST 被拒绝或按写工具处理。

#### Scenario: 只读工具自动执行
- **WHEN** 模型调用一个 `riskLevel = read` 且无需审批的工具
- **THEN** 系统直接执行并记录状态为 `succeeded`

#### Scenario: 需审批工具进入等待
- **WHEN** 模型调用一个 `requiresApproval = true` 的工具
- **THEN** 该调用进入 `awaiting_approval`，并向前端推送 `tool-approval-request`
- **AND** 在用户决策前 MUST NOT 执行

#### Scenario: 审批参数被替换
- **WHEN** 用户批准时提交的 `argumentsHash` 与实际调用参数不一致
- **THEN** 系统拒绝该审批，工具 MUST NOT 执行，必须重新请求审批

#### Scenario: 重复审批
- **WHEN** 用户对一个已决策的工具调用再次提交审批
- **THEN** 系统拒绝，不产生第二次副作用

#### Scenario: 写类工具已登记
- **WHEN** 模型调用会修改研究蓝图、稿件或 wiki 的工具
- **THEN** 该工具在登记表中标记为 `write`，审计记录不得显示为 `read`

### Requirement: 断线恢复
系统 SHALL 支持客户端在订阅中断后恢复：加载消息快照、查询该会话未结束的 Run、重新订阅并接收该 Run 已产生事件的完整重放后继续实时推送；客户端事件归并 MUST 幂等，重放 MUST NOT 造成文本或工具结果重复。事件接口 MUST 接受 `afterSeq`，只重放序号大于该值的事件；客户端 MUST 自动重连并携带最后收到的序号。断线 MUST NOT 被解释为取消，Run SHALL 继续执行。服务端 SHALL 发送心跳；客户端在超过配置的静默窗口后 MUST 显示“正在重连”，而不是继续假装流式输出。

#### Scenario: 订阅中断后重连
- **WHEN** 事件流中断后客户端以 `afterSeq` 重新订阅一个仍在执行的 Run
- **THEN** 系统只重放尚未收到的事件，随后继续实时推送
- **AND** 客户端按 part `id` 归并后，文本与工具结果不出现重复

#### Scenario: 断线不取消
- **WHEN** 客户端连接断开但未调用取消接口
- **THEN** Run 继续执行并持久化终态
- **AND** 客户端重连后可继续接收或从历史恢复

#### Scenario: Run 已结束
- **WHEN** 客户端订阅一个已结束的 Run
- **THEN** 系统推完剩余事件后关闭连接

#### Scenario: 静默超时
- **WHEN** 客户端超过静默窗口未收到任何事件或心跳
- **THEN** 界面显示“正在重连”，并按退避重新订阅
- **AND** MUST NOT 在未确认终态时显示“已完成”

### Requirement: 运行记录
系统 SHALL 为每次 Run 持久化执行事实：输入与输出消息、模型供应方与模型名、提示版本、用量、耗时、错误、状态，以及该 Run 内每次工具调用的序号、参数、结果、状态与耗时。运行记录 SHALL 独立于用户界面消息的展示形态。Run 进入终态后，系统 SHALL 把事件流中实际产生的 text、按 `toolCallId` 区分的 tool、proposal 与 error 忠实写入 assistant 消息的 parts；历史接口 MUST 返回消息状态、Run 状态、会话 `threadVersion` 与未决建议，使刷新后建议卡、工具卡与错误可恢复。系统 MUST NOT 把多次工具调用合并为一条无身份的聚合记录。

#### Scenario: 一次 Run 的事实可查
- **WHEN** 一次 Run 结束
- **THEN** 其模型配置快照、用量与耗时已持久化
- **AND** 该 Run 的每次工具调用可按序号检索，用于统计调用次数与计算量

#### Scenario: 刷新后恢复建议与工具
- **WHEN** 用户刷新页面并重新打开该会话
- **THEN** 历史中的每个工具调用仍保留 `toolCallId`、名称、状态与结果
- **AND** 未决建议仍显示为可应用/拒绝的建议卡，而不是普通文本

#### Scenario: 部分输出与失败
- **WHEN** Run 被取消或失败
- **THEN** 已产生的文本、工具与错误按实际终态持久化
- **AND** 历史接口返回该 Run 的终态，不把中断显示为完成

#### Scenario: 大结果不进入消息与模型上下文
- **WHEN** 工具返回超出阈值的结果
- **THEN** 系统将其存为 artifact，事件与模型上下文只携带摘要与引用

## ADDED Requirements

### Requirement: 取消与重试
取消接口 SHALL 幂等；界面 SHALL 区分 `streaming`、`cancelling` 与 `cancelled`，取消失败 MUST 可见。重试 SHALL 复用原 Turn 创建新 Run，并接受 `clientRequestId` 进行幂等；相同 `clientRequestId` 的重试 MUST NOT 创建多个 Run、重复调用模型或重复计费。重试 MUST 仅适用于 `failed` 或 `cancelled` 的 Run；对 `running` Run 的重试 MUST 被拒绝。

#### Scenario: 取消进行中的 Run
- **WHEN** 用户点击停止
- **THEN** 界面进入 `cancelling` 并禁止再次发送
- **AND** 服务端写入 `cancelled` 终态后界面显示已取消

#### Scenario: 重复取消
- **WHEN** 用户或客户端重复调用取消
- **THEN** 系统返回同一终态，不产生第二次副作用

#### Scenario: 取消失败
- **WHEN** 取消请求失败
- **THEN** 界面显示失败原因并保留当前 Run 状态，不假装已取消

#### Scenario: 幂等重试
- **WHEN** 用户对失败 Run 重试且网络重复提交同一 `clientRequestId`
- **THEN** 系统只创建一个新 Run

#### Scenario: 运行中重试被拒
- **WHEN** 用户对仍在执行的 Run 请求重试
- **THEN** 系统拒绝，不创建并行 Run
