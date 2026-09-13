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

## D7 Hybrid RAG + LLM Wiki（任务书「资料库 RAG」项）
- internal/rag：向量（内存余弦）+ BM25（纯 Go 内存倒排，CJK bigram/
  英文分词）双路召回，RRF 融合。**向量入库**：embedding JSON 列随
  chunk 落 rag_chunks（唯一真相源），进程内索引从 DB 惰性加载——
  重启只 reload 绝不重新嵌入；嵌入调用仅存于写路径（IndexSource
  批量）与读路径（查询文本单次）
- 嵌入：OpenAI 兼容 /embeddings（EMBED_* env），未配置 HashEmbedder
  兜底（保证管线可用，语义质量由真实嵌入决定）
- 原件为真相源：documents confirm 钩子异步（抽取→分块→嵌入→索引），
  失败仅日志不阻断入库
- LLM wiki（agent_wiki_pages）：子 agent 检索结论沉淀页，UpsertWiki
  同步进索引——下次检索直达知识而非重读原文
- 分块：段落优先递归聚合（~1200 字），超长段句号切分

## D8 Agentic search（子 agent 检索）
- 主 agent 只见一个 search_references 工具（Eino AgentAsTool）；
  内部子 agent（检索专家）持 rag_search/wiki_read/wiki_write 三个
  工具做多跳检索：改写查询、读块、沉淀 wiki，输出带出处的综合摘要
- AgentTool 注册名 = 子 agent Name（ADK 约定）
- documents 域经 DocIndexer 回调解耦（不 import rag，装配层闭包）
