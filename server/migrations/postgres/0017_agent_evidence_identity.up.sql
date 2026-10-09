CREATE UNIQUE INDEX uq_evidence_run_chunk_version
  ON agent_evidence (run_id, chunk_id, source_version);
