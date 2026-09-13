-- 0010: 向量下推 TiDB (VECTOR 类型 + vec_cosine_distance)
-- 应用进程不再持有向量索引; 检索走库端 (Serverless 为库内精确扫描,
-- Dedicated/Enterprise 有 columnar replica 时可建 HNSW:
--   CREATE VECTOR INDEX vidx ON rag_chunks ((VEC_COSINE_DISTANCE(embedding)))
-- 无维度声明以兼容不同嵌入模型 (bge-m3=1024 等)。
-- 注: TiDB 不支持 varbinary->vector 的 MODIFY, 走 drop+add
-- (存量向量会丢失, 重新入库即可恢复)。
ALTER TABLE rag_chunks DROP COLUMN embedding;
ALTER TABLE rag_chunks ADD COLUMN embedding VECTOR;
