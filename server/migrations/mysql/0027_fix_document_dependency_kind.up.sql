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
-- 自引用子查询包一层派生表，规避 MySQL/TiDB 的 ERROR 1093。

-- 1) 文档有来源版本 → 改写为当前（最大 revision）版本，与新建路径一致。
--    目标依赖已存在时（同一修订同时按版本与文档引用过同一版本）不改写，交给第 3 步清理。
UPDATE research_artifact_dependencies d
JOIN documents doc ON doc.id = d.ref_id
JOIN source_versions v ON v.source_id = doc.source_id
  AND v.revision = (SELECT MAX(v2.revision) FROM source_versions v2 WHERE v2.source_id = doc.source_id)
SET d.ref_id = v.id
WHERE d.kind = 'source_version'
  AND NOT EXISTS (SELECT 1 FROM source_versions sv WHERE sv.id = d.ref_id)
  AND NOT EXISTS (
      SELECT 1 FROM (
          SELECT artifact_id, revision, ref_id
          FROM research_artifact_dependencies
          WHERE kind = 'source_version'
      ) x
      WHERE x.artifact_id = d.artifact_id AND x.revision = d.revision AND x.ref_id = v.id
  );

-- 2) 文档没有来源版本 → 改记为 document 依赖。
UPDATE research_artifact_dependencies d
JOIN documents doc ON doc.id = d.ref_id
LEFT JOIN source_versions v ON v.source_id = doc.source_id
  AND v.revision = (SELECT MAX(v2.revision) FROM source_versions v2 WHERE v2.source_id = doc.source_id)
SET d.kind = 'document'
WHERE d.kind = 'source_version'
  AND v.id IS NULL
  AND NOT EXISTS (SELECT 1 FROM source_versions sv WHERE sv.id = d.ref_id)
  AND NOT EXISTS (
      SELECT 1 FROM (
          SELECT artifact_id, revision, ref_id
          FROM research_artifact_dependencies
          WHERE kind = 'document'
      ) x
      WHERE x.artifact_id = d.artifact_id AND x.revision = d.revision AND x.ref_id = d.ref_id
  );

-- 3) 目标依赖已存在的历史错误行是重复信息，删除。
DELETE d FROM research_artifact_dependencies d
JOIN documents doc ON doc.id = d.ref_id
WHERE d.kind = 'source_version'
  AND NOT EXISTS (SELECT 1 FROM source_versions sv WHERE sv.id = d.ref_id);
