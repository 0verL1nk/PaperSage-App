"""S3 storage client for LaTeX file persistence."""

from __future__ import annotations

import logging
from pathlib import PurePosixPath
from typing import Any

import boto3
from botocore.config import Config as BotoConfig
from botocore.exceptions import ClientError

from app.config import get_settings

logger = logging.getLogger(__name__)


class S3Storage:
    """S3-compatible storage client (Cloudflare R2 / MinIO / AWS S3)."""

    def __init__(self, client: Any | None = None) -> None:
        settings = get_settings()
        self._bucket = settings.s3_bucket
        # R2 requires "us-east-1" as region regardless of actual location
        region = settings.s3_region
        if region == "auto":
            region = "us-east-1"
        self._client = client or boto3.client(
            "s3",
            endpoint_url=settings.s3_endpoint,
            aws_access_key_id=settings.s3_access_key,
            aws_secret_access_key=settings.s3_secret_key,
            region_name=region,
            config=BotoConfig(signature_version="s3v4"),
        )
        # Removed self._ensure_bucket() to avoid blocking I/O on init

    def _ensure_bucket(self) -> None:
        """Verify bucket exists. R2 buckets must be created via dashboard."""
        try:
            self._client.head_bucket(Bucket=self._bucket)
        except ClientError:
            # Try creating for local MinIO dev; R2 will fail gracefully
            try:
                self._client.create_bucket(Bucket=self._bucket)
                logger.info("Created S3 bucket: %s", self._bucket)
            except ClientError:
                logger.warning(
                    "Bucket %s not found and cannot be auto-created. "
                    "Create it manually in your S3 provider dashboard.",
                    self._bucket,
                )
                raise

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

    def generate_presigned_url(self, key: str, expires_in: int = 300) -> str:
        """Generate a presigned URL for temporary read access."""
        return self._client.generate_presigned_url(
            "get_object",
            Params={"Bucket": self._bucket, "Key": key},
            ExpiresIn=expires_in,
        )

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
