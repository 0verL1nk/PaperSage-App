"""S3 storage service tests using moto mock."""

import boto3
import pytest
from moto import mock_aws

from app.config import get_settings
from services.storage.s3_client import S3Storage


@pytest.fixture(autouse=True)
def _fresh_settings(monkeypatch):
    """Reset settings cache so env vars are picked up."""
    get_settings.cache_clear()
    monkeypatch.setenv("S3_BUCKET", "test-bucket")
    monkeypatch.setenv("S3_REGION", "us-east-1")


@pytest.fixture
def s3_client(_fresh_settings):
    """Provide an S3Storage instance with mocked S3."""
    with mock_aws():
        mock_client = boto3.client("s3", region_name="us-east-1")
        yield S3Storage(client=mock_client)


class TestS3Storage:
    def test_auto_bucket_creation(self, _fresh_settings):
        """Bucket is created automatically on init."""
        with mock_aws():
            mock_client = boto3.client("s3", region_name="us-east-1")
            storage = S3Storage(client=mock_client)
            resp = storage._client.head_bucket(Bucket="test-bucket")
            assert resp is not None

    def test_put_and_get_object(self, s3_client):
        key = "projects/abc/latex/main.tex"
        data = r"\documentclass{article}"
        s3_client.put_object(key, data)

        result = s3_client.get_object(key)
        assert result == data.encode("utf-8")

    def test_put_binary_data(self, s3_client):
        key = "projects/abc/latex/image.png"
        data = b"\x89PNG\r\n"
        s3_client.put_object(key, data)

        result = s3_client.get_object(key)
        assert result == data

    def test_delete_object(self, s3_client):
        key = "projects/abc/latex/main.tex"
        s3_client.put_object(key, "content")
        s3_client.delete_object(key)

        assert not s3_client.object_exists(key)

    def test_list_objects(self, s3_client):
        prefix = "projects/abc/latex/"
        s3_client.put_object(f"{prefix}main.tex", "main")
        s3_client.put_object(f"{prefix}refs.bib", "refs")

        keys = s3_client.list_objects(prefix)
        assert len(keys) == 2
        assert f"{prefix}main.tex" in keys
        assert f"{prefix}refs.bib" in keys

    def test_object_exists_true(self, s3_client):
        key = "projects/abc/latex/main.tex"
        s3_client.put_object(key, "content")
        assert s3_client.object_exists(key)

    def test_object_exists_false(self, s3_client):
        assert not s3_client.object_exists("nonexistent/key")

    def test_build_key(self):
        key = S3Storage.build_key("abc-123", "chapter1/intro.tex")
        assert key == "projects/abc-123/latex/chapter1/intro.tex"
