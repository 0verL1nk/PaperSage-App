ALTER TABLE agent_evidence
  ADD COLUMN source_name varchar(255) NOT NULL DEFAULT '';

UPDATE agent_evidence AS evidence
SET source_name = chunks.source_name
FROM (
  SELECT DISTINCT ON (source_type, source_id) source_type, source_id, source_name
  FROM rag_chunks
  ORDER BY source_type, source_id, source_name
) AS chunks
WHERE evidence.source_type = chunks.source_type
  AND evidence.source_id = chunks.source_id;
