"""Shared test fixtures."""

import pytest

from app.config import get_settings


@pytest.fixture(autouse=True)
def _mock_s3_settings(monkeypatch):
    """Provide valid S3 env vars so boto3.client() does not fail on init.

    CI has no real S3 credentials; the default placeholder endpoint
    'https://account_id.r2.cloudflarestorage.com' causes ValueError.
    """
    monkeypatch.setenv("S3_ENDPOINT", "http://localhost:9000")
    monkeypatch.setenv("S3_ACCESS_KEY", "test-access-key")
    monkeypatch.setenv("S3_SECRET_KEY", "test-secret-key")
    monkeypatch.setenv("S3_BUCKET", "test-bucket")
    monkeypatch.setenv("S3_REGION", "us-east-1")
    get_settings.cache_clear()
