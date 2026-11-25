-- 修正历史数据缺陷：documentId 证据曾被记成 kind='source_version' 的依赖，
-- 而 ref_id 实际是 documents.id，导致 adopt 按 source_versions 校验必然 400。
--
-- 只读计数（上线前先执行确认影响行数）：
--   SELECT count(*) FROM research_artifact_dependencies d
--   JOIN documents doc ON doc.id = d.ref_id
--   WHERE d.kind = 'source_version'
--     AND NOT EXISTS (SELECT 1 FROM source_versions v WHERE v.id = d.ref_id);
--
-- 幂等：每条语句都只命中「kind='source_version'、ref_id 不是来源版本、ref_id 是现存文档」的行。

-- 1) 文档有来源版本 → 改写为当前（最大 revision）版本，与新建路径一致。
--    目标依赖已存在时（同一修订同时按版本与文档引用过同一版本）不改写，交给第 3 步清理。
UPDATE research_artifact_dependencies d
SET ref_id = v.id
FROM documents doc
JOIN LATERAL (
    SELECT id FROM source_versions
    WHERE source_id = doc.source_id
    ORDER BY revision DESC
    LIMIT 1
) v ON true
WHERE d.kind = 'source_version'
  AND d.ref_id = doc.id
  AND NOT EXISTS (SELECT 1 FROM source_versions sv WHERE sv.id = d.ref_id)
  AND NOT EXISTS (
      SELECT 1 FROM research_artifact_dependencies x
      WHERE x.artifact_id = d.artifact_id AND x.revision = d.revision
        AND x.kind = 'source_version' AND x.ref_id = v.id
  );

-- 2) 文档没有来源版本 → 改记为 document 依赖。
UPDATE research_artifact_dependencies d
SET kind = 'document'
FROM documents doc
LEFT JOIN LATERAL (
    SELECT id FROM source_versions
    WHERE source_id = doc.source_id
    ORDER BY revision DESC
    LIMIT 1
) v ON true
WHERE d.kind = 'source_version'
  AND d.ref_id = doc.id
  AND v.id IS NULL
  AND NOT EXISTS (SELECT 1 FROM source_versions sv WHERE sv.id = d.ref_id)
  AND NOT EXISTS (
      SELECT 1 FROM research_artifact_dependencies x
      WHERE x.artifact_id = d.artifact_id AND x.revision = d.revision
        AND x.kind = 'document' AND x.ref_id = d.ref_id
  );

-- 3) 目标依赖已存在的历史错误行是重复信息，删除。
DELETE FROM research_artifact_dependencies d
WHERE d.kind = 'source_version'
  AND EXISTS (SELECT 1 FROM documents doc WHERE doc.id = d.ref_id)
  AND NOT EXISTS (SELECT 1 FROM source_versions sv WHERE sv.id = d.ref_id);
