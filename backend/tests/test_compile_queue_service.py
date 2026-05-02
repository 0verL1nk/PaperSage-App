"""Tests for compile queue service DI and unit-testable behavior."""

from __future__ import annotations

import asyncio
import hashlib
import uuid
from contextlib import asynccontextmanager
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from models.compile_job import CompileJobStatus
from services.latex.compile_queue import (
    CompileQueueService,
    get_compile_queue,
    reset_compile_queue,
)
from services.latex.resource_builder import ProjectFile

# -- Helpers & fixtures -------------------------------------------------------


def _make_pending_job(job_id: uuid.UUID | None = None) -> MagicMock:
    """Create a mock CompileJob with PENDING status."""
    job = MagicMock()
    job.id = job_id or uuid.uuid4()
    job.project_id = uuid.uuid4()
    job.status = CompileJobStatus.PENDING
    job.root_file = "main.tex"
    job.compiler = "pdflatex"
    job.sync_state = None
    return job


def _make_session_factory():
    """Build a mock async session factory with a reusable inner session."""
    session = AsyncMock()
    session.commit = AsyncMock()
    session.refresh = AsyncMock()
    session.get = AsyncMock(return_value=None)
    session.execute = AsyncMock(return_value=MagicMock())
    session.add = MagicMock()

    @asynccontextmanager
    async def factory():
        yield session

    factory._session = session
    return factory


@pytest.fixture(autouse=True)
def _reset_singleton():
    reset_compile_queue()
    yield
    reset_compile_queue()


@pytest.fixture()
def mock_latex_service():
    svc = AsyncMock()
    svc.compile_project = AsyncMock()
    return svc


@pytest.fixture()
def session_factory():
    return _make_session_factory()


@pytest.fixture()
def service(mock_latex_service, session_factory):
    return CompileQueueService(latex_service=mock_latex_service, session_factory=session_factory)


# -- Constructor injection ----------------------------------------------------


class TestConstructorInjection:
    def test_injected_latex_service_is_used(self, mock_latex_service, session_factory):
        svc = CompileQueueService(latex_service=mock_latex_service, session_factory=session_factory)
        assert svc._latex_service is mock_latex_service

    def test_injected_session_factory_is_used(self, mock_latex_service, session_factory):
        svc = CompileQueueService(latex_service=mock_latex_service, session_factory=session_factory)
        assert svc._session_factory is session_factory

    def test_none_latex_service_creates_default(self):
        with patch("services.latex.compile_queue.LaTeXService") as mock_cls:
            instance = MagicMock()
            mock_cls.return_value = instance
            svc = CompileQueueService(latex_service=None)
            mock_cls.assert_called_once()
            assert svc._latex_service is instance

    def test_none_session_factory_stored_as_none(self, mock_latex_service):
        svc = CompileQueueService(latex_service=mock_latex_service, session_factory=None)
        assert svc._session_factory is None

    def test_semaphore_initialized(self, service):
        assert isinstance(service._semaphore, asyncio.Semaphore)
        assert service._semaphore._value > 0

    def test_active_tasks_initialized_empty(self, service):
        assert service._active_tasks == {}


# -- _get_session_factory -----------------------------------------------------


class TestGetSessionFactory:
    def test_returns_injected_factory(self, service, session_factory):
        assert service._get_session_factory() is session_factory

    def test_falls_back_to_database_module(self, mock_latex_service):
        mock_db_factory = MagicMock()
        svc = CompileQueueService(latex_service=mock_latex_service, session_factory=None)
        with patch("core.database.async_session_factory", mock_db_factory):
            assert svc._get_session_factory() is mock_db_factory


# -- Singleton management -----------------------------------------------------


class TestSingletonManagement:
    def test_get_creates_singleton(self):
        assert isinstance(get_compile_queue(), CompileQueueService)

    def test_get_returns_same_instance(self):
        assert get_compile_queue() is get_compile_queue()

    def test_reset_clears_singleton(self):
        q = get_compile_queue()
        reset_compile_queue()
        assert get_compile_queue() is not q

    def test_reset_when_none_is_safe(self):
        reset_compile_queue()
        reset_compile_queue()


# -- _execute: success --------------------------------------------------------


class TestExecuteSuccess:
    @pytest.mark.asyncio
    async def test_success_result(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job
        service._latex_service.compile_project.return_value = MagicMock(
            status="success",
            build_id="build-123",
            logs="log",
            error=None,
        )
        with patch.object(service, "_load_project_files", new_callable=AsyncMock, return_value=[]):
            await service._execute(job.id)
        assert job.status == CompileJobStatus.SUCCESS
        assert job.build_id == "build-123"
        session.commit.assert_called()

    @pytest.mark.asyncio
    async def test_failure_result(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job
        service._latex_service.compile_project.return_value = MagicMock(
            status="failure",
            build_id="build-456",
            logs="err",
            error="bad",
        )
        with patch.object(service, "_load_project_files", new_callable=AsyncMock, return_value=[]):
            await service._execute(job.id)
        assert job.status == CompileJobStatus.FAILED
        assert job.error == "bad"

    @pytest.mark.asyncio
    async def test_success_persists_sync_state(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job
        service._latex_service.compile_project.return_value = MagicMock(
            status="success",
            build_id="build-789",
            logs="",
            error=None,
        )
        files = [ProjectFile(path="main.tex", content="hello")]
        with patch.object(
            service, "_load_project_files", new_callable=AsyncMock, return_value=files
        ):
            await service._execute(job.id)
        assert job.sync_state is not None
        assert "main.tex" in job.sync_state


# -- _execute: error paths ----------------------------------------------------


class TestExecuteErrors:
    @pytest.mark.asyncio
    async def test_timeout(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job
        service._latex_service.compile_project.side_effect = TimeoutError()
        with (
            patch.object(service, "_load_project_files", new_callable=AsyncMock, return_value=[]),
            patch.object(service, "_cleanup_clsi", new_callable=AsyncMock),
        ):
            await service._execute(job.id)
        assert job.status == CompileJobStatus.TIMEOUT
        assert "timed out" in job.error

    @pytest.mark.asyncio
    async def test_cancelled(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job
        service._latex_service.compile_project.side_effect = asyncio.CancelledError()
        with (
            patch.object(service, "_load_project_files", new_callable=AsyncMock, return_value=[]),
            patch.object(service, "_cleanup_clsi", new_callable=AsyncMock),
        ):
            await service._execute(job.id)
        assert job.status == CompileJobStatus.CANCELLED

    @pytest.mark.asyncio
    async def test_generic_exception(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job
        service._latex_service.compile_project.side_effect = RuntimeError("boom")
        with (
            patch.object(service, "_load_project_files", new_callable=AsyncMock, return_value=[]),
            patch.object(service, "_cleanup_clsi", new_callable=AsyncMock),
        ):
            await service._execute(job.id)
        assert job.status == CompileJobStatus.FAILED
        assert job.error == "boom"

    @pytest.mark.asyncio
    async def test_skips_missing_job(self, service, session_factory):
        session_factory._session.get.return_value = None
        await service._execute(uuid.uuid4())
        service._latex_service.compile_project.assert_not_called()

    @pytest.mark.asyncio
    async def test_skips_non_pending_job(self, service, session_factory):
        job = _make_pending_job()
        job.status = CompileJobStatus.SUCCESS
        session_factory._session.get.return_value = job
        await service._execute(uuid.uuid4())
        service._latex_service.compile_project.assert_not_called()


# -- _cleanup_clsi ------------------------------------------------------------


class TestCleanupClsi:
    @pytest.mark.asyncio
    async def test_swallows_exceptions(self, service):
        with patch("services.latex.compile_queue.ClsiClient") as mock_cls:
            client = AsyncMock()
            client.stop_compile.side_effect = ConnectionError("refused")
            mock_cls.return_value = client
            await service._cleanup_clsi("proj-1")


# -- recover_on_startup -------------------------------------------------------


class TestRecoverOnStartup:
    @pytest.mark.asyncio
    async def test_marks_running_jobs_as_failed(self, service, session_factory):
        session = session_factory._session
        pending_job = _make_pending_job()
        running_job = _make_pending_job()
        running_job.status = CompileJobStatus.RUNNING
        call_n = 0

        def side_effect(_stmt):
            nonlocal call_n
            call_n += 1
            m = MagicMock()
            m.scalars.return_value.all.return_value = (
                [pending_job] if call_n == 1 else [running_job]
            )
            return m

        session.execute = AsyncMock(side_effect=side_effect)
        with patch.object(service, "_execute", new_callable=AsyncMock):
            await service.recover_on_startup()
        assert running_job.status == CompileJobStatus.FAILED
        assert running_job.error == "Process restarted during compilation"


# -- cleanup_old_jobs ---------------------------------------------------------


class TestCleanupOldJobs:
    @pytest.mark.asyncio
    async def test_returns_cleaned_count(self, service, session_factory):
        session = session_factory._session
        result_mock = MagicMock()
        result_mock.all.return_value = [uuid.uuid4(), uuid.uuid4()]
        session.execute = AsyncMock(return_value=result_mock)
        cleaned = await service.cleanup_old_jobs()
        assert cleaned == 2
        session.commit.assert_called()


# ---------------------------------------------------------------------------
# get_status
# ---------------------------------------------------------------------------


class TestGetStatus:
    @pytest.mark.asyncio
    async def test_returns_job_when_owner_matches(self, service):
        db = AsyncMock()
        job = MagicMock()
        job.user_id = uuid.UUID(int=1)
        db.get = AsyncMock(return_value=job)

        result = await service.get_status(job.id, uuid.UUID(int=1), db)
        assert result is job

    @pytest.mark.asyncio
    async def test_returns_none_when_not_found(self, service):
        db = AsyncMock()
        db.get = AsyncMock(return_value=None)

        result = await service.get_status(uuid.uuid4(), uuid.uuid4(), db)
        assert result is None

    @pytest.mark.asyncio
    async def test_returns_none_when_wrong_owner(self, service):
        db = AsyncMock()
        job = MagicMock()
        job.user_id = uuid.UUID(int=1)
        db.get = AsyncMock(return_value=job)

        result = await service.get_status(job.id, uuid.UUID(int=2), db)
        assert result is None


# ---------------------------------------------------------------------------
# cancel
# ---------------------------------------------------------------------------


class TestCancel:
    @pytest.mark.asyncio
    async def test_cancel_pending_job(self, service):
        db = AsyncMock()
        job = MagicMock()
        job.id = uuid.uuid4()
        job.user_id = uuid.UUID(int=1)
        job.status = CompileJobStatus.PENDING
        db.get = AsyncMock(return_value=job)

        result = await service.cancel(job.id, uuid.UUID(int=1), db)
        assert result is job
        assert job.status == CompileJobStatus.CANCELLED
        db.commit.assert_called()

    @pytest.mark.asyncio
    async def test_cancel_terminal_job_returns_unchanged(self, service):
        db = AsyncMock()
        job = MagicMock()
        job.id = uuid.uuid4()
        job.user_id = uuid.UUID(int=1)
        job.status = CompileJobStatus.SUCCESS
        db.get = AsyncMock(return_value=job)

        result = await service.cancel(job.id, uuid.UUID(int=1), db)
        assert result is job
        assert job.status == CompileJobStatus.SUCCESS

    @pytest.mark.asyncio
    async def test_cancel_not_found(self, service):
        db = AsyncMock()
        db.get = AsyncMock(return_value=None)

        result = await service.cancel(uuid.uuid4(), uuid.uuid4(), db)
        assert result is None

    @pytest.mark.asyncio
    async def test_cancel_wrong_owner(self, service):
        db = AsyncMock()
        job = MagicMock()
        job.user_id = uuid.UUID(int=1)
        db.get = AsyncMock(return_value=job)

        result = await service.cancel(job.id, uuid.UUID(int=2), db)
        assert result is None

    @pytest.mark.asyncio
    async def test_cancel_cancels_active_task(self, service):
        db = AsyncMock()
        job = MagicMock()
        job.id = uuid.uuid4()
        job.user_id = uuid.UUID(int=1)
        job.status = CompileJobStatus.RUNNING
        db.get = AsyncMock(return_value=job)

        mock_task = MagicMock()
        mock_task.done.return_value = False
        service._active_tasks[job.id] = mock_task

        result = await service.cancel(job.id, uuid.UUID(int=1), db)  # noqa: F841
        mock_task.cancel.assert_called_once()
        assert job.status == CompileJobStatus.CANCELLED


# ---------------------------------------------------------------------------
# get_history
# ---------------------------------------------------------------------------


class TestGetHistory:
    @pytest.mark.asyncio
    async def test_returns_items_and_total(self, service):
        db = AsyncMock()

        count_result = MagicMock()
        count_result.scalar.return_value = 2

        job1, job2 = MagicMock(), MagicMock()
        list_result = MagicMock()
        list_result.scalars.return_value.all.return_value = [job1, job2]

        db.execute = AsyncMock(side_effect=[count_result, list_result])

        items, total = await service.get_history(uuid.uuid4(), uuid.uuid4(), db)
        assert total == 2
        assert len(items) == 2

    @pytest.mark.asyncio
    async def test_returns_empty_when_no_jobs(self, service):
        db = AsyncMock()

        count_result = MagicMock()
        count_result.scalar.return_value = 0

        list_result = MagicMock()
        list_result.scalars.return_value.all.return_value = []

        db.execute = AsyncMock(side_effect=[count_result, list_result])

        items, total = await service.get_history(uuid.uuid4(), uuid.uuid4(), db)
        assert total == 0
        assert items == []


# ---------------------------------------------------------------------------
# _compute_sync_params
# ---------------------------------------------------------------------------


class TestComputeSyncParams:
    def test_full_sync_when_disabled(self):
        job = MagicMock(sync_state={"main.tex": "abc123"})
        files = [ProjectFile(path="main.tex", content="hello")]
        sync_type, state_val, hashes = CompileQueueService._compute_sync_params(
            files,
            job,
            incremental_enabled=False,
        )
        assert sync_type == "full"
        assert state_val is None
        assert "main.tex" in hashes

    def test_full_sync_when_no_previous_state(self):
        job = MagicMock(sync_state=None)
        files = [ProjectFile(path="main.tex", content="hello")]
        sync_type, state_val, hashes = CompileQueueService._compute_sync_params(
            files,
            job,
            incremental_enabled=True,
        )
        assert sync_type == "full"
        assert state_val is None

    def test_full_sync_when_file_set_changed(self):
        job = MagicMock(sync_state={"main.tex": "abc123"})
        files = [
            ProjectFile(path="main.tex", content="hello"),
            ProjectFile(path="intro.tex", content="intro"),
        ]
        sync_type, state_val, _ = CompileQueueService._compute_sync_params(
            files,
            job,
            incremental_enabled=True,
        )
        assert sync_type == "full"
        assert state_val is None

    def test_full_sync_when_no_content_changes(self):
        content = "hello"
        h = hashlib.md5(content.encode()).hexdigest()
        job = MagicMock(sync_state={"main.tex": h})
        files = [ProjectFile(path="main.tex", content=content)]
        sync_type, state_val, _ = CompileQueueService._compute_sync_params(
            files,
            job,
            incremental_enabled=True,
        )
        assert sync_type == "full"
        assert state_val is None

    def test_incremental_when_content_changed(self):
        job = MagicMock(sync_state={"main.tex": "old_hash"})
        files = [ProjectFile(path="main.tex", content="new content")]
        sync_type, state_val, hashes = CompileQueueService._compute_sync_params(
            files,
            job,
            incremental_enabled=True,
        )
        assert sync_type == "incremental"
        assert state_val is not None
        assert hashes["main.tex"] != "old_hash"

    def test_binary_files_excluded_from_hashes(self):
        job = MagicMock(sync_state={"main.tex": "abc"})
        files = [
            ProjectFile(path="main.tex", content="hello", is_binary=False),
            ProjectFile(path="fig.png", url="http://x.com/fig.png", is_binary=True),
        ]
        _, _, hashes = CompileQueueService._compute_sync_params(
            files,
            job,
            incremental_enabled=False,
        )
        assert "main.tex" in hashes
        assert "fig.png" not in hashes


# ---------------------------------------------------------------------------
# _filter_files_for_incremental
# ---------------------------------------------------------------------------


class TestFilterFilesForIncremental:
    def test_returns_all_when_no_previous_state(self):
        files = [ProjectFile(path="main.tex", content="hello")]
        result = CompileQueueService._filter_files_for_incremental(files, None)
        assert len(result) == 1

    def test_includes_changed_text_files(self):
        old_hash = hashlib.md5(b"old content").hexdigest()
        previous = {"main.tex": old_hash}
        files = [ProjectFile(path="main.tex", content="new content")]
        result = CompileQueueService._filter_files_for_incremental(files, previous)
        assert len(result) == 1

    def test_excludes_unchanged_text_files(self):
        content = "same content"
        h = hashlib.md5(content.encode()).hexdigest()
        previous = {"main.tex": h}
        files = [ProjectFile(path="main.tex", content=content)]
        result = CompileQueueService._filter_files_for_incremental(files, previous)
        assert len(result) == 0

    def test_always_includes_binary_files(self):
        previous = {"main.tex": "abc123"}
        files = [
            ProjectFile(path="main.tex", content="changed"),
            ProjectFile(path="fig.png", url="http://x.com/fig.png", is_binary=True),
        ]
        result = CompileQueueService._filter_files_for_incremental(files, previous)
        assert len(result) == 2

    def test_binary_only_when_text_unchanged(self):
        content = "same"
        h = hashlib.md5(content.encode()).hexdigest()
        previous = {"main.tex": h}
        files = [
            ProjectFile(path="main.tex", content=content),
            ProjectFile(path="fig.png", url="http://x.com/fig.png", is_binary=True),
        ]
        result = CompileQueueService._filter_files_for_incremental(files, previous)
        assert len(result) == 1
        assert result[0].path == "fig.png"


# ---------------------------------------------------------------------------
# Conflict retry
# ---------------------------------------------------------------------------


class TestExecuteConflictRetry:
    @pytest.mark.asyncio
    async def test_conflict_retries_full_sync(self, service, session_factory):
        session = session_factory._session
        job = _make_pending_job()
        session.get.return_value = job

        conflict_result = MagicMock(status="conflict", build_id=None, logs="", error=None)
        success_result = MagicMock(
            status="success",
            build_id="build-retry",
            logs="",
            error=None,
        )
        service._latex_service.compile_project.side_effect = [conflict_result, success_result]

        with patch.object(service, "_load_project_files", new_callable=AsyncMock, return_value=[]):
            await service._execute(job.id)

        assert job.status == CompileJobStatus.SUCCESS
        assert job.build_id == "build-retry"
        assert service._latex_service.compile_project.call_count == 2
        # Second call should be full sync
        second_call = service._latex_service.compile_project.call_args_list[1]
        assert (
            second_call.kwargs.get("sync_type") == "full"
            or second_call[1].get("sync_type") == "full"
        )
