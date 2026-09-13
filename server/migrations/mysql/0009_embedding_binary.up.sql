-- 0009：rag_chunks.embedding 文本 JSON → binary float32（MySQL/TiDB）
ALTER TABLE rag_chunks DROP COLUMN embedding;
ALTER TABLE rag_chunks ADD COLUMN embedding varbinary(8192);
