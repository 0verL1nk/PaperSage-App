## ADDED Requirements

### Requirement: Turn 与 Run 生命周期
系统 SHALL 将一次用户提交建模为一个 Turn 与一个 Run；Run SHALL 独立于 HTTP 连接存在，可被查询、取消与重试。同一会话内相同 `clientRequestId` 的重复提交 MUST NOT 产生第二个 Turn 或第二次模型调用；提交 SHALL 携带 `expectedThreadVersion`，版本不匹配时 MUST 拒绝并返回最新版本。

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

#### Scenario: 取消之后的语义
- **WHEN** 用户取消一个进行中的 Run
- **THEN** Run 状态变为 `cancelled`
- **AND** 系统 MUST NOT 将断流视为执行成功，MUST NOT 将取消视为已撤销已生效的修改

### Requirement: 统一事件流
系统 SHALL 以 AI SDK UI Message Stream Protocol 推送执行事件，响应头为 `x-vercel-ai-ui-message-stream: v1`；事件 MUST 使用该协议的 part 词汇（`start`、`text-start|delta|end`、`start-step`、`finish-step`、`tool-input-start|delta|available`、`tool-approval-request|response`、`tool-output-available|denied`、`source-document`、`data-*`、`error`、`finish`、`abort`、`[DONE]`）。工具相关事件 MUST 携带 `toolCallId`；MUST NOT 依赖工具名进行调用与结果的配对。

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

### Requirement: 工具调用状态机与审批
工具定义 SHALL 包含 `riskLevel`、`requiresApproval`、`timeoutMs`、`retryPolicy` 与展示元数据。工具调用 SHALL 经过 `input_streaming → ready → validating → awaiting_approval → approved | denied → executing → succeeded | failed` 状态流转；`denied` 与 `failed` 为终态，无需审批的工具跳过 `awaiting_approval`。审批 SHALL 绑定参数哈希，参数与审批时不一致时该审批 MUST 失效。

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

### Requirement: 断线恢复
系统 SHALL 支持客户端在订阅中断后恢复：加载消息快照、查询该会话未结束的 Run、重新订阅并接收该 Run 已产生事件的完整重放后继续实时推送；客户端事件归并 MUST 幂等，重放 MUST NOT 造成文本或工具结果重复。

#### Scenario: 订阅中断后重连
- **WHEN** 事件流中断后客户端重新订阅一个仍在执行的 Run
- **THEN** 系统重放该 Run 已产生的 parts，随后继续推送实时事件
- **AND** 客户端按 part `id` 归并后，文本与工具结果不出现重复

#### Scenario: Run 已结束
- **WHEN** 客户端订阅一个已结束的 Run
- **THEN** 系统推完剩余 parts 后关闭连接

### Requirement: 运行记录
系统 SHALL 为每次 Run 持久化执行事实：输入与输出消息、模型供应方与模型名、提示版本、用量、耗时、错误、状态，以及该 Run 内每次工具调用的序号、参数、结果、状态与耗时。运行记录 SHALL 独立于用户界面消息的展示形态。

#### Scenario: 一次 Run 的事实可查
- **WHEN** 一次 Run 结束
- **THEN** 其模型配置快照、用量与耗时已持久化
- **AND** 该 Run 的每次工具调用可按序号检索，用于统计调用次数与计算量

#### Scenario: 大结果不进入消息与模型上下文
- **WHEN** 工具返回超出阈值的结果
- **THEN** 系统将其存为 artifact，事件与模型上下文只携带摘要与引用
