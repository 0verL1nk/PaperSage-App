"""S3 storage client for LaTeX file persistence."""

from __future__ import annotations

import logging
from pathlib import PurePosixPath
from typing import Any

import boto3
from botocore.exceptions import ClientError

from app.config import get_settings

logger = logging.getLogger(__name__)


class S3Storage:
    """S3-compatible storage client (works with MinIO)."""

    def __init__(self, client: Any | None = None) -> None:
        settings = get_settings()
        self._bucket = settings.s3_bucket
        self._client = client or boto3.client(
            "s3",
            endpoint_url=settings.s3_endpoint,
            aws_access_key_id=settings.s3_access_key,
            aws_secret_access_key=settings.s3_secret_key,
            region_name=settings.s3_region,
        )
        self._ensure_bucket()

    def _ensure_bucket(self) -> None:
        """Create bucket if it does not exist."""
        try:
            self._client.head_bucket(Bucket=self._bucket)
        except ClientError:
            self._client.create_bucket(Bucket=self._bucket)
            logger.info("Created S3 bucket: %s", self._bucket)

    def put_object(self, key: str, data: bytes | str) -> None:
        """Upload an object to S3."""
        if isinstance(data, str):
            data = data.encode("utf-8")
        self._client.put_object(Bucket=self._bucket, Key=key, Body=data)

    def get_object(self, key: str) -> bytes:
        """Download an object from S3."""
        resp = self._client.get_object(Bucket=self._bucket, Key=key)
        return resp["Body"].read()

    def delete_object(self, key: str) -> None:
        """Delete an object from S3."""
        self._client.delete_object(Bucket=self._bucket, Key=key)

    def list_objects(self, prefix: str) -> list[str]:
        """List object keys under a prefix."""
        keys: list[str] = []
        paginator = self._client.get_paginator("list_objects_v2")
        for page in paginator.paginate(Bucket=self._bucket, Prefix=prefix):
            for obj in page.get("Contents", []):
                keys.append(obj["Key"])
        return keys

    def object_exists(self, key: str) -> bool:
        """Check if an object exists in S3."""
        try:
            self._client.head_object(Bucket=self._bucket, Key=key)
            return True
        except ClientError:
            return False

    @staticmethod
    def build_key(project_id: str, file_path: str) -> str:
        """Build S3 key for a project file."""
        return str(PurePosixPath("projects") / project_id / "latex" / file_path)
