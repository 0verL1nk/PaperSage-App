"""Async compile queue service with concurrency control."""

from __future__ import annotations

import asyncio
import hashlib
import logging
import uuid
from collections.abc import AsyncGenerator, Callable
from datetime import UTC, datetime, timedelta

from sqlalchemy import func, select, update
from sqlalchemy.ext.asyncio import AsyncSession

from app.config import get_settings
from models.compile_job import CompileJob, CompileJobStatus
from services.latex.clsi_client import STATUS_CONFLICT, ClsiClient
from services.latex.resource_builder import ProjectFile
from services.latex.service import LaTeXService

logger = logging.getLogger(__name__)


class CompileQueueService:
    """Manages async compile jobs with semaphore-based concurrency control."""

    def __init__(
        self,
        latex_service: LaTeXService | None = None,
        session_factory: Callable[[], AsyncGenerator[AsyncSession, None]] | None = None,
    ) -> None:
        settings = get_settings()
        self._semaphore = asyncio.Semaphore(settings.max_concurrent_compiles)
        self._compile_timeout = settings.latex_compile_timeout
        self._history_limit = settings.compile_history_limit
        self._max_age_hours = settings.compile_job_max_age_hours
        self._latex_service = latex_service or LaTeXService()
        self._session_factory = session_factory
        self._active_tasks: dict[uuid.UUID, asyncio.Task] = {}

    async def submit(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
        root_file: str = "main.tex",
        compiler: str = "pdflatex",
    ) -> CompileJob:
        """Submit a new compile job and schedule background execution."""
        job = CompileJob(
            project_id=project_id,
            user_id=user_id,
            status=CompileJobStatus.PENDING,
            root_file=root_file,
            compiler=compiler,
        )
        db.add(job)
        await db.commit()
        await db.refresh(job)

        task = asyncio.create_task(
            self._execute(job.id),
            name=f"compile-{job.id}",
        )
        self._active_tasks[job.id] = task
        task.add_done_callback(lambda t: self._active_tasks.pop(job.id, None))

        logger.info("Submitted compile job %s for project %s", job.id, project_id)
        return job

    def _get_session_factory(self):
        """Get session factory, falling back to default DB module."""
        if self._session_factory is not None:
            return self._session_factory
        from core.database import async_session_factory

        return async_session_factory

    async def _execute(self, job_id: uuid.UUID) -> None:
        """Execute a compile job with semaphore-controlled concurrency."""
        session_factory = self._get_session_factory()

        async with self._semaphore, session_factory() as db:
            job = await db.get(CompileJob, job_id)
            if not job or job.status != CompileJobStatus.PENDING:
                return

            job.status = CompileJobStatus.RUNNING
            await db.commit()

            try:
                files = await self._load_project_files(job.project_id, db)
                settings = get_settings()
                project_id_str = str(job.project_id).replace("-", "")

                # Determine sync mode
                sync_type, sync_state_val, current_hashes = self._compute_sync_params(
                    files,
                    job,
                    settings.compile_incremental_sync_enabled,
                )
                files_to_send = (
                    self._filter_files_for_incremental(files, job.sync_state)
                    if sync_type == "incremental"
                    else files
                )

                result = await asyncio.wait_for(
                    self._latex_service.compile_project(
                        project_id=project_id_str,
                        files=files_to_send,
                        root_file=job.root_file,
                        compiler=job.compiler,
                        sync_type=sync_type,
                        sync_state=sync_state_val,
                    ),
                    timeout=self._compile_timeout,
                )

                # Conflict → retry with full sync
                if result.status == STATUS_CONFLICT:
                    logger.info("Conflict on job %s, retrying full sync", job_id)
                    result = await asyncio.wait_for(
                        self._latex_service.compile_project(
                            project_id=project_id_str,
                            files=files,
                            root_file=job.root_file,
                            compiler=job.compiler,
                            sync_type="full",
                        ),
                        timeout=self._compile_timeout,
                    )

                job.build_id = result.build_id
                job.logs = result.logs
                job.error = result.error

                if result.status == "success":
                    job.status = CompileJobStatus.SUCCESS
                    job.sync_state = current_hashes
                else:
                    job.status = CompileJobStatus.FAILED

            except TimeoutError:
                job.status = CompileJobStatus.TIMEOUT
                job.error = f"Compilation timed out after {self._compile_timeout}s"
                await self._cleanup_clsi(str(job.project_id))

            except asyncio.CancelledError:
                job.status = CompileJobStatus.CANCELLED
                await self._cleanup_clsi(str(job.project_id))

            except Exception as exc:
                job.status = CompileJobStatus.FAILED
                job.error = str(exc)
                await self._cleanup_clsi(str(job.project_id))
                logger.error("Compile job %s failed: %s", job_id, exc, exc_info=True)

            await db.commit()

    @staticmethod
    def _compute_sync_params(
        files: list[ProjectFile],
        job: CompileJob,
        incremental_enabled: bool,
    ) -> tuple[str, str | None, dict[str, str]]:
        """Determine sync type and build content hash snapshot.

        Returns (sync_type, sync_state_value, current_hashes).
        """
        current_hashes: dict[str, str] = {}
        for f in files:
            if f.content is not None:
                current_hashes[f.path] = hashlib.md5(f.content.encode()).hexdigest()

        if not incremental_enabled or job.sync_state is None:
            return "full", None, current_hashes

        previous = job.sync_state
        # File set changed → full sync
        if set(current_hashes.keys()) != set(previous.keys()):
            return "full", None, current_hashes

        # No content changes → still send full (cheap, avoids edge cases)
        if current_hashes == previous:
            return "full", None, current_hashes

        # Content changed → incremental
        state_parts = [f"{k}:{v}" for k, v in sorted(previous.items())]
        sync_state_val = hashlib.md5(":".join(state_parts).encode()).hexdigest()
        return "incremental", sync_state_val, current_hashes

    @staticmethod
    def _filter_files_for_incremental(
        files: list[ProjectFile],
        previous_state: dict[str, str] | None,
    ) -> list[ProjectFile]:
        """Return only changed text files + all binary files for incremental sync."""
        if previous_state is None:
            return files

        result: list[ProjectFile] = []
        for f in files:
            # Always include binary files (URL-based, cheap)
            if f.is_binary:
                result.append(f)
                continue
            # Include text files whose content changed
            if f.content is not None:
                current_hash = hashlib.md5(f.content.encode()).hexdigest()
                if previous_state.get(f.path) != current_hash:
                    result.append(f)
        return result

    async def _load_project_files(
        self,
        project_id: uuid.UUID,
        db: AsyncSession,
    ) -> list[ProjectFile]:
        """Load project files: text as content, binary as presigned URLs."""
        from services.latex.file_service import LatexFileService

        file_svc = LatexFileService()
        settings = get_settings()
        ttl = settings.s3_presigned_url_ttl_seconds

        # Text files: parallel load from cache/S3
        text_pairs = await file_svc.load_text_contents(project_id, db)

        # Binary files: generate presigned URLs
        binary_keys = await file_svc.get_binary_file_keys(project_id, db)

        files: list[ProjectFile] = [
            ProjectFile(path=path, content=content) for path, content in text_pairs
        ]

        for path, s3_key in binary_keys:
            url = await asyncio.to_thread(file_svc._storage.generate_presigned_url, s3_key, ttl)
            files.append(ProjectFile(path=path, url=url, is_binary=True))

        return files

    async def _cleanup_clsi(self, project_id: str) -> None:
        """Stop compile and clear cache on failure/timeout."""
        try:
            client = ClsiClient()
            await client.stop_compile(project_id)
            await client.clear_cache(project_id)
        except Exception:
            logger.warning("Failed to cleanup CLSI for project %s", project_id, exc_info=True)

    async def get_status(
        self,
        job_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> CompileJob | None:
        """Get compile job status with ownership check."""
        job = await db.get(CompileJob, job_id)
        if not job:
            return None
        if job.user_id != user_id:
            return None
        return job

    async def cancel(
        self,
        job_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> CompileJob | None:
        """Cancel a running or pending compile job."""
        job = await db.get(CompileJob, job_id)
        if not job or job.user_id != user_id:
            return None
        if CompileJobStatus(job.status).is_terminal:
            return job

        task = self._active_tasks.get(job_id)
        if task and not task.done():
            task.cancel()

        job.status = CompileJobStatus.CANCELLED
        await db.commit()
        await db.refresh(job)
        return job

    async def get_history(
        self,
        project_id: uuid.UUID,
        user_id: uuid.UUID,
        db: AsyncSession,
    ) -> tuple[list[CompileJob], int]:
        """Get compile history for a project."""
        base_filter = (
            CompileJob.project_id == project_id,
            CompileJob.user_id == user_id,
        )
        count_result = await db.execute(
            select(func.count()).select_from(CompileJob).where(*base_filter)
        )
        total = count_result.scalar() or 0

        result = await db.execute(
            select(CompileJob)
            .where(*base_filter)
            .order_by(CompileJob.created_at.desc())
            .limit(self._history_limit)
        )
        items = list(result.scalars().all())
        return items, total

    async def recover_on_startup(self) -> None:
        """Recover jobs from previous session on startup."""
        session_factory = self._get_session_factory()

        async with session_factory() as db:
            pending = await db.execute(
                select(CompileJob).where(CompileJob.status == CompileJobStatus.PENDING)
            )
            for job in pending.scalars().all():
                task = asyncio.create_task(
                    self._execute(job.id),
                    name=f"compile-recover-{job.id}",
                )
                self._active_tasks[job.id] = task
                task.add_done_callback(lambda t, jid=job.id: self._active_tasks.pop(jid, None))
                logger.info("Recovered pending job %s", job.id)

            running = await db.execute(
                select(CompileJob).where(CompileJob.status == CompileJobStatus.RUNNING)
            )
            for job in running.scalars().all():
                job.status = CompileJobStatus.FAILED
                job.error = "Process restarted during compilation"
                logger.info("Marked orphaned running job %s as failed", job.id)
            await db.commit()

    async def cleanup_old_jobs(self) -> int:
        """Remove jobs older than max_age_hours. Returns count of cleaned jobs."""
        session_factory = self._get_session_factory()

        async with session_factory() as db:
            cutoff = datetime.now(UTC) - timedelta(hours=self._max_age_hours)
            result = await db.execute(
                update(CompileJob)
                .where(
                    CompileJob.status.in_(
                        [
                            CompileJobStatus.SUCCESS,
                            CompileJobStatus.FAILED,
                            CompileJobStatus.TIMEOUT,
                            CompileJobStatus.CANCELLED,
                        ]
                    ),
                    CompileJob.updated_at < cutoff,
                )
                .values(status=CompileJobStatus.CANCELLED)
                .returning(CompileJob.id)
            )
            cleaned = len(result.all())
            await db.commit()
            return cleaned


_queue_service: CompileQueueService | None = None


def get_compile_queue() -> CompileQueueService:
    """Get or create the singleton compile queue service."""
    global _queue_service
    if _queue_service is None:
        _queue_service = CompileQueueService()
    return _queue_service


def reset_compile_queue() -> None:
    """Reset the singleton — for testing only."""
    global _queue_service
    _queue_service = None
