import mimetypes
from pathlib import Path
from urllib.parse import urlparse

import boto3

from ..config import AWS_REGION, AWS_S3_BUCKET, AWS_S3_ENDPOINT_URL, AWS_S3_PREFIX


def s3_client():
    kwargs = {}
    if AWS_REGION:
        kwargs["region_name"] = AWS_REGION
    if AWS_S3_ENDPOINT_URL:
        kwargs["endpoint_url"] = AWS_S3_ENDPOINT_URL
    return boto3.client("s3", **kwargs)


def configured_s3_location() -> tuple[str | None, str]:
    if not AWS_S3_BUCKET:
        return None, AWS_S3_PREFIX

    bucket = AWS_S3_BUCKET.strip()
    prefix = AWS_S3_PREFIX

    if bucket.startswith("s3://"):
        parsed = urlparse(bucket)
        bucket = parsed.netloc
        uri_prefix = parsed.path.strip("/")
        prefix = "/".join(part.strip("/") for part in [uri_prefix, prefix] if part)

    return bucket, prefix


def s3_document_prefix(base_prefix: str, user_id: str, document_id: str) -> str:
    parts = [base_prefix, "users", user_id, "documents", document_id]
    return "/".join(part.strip("/") for part in parts if part)


def upload_output_to_s3(document_output_dir: Path, user_id: str, document_id: str) -> dict:
    bucket, base_output_prefix = configured_s3_location()

    if not bucket:
        return {
            "status": "skipped",
            "reason": "AWS_S3_BUCKET is not configured",
        }

    client = s3_client()
    base_prefix = s3_document_prefix(base_output_prefix, user_id, document_id)
    uploaded_files = []
    keys_by_path = {}

    for local_path in sorted(document_output_dir.rglob("*")):
        if not local_path.is_file():
            continue

        relative_path = local_path.relative_to(document_output_dir).as_posix()
        s3_key = f"{base_prefix}/{relative_path}"
        content_type = mimetypes.guess_type(local_path.name)[0]

        if content_type:
            client.upload_file(
                str(local_path),
                bucket,
                s3_key,
                ExtraArgs={"ContentType": content_type},
            )
        else:
            client.upload_file(str(local_path), bucket, s3_key)

        uploaded_files.append(s3_key)
        keys_by_path[relative_path] = s3_key

    return {
        "status": "uploaded",
        "bucket": bucket,
        "prefix": base_prefix,
        "markdown_key": keys_by_path.get("output.md"),
        "files": uploaded_files,
        "keys_by_path": keys_by_path,
    }


def download_text_from_s3(bucket: str, key: str) -> str:
    response = s3_client().get_object(Bucket=bucket, Key=key)
    return response["Body"].read().decode("utf-8")
