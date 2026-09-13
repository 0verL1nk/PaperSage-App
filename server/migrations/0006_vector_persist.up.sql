-- 0006：向量入库（embedding 列 = rag_chunks 唯一真相源的一部分）
-- 进程内向量索引从本列惰性加载；重启/换实例只 reload 不重新嵌入。
ALTER TABLE rag_chunks ADD COLUMN IF NOT EXISTS embedding text;
