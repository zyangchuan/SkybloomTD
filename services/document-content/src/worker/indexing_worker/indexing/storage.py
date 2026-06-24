from __future__ import annotations

import logging
from pathlib import Path

import boto3
from botocore.config import Config

from indexing_worker.config import AWS_REGION, AWS_S3_ENDPOINT_URL

LOG = logging.getLogger(__name__)

S3_CLIENT = boto3.client(
    "s3",
    region_name=AWS_REGION,
    endpoint_url=AWS_S3_ENDPOINT_URL,
    config=Config(s3={"addressing_style": "path"}),
)


def download_file_from_s3(s3_bucket: str, s3_key: str, destination: Path) -> Path:
    destination.parent.mkdir(parents=True, exist_ok=True)
    LOG.info("downloading OCR markdown from s3 bucket=%s key=%s", s3_bucket, s3_key)
    S3_CLIENT.download_file(s3_bucket, s3_key, str(destination))
    return destination
