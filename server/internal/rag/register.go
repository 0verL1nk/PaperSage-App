package rag

import "github.com/0verL1nk/GraduateProject/server/internal/store"

// 注册 RAG 域模型参与 store 的 AutoMigrate。
//
// store 不能反向 import rag（rag 依赖 store，会成环），故用注册口。
// 缺这次注册的后果：SQLite 开发/E2E 路径下 rag_chunks 与 agent_wiki_pages
// 根本不会被创建，而索引失败只记日志不阻断入库——RAG 静默失效，无人察觉。
func init() {
	store.RegisterModels(&IndexedChunk{}, &WikiPage{}, &RagIndexGeneration{})
}
