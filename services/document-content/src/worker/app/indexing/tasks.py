from pathlib import Path
from typing import Any

from ..celery_app import celery_app
from ..config import CHUNK_MAX_CHARS, CHUNK_OVERLAP_LINES, ENABLE_EMBEDDINGS
from ..ocr.output import remove_document_output
from ..uploads.s3 import download_text_from_s3
from .db import DocumentIndexRecord, insert_document_index
from .embeddings import create_embeddings
from .sections import chunk_chapters, parse_markdown_sections


def markdown_s3_key(s3_upload: dict[str, Any]) -> str:
    key = s3_upload.get("markdown_key")
    if key:
        return key

    keys_by_path = s3_upload.get("keys_by_path") or {}
    key = keys_by_path.get("output.md")
    if key:
        return key

    raise ValueError("S3 upload result does not include output.md")


def read_markdown(upload_result: dict[str, Any], s3_bucket: str, s3_key: str) -> str:
    markdown_file = upload_result.get("markdown_file")
    if markdown_file:
        markdown_path = Path(markdown_file)
    else:
        markdown_path = Path(upload_result["output_dir"]) / "output.md"

    if markdown_path.exists():
        return markdown_path.read_text(encoding="utf-8")

    return download_text_from_s3(s3_bucket, s3_key)


@celery_app.task(
    name="worker.index_ocr_output",
    autoretry_for=(Exception,),
    retry_backoff=True,
    retry_jitter=True,
    retry_kwargs={"max_retries": 3},
)
def index_ocr_output(upload_result: dict[str, Any]):
    if upload_result.get("status") != "uploaded":
        return {
            "status": "skipped",
            "reason": "OCR output was not uploaded to S3",
            "upload_result": upload_result,
        }

    s3_upload = upload_result["s3_upload"]
    s3_bucket = s3_upload["bucket"]
    s3_key = markdown_s3_key(s3_upload)
    markdown = read_markdown(upload_result, s3_bucket, s3_key)
    chapters, sub_chapters = parse_markdown_sections(markdown)
    chunks = chunk_chapters(
        markdown,
        chapters,
        max_chars=CHUNK_MAX_CHARS,
        overlap_lines=CHUNK_OVERLAP_LINES,
    )
    embeddings = (
        create_embeddings([chunk.text for chunk in chunks])
        if ENABLE_EMBEDDINGS
        else [None] * len(chunks)
    )

    document_index = insert_document_index(
        DocumentIndexRecord(
            document_id=upload_result["document_id"],
            user_id=upload_result["user_id"],
            s3_bucket=s3_bucket,
            s3_key=s3_key,
            filename=upload_result.get("filename") or "output.md",
            chapters=chapters,
            sub_chapters=sub_chapters,
            chunks=chunks,
            embeddings=embeddings,
        )
    )
    local_output_cleanup = remove_document_output(Path(upload_result["output_dir"]))

    return {
        "status": "indexed",
        **document_index,
        "s3_bucket": s3_bucket,
        "s3_key": s3_key,
        "local_output_cleanup": local_output_cleanup,
    }
