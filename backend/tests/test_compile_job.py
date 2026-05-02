"""Tests for compile job model, schemas, and compile queue service."""

from __future__ import annotations

import uuid

from models.compile_job import CompileJob, CompileJobStatus
from schemas.compile_job import (
    CompileJobHistoryResponse,
    CompileJobListItem,
    CompileJobStatusResponse,
    CompileJobSubmitResponse,
)


class TestCompileJobStatus:
    """CompileJobStatus enum tests."""

    def test_all_statuses(self) -> None:
        assert CompileJobStatus.PENDING == "pending"
        assert CompileJobStatus.RUNNING == "running"
        assert CompileJobStatus.SUCCESS == "success"
        assert CompileJobStatus.FAILED == "failed"
        assert CompileJobStatus.TIMEOUT == "timeout"
        assert CompileJobStatus.CANCELLED == "cancelled"

    def test_is_terminal(self) -> None:
        assert not CompileJobStatus.PENDING.is_terminal
        assert not CompileJobStatus.RUNNING.is_terminal
        assert CompileJobStatus.SUCCESS.is_terminal
        assert CompileJobStatus.FAILED.is_terminal
        assert CompileJobStatus.TIMEOUT.is_terminal
        assert CompileJobStatus.CANCELLED.is_terminal

    def test_strenum_behavior(self) -> None:
        assert CompileJobStatus("pending") is CompileJobStatus.PENDING
        assert CompileJobStatus("success") is CompileJobStatus.SUCCESS


class TestCompileJobModel:
    """CompileJob ORM model tests."""

    def test_model_fields(self) -> None:
        assert CompileJob.__tablename__ == "compile_jobs"

    def test_model_has_required_columns(self) -> None:
        columns = {c.name for c in CompileJob.__table__.columns}
        required = {
            "id",
            "project_id",
            "user_id",
            "status",
            "root_file",
            "compiler",
            "build_id",
            "error",
            "logs",
            "created_at",
            "updated_at",
        }
        assert required.issubset(columns)


class TestCompileJobSchemas:
    """Pydantic schema tests."""

    def test_submit_response(self) -> None:
        job_id = uuid.uuid4()
        resp = CompileJobSubmitResponse(job_id=job_id, status=CompileJobStatus.PENDING)
        assert resp.job_id == job_id
        assert resp.status == "pending"

    def test_status_response(self) -> None:
        from datetime import UTC, datetime

        now = datetime.now(UTC)
        resp = CompileJobStatusResponse(
            job_id=uuid.uuid4(),
            status=CompileJobStatus.RUNNING,
            build_id=None,
            error=None,
            logs=None,
            root_file="main.tex",
            compiler="pdflatex",
            elapsed_seconds=5.0,
            created_at=now,
            updated_at=now,
        )
        assert resp.status == "running"
        assert resp.elapsed_seconds == 5.0

    def test_list_item(self) -> None:
        from datetime import UTC, datetime

        now = datetime.now(UTC)
        item = CompileJobListItem(
            job_id=uuid.uuid4(),
            status=CompileJobStatus.FAILED,
            root_file="main.tex",
            compiler="pdflatex",
            build_id=None,
            error="some error",
            created_at=now,
            updated_at=now,
        )
        assert item.error == "some error"

    def test_history_response(self) -> None:
        resp = CompileJobHistoryResponse(items=[], total=0)
        assert resp.items == []
        assert resp.total == 0

    def test_history_response_with_items(self) -> None:
        from datetime import UTC, datetime

        now = datetime.now(UTC)
        items = [
            CompileJobListItem(
                job_id=uuid.uuid4(),
                status=CompileJobStatus.SUCCESS,
                root_file="main.tex",
                compiler="pdflatex",
                created_at=now,
                updated_at=now,
            ),
        ]
        resp = CompileJobHistoryResponse(items=items, total=1)
        assert len(resp.items) == 1
        assert resp.total == 1


class TestCompileQueueService:
    """Compile queue service unit tests (no DB)."""

    def test_queue_service_imports(self) -> None:
        from services.latex.compile_queue import CompileQueueService, get_compile_queue

        assert CompileQueueService is not None
        assert get_compile_queue is not None

    def test_queue_service_singleton(self) -> None:
        from services.latex.compile_queue import get_compile_queue

        q1 = get_compile_queue()
        q2 = get_compile_queue()
        assert q1 is q2

    def test_queue_service_semaphore_config(self) -> None:
        from services.latex.compile_queue import CompileQueueService

        svc = CompileQueueService()
        assert svc._semaphore._value > 0

    def test_queue_service_has_methods(self) -> None:
        from services.latex.compile_queue import CompileQueueService

        svc = CompileQueueService()
        assert hasattr(svc, "submit")
        assert hasattr(svc, "get_status")
        assert hasattr(svc, "cancel")
        assert hasattr(svc, "get_history")
        assert hasattr(svc, "recover_on_startup")
        assert hasattr(svc, "cleanup_old_jobs")
        assert hasattr(svc, "_execute")
        assert hasattr(svc, "_cleanup_clsi")


class TestCompileRouteRegistration:
    """Verify async compile routes are registered."""

    def test_compile_route_exists(self) -> None:
        from api.routes.latex import router

        routes = [r.path for r in router.routes]
        assert "/compile" in routes

    def test_compile_status_route_exists(self) -> None:
        from api.routes.latex import router

        routes = [r.path for r in router.routes]
        assert "/compile/{job_id}/status" in routes

    def test_compile_history_route_exists(self) -> None:
        from api.routes.latex import router

        routes = [r.path for r in router.routes]
        assert "/compile/history" in routes
