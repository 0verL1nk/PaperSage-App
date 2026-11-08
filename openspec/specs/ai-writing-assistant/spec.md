# ai-writing-assistant Specification

## Purpose
TBD - created by archiving change 2026-09-13-ai-agent-module. Update Purpose after archive.
## Requirements
### Requirement: 项目内 AI 对话
系统 **SHALL** 在论文撰写页提供 AI 助手面板，用户可发起对话并收到流式回复。

#### Scenario: 已配置模型时流式对话
- **WHEN** 用户在撰写页发送消息
- **THEN** SSE 依次推送 delta 文本事件，结束推送 done
- **AND** 会话与消息落库，刷新后可继续

#### Scenario: agent 自主调用工具
- **WHEN** 用户请求涉及项目内容（如"看看我的文件列表"）
- **THEN** 模型发起 tool_call，系统执行项目工具并回传 tool_result
- **AND** 工具调用过程作为事件流推送给前端展示

#### Scenario: 未配置模型
- **WHEN** 无 per-user provider 且无服务端 env 兜底
- **THEN** 返回 400 与引导信息，前端展示设置入口

### Requirement: 会话管理
系统 **SHALL** 按项目隔离会话，支持列表、历史消息读取与删除。

#### Scenario: 会话列表与历史
- **WHEN** 用户打开助手面板
- **THEN** 展示当前项目的会话列表，选中后加载历史消息

