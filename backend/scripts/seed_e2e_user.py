"""Seed the e2e SQLite database with deterministic test data.

Runs as part of the GitHub Actions E2E workflow. Idempotent: re-running
on an already-seeded DB is a no-op (it only inserts when the row is
absent). Outputs E2E_USER / E2E_PASS / E2E_PROJECT_ID to GITHUB_ENV
so subsequent steps and the spec helpers can read them.

Usage:
    cd backend && uv run python scripts/seed_e2e_user.py
"""

from __future__ import annotations

import asyncio
import hashlib
import os
import sys
import time
import uuid
from pathlib import Path

# Make `core`, `models`, etc. importable when run from backend/
BACKEND_ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(BACKEND_ROOT))

import boto3  # noqa: E402
from botocore.config import Config as BotoConfig  # noqa: E402
from botocore.exceptions import ClientError, EndpointConnectionError  # noqa: E402
from sqlalchemy import select  # noqa: E402
from sqlalchemy.ext.asyncio import (  # noqa: E402
    AsyncSession,
    async_sessionmaker,
    create_async_engine,
)

from core.base import Base  # noqa: E402
from core.security import hash_password  # noqa: E402
from models import LatexFile, Project, ProjectGroup, User  # noqa: E402

# Deterministic IDs make spec assertions easier and let us locate the
# same row across re-runs.
E2E_USER_ID = uuid.UUID("11111111-1111-1111-1111-111111111111")
E2E_PROJECT_ID = uuid.UUID("22222222-2222-2222-2222-222222222222")
E2E_GROUP_ID = uuid.UUID("33333333-3333-3333-3333-333333333333")

E2E_USERNAME = "e2euser"
# Must satisfy the zod PASSWORD_PATTERN: at least 8 chars, letter and digit.
E2E_PASSWORD = "e2epass1"
E2E_PROJECT_NAME = "default-project"


def _frontend_hashed_password(plain: str) -> str:
    """Match the frontend's double-hash: SHA-256(plain)."""
    return hashlib.sha256(plain.encode("utf-8")).hexdigest()


def _file_seed() -> list[tuple[str, str, str]]:
    """(file_path, content_type, content) tuples for the seed project."""
    return [
        (
            "main.tex",
            "text/x-tex",
            "\\documentclass{article}\n\\begin{document}\nHello\n\\end{document}\n",
        ),
        ("refs.bib", "text/x-bibtex", "@misc{test, title={Test}, year={2025}}\n"),
        (
            "sections/intro.tex",
            "text/x-tex",
            "\\section{Introduction}\nIntro text.\n",
        ),
    ]


def _s3_client():
    """Build a boto3 S3 client from CI env vars (mirrors S3Storage but standalone)."""
    endpoint = os.environ.get("S3_ENDPOINT", "")
    access = os.environ.get("S3_ACCESS_KEY", "")
    secret = os.environ.get("S3_SECRET_KEY", "")
    region = os.environ.get("S3_REGION", "us-east-1") or "us-east-1"
    if region == "auto":
        region = "us-east-1"
    return boto3.client(
        "s3",
        endpoint_url=endpoint,
        aws_access_key_id=access,
        aws_secret_access_key=secret,
        region_name=region,
        config=BotoConfig(signature_version="s3v4"),
    )


def _wait_for_s3(timeout_s: int = 60) -> None:
    """Poll S3 until it responds or the budget runs out.

    The minio service container is healthy by the time the runner starts
    the main steps, but the bucket may take a moment to be reachable from
    boto3. Best-effort: if S3 never comes up, log a warning and continue —
    the DB rows still get seeded, only the file content fetch would 404.
    """
    bucket = os.environ.get("S3_BUCKET", "papersage")
    if not os.environ.get("S3_ENDPOINT"):
        print("! S3_ENDPOINT not set, skipping S3 seed")
        return
    client = _s3_client()
    deadline = time.monotonic() + timeout_s
    while time.monotonic() < deadline:
        try:
            client.head_bucket(Bucket=bucket)
            print(f"✓ S3 bucket '{bucket}' reachable")
            return
        except (ClientError, EndpointConnectionError, Exception) as exc:  # noqa: BLE001
            print(f"… waiting for S3 ({type(exc).__name__})")
            time.sleep(2)
    print(
        f"! S3 bucket '{bucket}' not reachable after {timeout_s}s — "
        f"latex file content will be missing from S3"
    )


def _seed_s3_objects(file_seed: list[tuple[str, str, str]]) -> None:
    """Upload the seed latex contents to S3 so GET /latex-files/{path} works.

    Mirrors the DB-seed idempotency contract: existing objects are left
    alone. The DB row's s3_key is the source of truth.
    """
    bucket = os.environ.get("S3_BUCKET", "papersage")
    project_id = E2E_PROJECT_ID
    if not os.environ.get("S3_ENDPOINT"):
        return
    client = _s3_client()
    for path, ctype, content in file_seed:
        key = f"projects/{project_id}/latex/{path}"
        try:
            client.head_object(Bucket=bucket, Key=key)
            print(f"= S3 object {key} already exists, skipping")
            continue
        except ClientError:
            pass
        try:
            client.put_object(
                Bucket=bucket,
                Key=key,
                Body=content.encode("utf-8"),
                ContentType=ctype,
            )
            print(f"✓ Uploaded S3 object {key}")
        except Exception as exc:  # noqa: BLE001
            print(f"! Failed to upload S3 object {key}: {exc}")


async def _seed() -> None:
    database_url = os.environ.get("DATABASE_URL")
    if not database_url:
        raise SystemExit("DATABASE_URL must be set (workflow env: sqlite+aiosqlite:///./e2e.db)")

    engine = create_async_engine(database_url, future=True)
    factory = async_sessionmaker(engine, expire_on_commit=False, class_=AsyncSession)

    # Create all tables (mirrors the lifespan's `create_all` so this
    # script works even if the backend is started AFTER it).
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)

    async with factory() as session:  # type: AsyncSession
        # ── user ────────────────────────────────────────────
        existing_user = await session.scalar(select(User).where(User.username == E2E_USERNAME))
        if existing_user is None:
            sha = _frontend_hashed_password(E2E_PASSWORD)
            user = User(
                id=E2E_USER_ID,
                username=E2E_USERNAME,
                password_hash=hash_password(sha),
                email="e2e@papersage.test",
                is_active=True,
            )
            session.add(user)
            print(f"✓ Created user id={E2E_USER_ID} username={E2E_USERNAME}")
        else:
            print(f"= User {E2E_USERNAME} already exists, skipping")

        # ── group ───────────────────────────────────────────
        existing_group = await session.get(ProjectGroup, E2E_GROUP_ID)
        if existing_group is None:
            group = ProjectGroup(
                id=E2E_GROUP_ID,
                name="e2e-default-group",
                owner_id=E2E_USER_ID,
                sort_order=0,
            )
            session.add(group)
            print(f"✓ Created group id={E2E_GROUP_ID}")
        else:
            print(f"= Group {E2E_GROUP_ID} already exists, skipping")

        # ── project ─────────────────────────────────────────
        existing_project = await session.get(Project, E2E_PROJECT_ID)
        if existing_project is None:
            project = Project(
                id=E2E_PROJECT_ID,
                name=E2E_PROJECT_NAME,
                description="E2E seed project",
                owner_id=E2E_USER_ID,
                group_id=E2E_GROUP_ID,
                status="active",
                is_deleted=False,
            )
            session.add(project)
            await session.flush()  # need project.id for FK
            print(f"✓ Created project id={E2E_PROJECT_ID}")
        else:
            project = existing_project
            print(f"= Project {E2E_PROJECT_ID} already exists, skipping")

        # ── latex files ─────────────────────────────────────
        existing_files = await session.scalars(
            select(LatexFile).where(LatexFile.project_id == project.id)
        )
        existing_file_paths = {f.file_path for f in existing_files.all()}
        for path, ctype, content in _file_seed():
            if path in existing_file_paths:
                print(f"= File {path} already exists, skipping")
                continue
            content_bytes = content.encode("utf-8")
            session.add(
                LatexFile(
                    project_id=project.id,
                    file_path=path,
                    content_type=ctype,
                    size_bytes=len(content_bytes),
                    content_hash=hashlib.sha256(content_bytes).hexdigest(),
                    s3_key=f"projects/{project.id}/latex/{path}",
                )
            )
            print(f"✓ Created latex file {path}")

        await session.commit()

    await engine.dispose()


def _write_github_env() -> None:
    """Export env vars for downstream steps (and Playwright)."""
    env_path = os.environ.get("GITHUB_ENV")
    if not env_path:
        return
    with open(env_path, "a", encoding="utf-8") as f:
        f.write(f"E2E_USER={E2E_USERNAME}\n")
        f.write(f"E2E_PASS={E2E_PASSWORD}\n")
        f.write(f"E2E_PROJECT_ID={E2E_PROJECT_ID}\n")
        f.write(f"E2E_USER_ID={E2E_USER_ID}\n")
        f.write(f"E2E_GROUP_ID={E2E_GROUP_ID}\n")
    print(f"→ Wrote E2E_* vars to {env_path}")


if __name__ == "__main__":
    # Seed S3 first (network call, may need to wait for MinIO to be ready).
    # Best-effort: failures are logged but do not abort — the DB seed is the
    # primary purpose; missing S3 content only breaks file-content fetches.
    _wait_for_s3()
    _seed_s3_objects(_file_seed())
    asyncio.run(_seed())
    _write_github_env()
    print("Seed complete.")
