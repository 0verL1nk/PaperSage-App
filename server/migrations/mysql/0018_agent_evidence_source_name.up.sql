ALTER TABLE `agent_evidence`
  ADD COLUMN `source_name` varchar(255) NOT NULL DEFAULT '' AFTER `source_id`;

UPDATE `agent_evidence` AS evidence
JOIN (
  SELECT `source_type`, `source_id`, MAX(`source_name`) AS `source_name`
  FROM `rag_chunks`
  GROUP BY `source_type`, `source_id`
) AS chunks
  ON evidence.`source_type` = chunks.`source_type`
  AND evidence.`source_id` = chunks.`source_id`
SET evidence.`source_name` = chunks.`source_name`;
