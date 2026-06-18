import logging
from pathlib import Path

import boto3
from botocore.config import Config

from ..config import AWS_REGION, AWS_S3_BUCKET, AWS_S3_ENDPOINT_URL

LOG = logging.getLogger(__name__)

S3_CLIENT = boto3.client(
    "s3",
    region_name=AWS_REGION,
    endpoint_url=AWS_S3_ENDPOINT_URL,
    config=Config(s3={"addressing_style": "path"}),
)

def upload_output_to_s3(document_output_dir: Path, user_id: str, document_id: str) -> dict:
    upload_directory_path = "/".join(
        part.strip("/") for part in [user_id, document_id] if part
    )

    output_file = document_output_dir / "output.md"
    s3_key = f"{upload_directory_path}/output.md"

    S3_CLIENT.upload_file(
        str(output_file),
        AWS_S3_BUCKET,
        s3_key,
        ExtraArgs={"ContentType": "text/markdown; charset=utf-8"},
    )

    return {
        "status": "uploaded",
        "s3_bucket": AWS_S3_BUCKET,
        "s3_directory_path": upload_directory_path,
        "s3_markdown_key": s3_key,
    }

def download_file_from_s3(s3_bucket: str, s3_key: str, destination: Path) -> Path:
    destination.parent.mkdir(parents=True, exist_ok=True)
    LOG.info("downloading source file from s3 bucket=%s key=%s", s3_bucket, s3_key)
    S3_CLIENT.download_file(s3_bucket, s3_key, str(destination))
    return destination
