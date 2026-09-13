-- 0009：rag_chunks.embedding 文本 JSON → binary float32
-- 定长 dim×4 字节小端序列化，体积 ~1/3 且解析零分配；
-- 向量数按嵌入端点而定（bge-m3=1024 维 → 4096 字节）。
ALTER TABLE rag_chunks DROP COLUMN IF EXISTS embedding;
ALTER TABLE rag_chunks ADD COLUMN IF NOT EXISTS embedding varbinary(8192);
