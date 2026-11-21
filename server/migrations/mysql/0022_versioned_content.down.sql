ALTER TABLE latex_files DROP FOREIGN KEY fk_latex_files_source, DROP INDEX uq_latex_files_source, DROP COLUMN source_id;
ALTER TABLE documents DROP FOREIGN KEY fk_documents_source, DROP INDEX uq_documents_source, DROP COLUMN source_id;
DROP TABLE source_segments;
DROP TABLE source_extractions;
DROP TABLE source_versions;
DROP TABLE project_sources;
