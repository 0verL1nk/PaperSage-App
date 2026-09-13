ALTER TABLE rag_chunks DROP COLUMN embedding;
ALTER TABLE rag_chunks ADD COLUMN embedding varbinary(8192);
