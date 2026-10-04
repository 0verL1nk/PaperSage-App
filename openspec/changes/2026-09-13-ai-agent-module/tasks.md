# Tasks

- [x] 1. 迁移 0004：agent_conversations / agent_messages + 模型 + 测试
- [x] 2. go get eino + eino-ext/openai
- [x] 3. model.go：解析链（user settings → env）+ 单测
- [x] 4. tools.go：6 工具（InferTool）+ 归属校验 + 单测（读侧断言+写侧落库）
- [x] 5. service.go + routes.go：SSE chat、会话 CRUD；httptest 假
     OpenAI 服务验证完整 ReAct 循环（tool_call → result → 终答）+ 落库
- [x] 6. main/testutil 装配
- [ ] 7. 前端 agent-api（SSE 解析）+ AIPanel 实装（流式/工具活动/引导）
- [ ] 8. e2e：未配置引导 + mock 模型下对话冒烟
- [ ] 9. .env 模板补 AGENT_*；文档更新

- [x] 10. 迁移 0005（rag_chunks/agent_wiki_pages）
- [x] 11. internal/rag：embedder（HTTP+Hash）、chunker、hybrid store
     （chromem+BM25+RRF）、wiki CRUD —— 5 组单测
- [x] 12. documents confirm 钩子（DocIndexer 回调解耦）+ main 装配
- [x] 13. search_references 子 agent（rag_search/wiki_read/wiki_write）
     + 端到端测试（5 轮脚本化假模型：主→子→rag→wiki→终答）
