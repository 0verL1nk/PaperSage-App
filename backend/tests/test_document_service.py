"""Document service and model tests."""



class TestDocumentModel:
    def test_imports(self):
        from models.document import Document

        assert Document.__tablename__ == "documents"

    def test_schema_imports(self):
        from schemas import DocumentMoveRequest, DocumentRemarkRequest, DocumentResponse

        assert DocumentMoveRequest.model_fields is not None
        assert DocumentRemarkRequest.model_fields is not None
        assert DocumentResponse.model_fields is not None


class TestDocumentResponseSchema:
    def test_has_required_fields(self):
        from schemas import DocumentResponse

        field_names = set(DocumentResponse.model_fields)
        expected = {
            "id",
            "name",
            "type",
            "size",
            "status",
            "s3_key",
            "remark",
            "project_id",
            "folder_id",
            "owner_id",
            "created_at",
            "updated_at",
        }
        assert expected == field_names

    def test_from_attributes(self):
        from schemas import DocumentResponse

        assert "from_attributes" in str(DocumentResponse.model_config)


class TestDocumentMoveSchema:
    def test_valid_move_to_folder(self):
        from schemas import DocumentMoveRequest

        req = DocumentMoveRequest(folder_id="00000000-0000-0000-0000-000000000001")
        assert req.folder_id is not None

    def test_move_to_root(self):
        from schemas import DocumentMoveRequest

        req = DocumentMoveRequest(folder_id=None)
        assert req.folder_id is None


class TestDocumentRemarkSchema:
    def test_valid_remark(self):
        from schemas import DocumentRemarkRequest

        req = DocumentRemarkRequest(remark="Important reference")
        assert req.remark == "Important reference"

    def test_empty_remark(self):
        from schemas import DocumentRemarkRequest

        req = DocumentRemarkRequest(remark="")
        assert req.remark == ""


class TestMimeValidation:
    def test_allowed_pdf(self):
        from services.documents.document_service import ALLOWED_MIME_TYPES

        assert "application/pdf" in ALLOWED_MIME_TYPES

    def test_allowed_docx(self):
        from services.documents.document_service import ALLOWED_MIME_TYPES

        assert (
            "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
            in ALLOWED_MIME_TYPES
        )

    def test_allowed_txt(self):
        from services.documents.document_service import ALLOWED_MIME_TYPES

        assert "text/plain" in ALLOWED_MIME_TYPES

    def test_reject_doc(self):
        from services.documents.document_service import ALLOWED_MIME_TYPES

        assert "application/msword" not in ALLOWED_MIME_TYPES

    def test_reject_exe(self):
        from services.documents.document_service import ALLOWED_MIME_TYPES

        assert "application/x-msdownload" not in ALLOWED_MIME_TYPES

    def test_exactly_three_types(self):
        from services.documents.document_service import ALLOWED_MIME_TYPES

        assert len(ALLOWED_MIME_TYPES) == 3


class TestFileSizeLimit:
    def test_max_size_50mb(self):
        from services.documents.document_service import MAX_FILE_SIZE_BYTES

        assert MAX_FILE_SIZE_BYTES == 50 * 1024 * 1024


class TestDocumentServiceImport:
    def test_service_imports(self):
        from services.documents.document_service import DocumentService

        assert DocumentService is not None

    def test_service_has_methods(self):
        from services.documents.document_service import DocumentService

        svc = DocumentService()
        assert hasattr(svc, "list_by_project")
        assert hasattr(svc, "list_by_folder")
        assert hasattr(svc, "upload")
        assert hasattr(svc, "get_preview")
        assert hasattr(svc, "move")
        assert hasattr(svc, "update_remark")
        assert hasattr(svc, "delete")
