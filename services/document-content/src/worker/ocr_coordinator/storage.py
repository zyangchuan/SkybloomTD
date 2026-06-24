import boto3
from botocore.config import Config

from ocr_coordinator.config import AWS_REGION, AWS_S3_ENDPOINT_URL

S3_CLIENT = boto3.client(
    "s3",
    region_name=AWS_REGION,
    endpoint_url=AWS_S3_ENDPOINT_URL,
    config=Config(signature_version="s3v4", s3={"addressing_style": "path"}),
)


def source_s3_key(user_id: str, document_id: str, source_filename: str) -> str:
    return f"{user_id}/{document_id}/source/{source_filename}"


def output_markdown_s3_key(user_id: str, document_id: str) -> str:
    return f"{user_id}/{document_id}/output.md"


def presigned_download_url(s3_bucket: str, s3_key: str, expires_in: int) -> str:
    return S3_CLIENT.generate_presigned_url(
        "get_object",
        Params={"Bucket": s3_bucket, "Key": s3_key},
        ExpiresIn=expires_in,
    )


def presigned_upload_url(s3_bucket: str, s3_key: str, expires_in: int) -> str:
    return S3_CLIENT.generate_presigned_url(
        "put_object",
        Params={"Bucket": s3_bucket, "Key": s3_key},
        ExpiresIn=expires_in,
    )
