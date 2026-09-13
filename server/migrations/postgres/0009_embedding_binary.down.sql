ALTER TABLE rag_chunks DROP COLUMN IF EXISTS embedding;
ALTER TABLE rag_chunks ADD COLUMN IF NOT EXISTS embedding text;
