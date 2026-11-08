# Proposal: AI Agent 模块（Eino ADK）

## Why
任务书核心研究内容的 2/3（全流程 AI 写作辅助、大模型接入+分场景 prompt
优化）尚未实现；三个工作流 tab（内容分类/逻辑链条/报告生成）为占位页。
本变更为论文撰写页提供 **agentic** AI 助手：模型通过工具自主读取项目
文献、读写 LaTeX、触发编译，而非单轮补全。

## What Changes
- 后端新域 `internal/agent`：基于 CloudWeGo Eino ADK 的 ChatModelAgent
  （ReAct 循环），eino-ext OpenAI 兼容模型组件（DeepSeek/OpenAI 等）
- 模型解析链：用户设置 agent.providers[active] → 服务端 env 兜底
  （AGENT_API_KEY/AGENT_BASE_URL/AGENT_MODEL）
- 项目级工具集：list_documents / read_document / list_latex_files /
  read_latex_file / write_latex_file / compile_project——全部校验归属
- 会话云端化（用户要求生产态不绑本地）：agent_conversations /
  agent_messages 落库（0004 迁移），重启/换端可续
- API：POST chat（SSE 流式：delta/tool_call/tool_result/done）、
  会话列表/消息/删除
- 前端：撰写页 AIPanel 实装（流式聊天 + 工具活动可视 + 未配置引导）

## Capabilities
### Capability: ai-writing-assistant
- 用户在撰写页与 agent 对话；agent 可调用项目工具多轮完成任务
- 流式输出（SSE）；工具调用过程对用户可见
- 会话持久化（DB）；按项目隔离；未配置模型时给出设置引导
