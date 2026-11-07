# ai-writing-assistant Specification

## MODIFIED Requirements

### Requirement: 项目内 AI 对话
系统 SHALL 在写作准备、内容撰写、检查完善三个阶段提供 AI 助手入口，用户可发起对话并收到真实模型的流式回复。回复 SHALL 支持文本、工具调用、修改建议与错误；断线、取消、重试与失败 MUST 有可见状态。系统 SHALL 在系统提示中注入当前场景（本科毕业设计/入门级学术小论文）、阶段（准备/撰写/检查）与研究蓝图摘要；详细状态仍由工具读取。检查阶段的助手 MUST 只提供解释、追问与审阅建议，MUST NOT 自动修改正文。真实 provider 的鉴权失败、限流、超时、上下文超限与网络中断 MUST 区分展示，MUST NOT 静默重试计费调用。

#### Scenario: 已配置模型时流式对话
- **WHEN** 用户在任一阶段发送消息
- **THEN** SSE 依次推送文本、工具与建议事件，结束推送终止标记
- **AND** 会话与消息落库，刷新后可继续

#### Scenario: agent 自主调用工具
- **WHEN** 用户请求涉及项目内容（如"看看我的文件列表"）
- **THEN** 模型发起 tool_call，系统执行项目工具并回传 tool_result
- **AND** 工具调用过程作为事件流推送给前端展示，并按 `toolCallId` 区分

#### Scenario: 未配置模型
- **WHEN** 无 per-user provider 且无服务端 env 兜底
- **THEN** 返回 400 与引导信息，前端展示设置入口

#### Scenario: 阶段上下文
- **WHEN** 用户在准备页、写作页或检查页打开助手
- **THEN** 系统提示包含当前场景、阶段与研究蓝图摘要
- **AND** 用户无需重复说明当前处于哪个阶段

#### Scenario: 检查页助手
- **WHEN** 用户在检查完善页打开助手
- **THEN** 助手可解释检查结果、追问依据并给出审阅建议
- **AND** 助手不会直接改写稿件

#### Scenario: 错误分类
- **WHEN** 模型调用失败
- **THEN** 界面区分鉴权失败、限流、超时、上下文超限与网络中断
- **AND** 给出可执行的下一步，而不是统一的“调用失败”

### Requirement: 会话管理
系统 SHALL 按项目隔离会话，支持列表、历史消息读取与删除。会话详情接口 SHALL 返回会话 `threadVersion`；消息接口 SHALL 返回消息状态、Run 状态与原始 part 类型。用户点击“新对话”后 MUST 进入空白新会话，MUST NOT 被自动重开为第一条历史会话。快速切换会话时，旧请求的结果 MUST NOT 覆盖当前会话。历史加载 MUST NOT 使用固定条数静默截断；游标分页为后续增强，不阻塞本变更。

#### Scenario: 会话列表与历史
- **WHEN** 用户打开助手面板
- **THEN** 展示当前项目的会话列表，选中后加载历史消息
- **AND** 历史中包含工具卡、建议卡与 Run 终态

#### Scenario: 新对话
- **WHEN** 用户点击“新对话”
- **THEN** 面板保持空白并等待第一条消息
- **AND** 不自动重新打开任何历史会话

#### Scenario: 快速切换会话
- **WHEN** 用户连续切换两个会话
- **THEN** 只有当前选中会话的历史被展示
- **AND** 较早请求的迟到响应不覆盖当前会话

#### Scenario: 历史分页
- **WHEN** 会话消息规模需要分页
- **THEN** 客户端可用游标继续加载更早消息
- **AND** 系统不因固定条数丢弃最近或中间消息

#### Scenario: 未完成 Run
- **WHEN** 用户重新打开存在 `running` 或 `cancelling` Run 的会话
- **THEN** 界面显示该 Run 的状态并提供继续接收
- **AND** 若 Run 已被服务重启收敛为失败，界面显示中断原因与重试入口

## ADDED Requirements

### Requirement: 流式体验与可访问性
助手界面 SHALL 区分 `idle`、`sending`、`streaming`、`cancelling`、`reconnecting` 与 `error` 状态。流式文本 SHALL 通过 `aria-live` 向辅助技术播报；停止、发送与重试控件 MUST 有可读名称。消息列表 SHALL 仅在用户接近底部时自动滚动，并提供“回到最新”入口。工具状态 MUST 使用用户可读文本，而不是内部英文枚举。修改建议应用失败或转为过期时 MUST 可见说明。

#### Scenario: 用户上翻阅读
- **WHEN** 用户向上滚动查看较早消息
- **THEN** 新事件不会强制把视图拉回底部
- **AND** 界面提供“回到最新”入口

#### Scenario: 辅助技术
- **WHEN** 助手流式输出或 Run 状态变化
- **THEN** 屏幕阅读器可获知状态与新增文本
- **AND** 停止按钮有可读名称而非仅图标

#### Scenario: 建议应用失败
- **WHEN** 用户应用修改建议时基准已变化或服务端失败
- **THEN** 界面显示失败原因与建议状态
- **AND** 不把失败显示为已应用
