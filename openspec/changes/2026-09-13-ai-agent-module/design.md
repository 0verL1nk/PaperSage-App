# Design: AI Agent 模块

## D1 选型：官方 Eino ADK
`adk.NewChatModelAgent`（ReAct 工具循环）+ `adk.NewRunner`（流式事件）+
eino-ext `openai.NewChatModel`（OpenAI 兼容，DeepSeek 走 BaseURL）。
不引入第三方包装（agentkit/goat 等）——官方面已覆盖所需，减少依赖面。

## D2 模型解析链
per-user BYO key（settings.agent.providers，形状 {id,name,base_url,
api_key,models,active}）优先；env 兜底便于演示与服务器统一配置；
都没有 → 400 + 前端引导到设置页。模型实例按请求构建（无全局态）。

## D3 工具设计（agentic 核心）
6 个项目域工具，闭包携带 (db, storage, projectID, userID)：
读侧 4 个（文献列表/文献内容(txt)/LaTeX 列表/LaTeX 内容）+ 写侧 2 个
（写 LaTeX 文件走 latex 域同款路径校验与 S3 存储；触发编译入队）。
写侧工具是「agentic」的分水岭：助手能真正改稿并验证编译。

## D4 会话与状态（云端化）
agent_conversations(project,user,title) + agent_messages(conversation,
role,content,tool_events JSON)。SSE 结束后统一落库（用户消息先写，
助手流结束后写终文+工具事件摘要）。无进程内会话缓存。

## D5 流式协议
POST /api/v1/projects/:id/agent/chat body {conversation_id?, content}
→ SSE `data:` 行，事件 {type: delta|tool_call|tool_result|done|error}。
前端用 fetch+ReadableStream 解析（EventSource 不支持 POST/headers）。

## D6 安全
- 工具内 project 归属校验（复用 verify 模式）
- write_latex_file 路径黑名单（../、危险扩展）复用 latex 域规则
- 模型 API key 不落日志；SSE 失败转 error 事件而非裸 500
