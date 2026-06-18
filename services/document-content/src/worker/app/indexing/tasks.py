from pathlib import Path
from typing import Any

from ..ocr.output import remove_document_output
from .db import DocumentIndexRecord, insert_document_index
from .sections import parse_markdown_sections

def index_ocr_output(upload_result: dict[str, Any]):
    s3_bucket = upload_result["s3_bucket"]
    s3_markdown_key = upload_result["s3_markdown_key"]
    local_markdown_path = Path(upload_result["local_markdown_path"])
    markdown = local_markdown_path.read_text(encoding="utf-8")
    chapters, sub_chapters = parse_markdown_sections(markdown)

    document_index = insert_document_index(
        DocumentIndexRecord(
            document_id=upload_result["document_id"],
            user_id=upload_result["user_id"],
            s3_bucket=s3_bucket,
            source_filename=upload_result["source_filename"],
            chapters=chapters,
            sub_chapters=sub_chapters,
        )
    )
    local_output_cleanup = remove_document_output(local_markdown_path.parent)

    return {
        "status": "indexed",
        **document_index,
        "s3_bucket": s3_bucket,
        "s3_markdown_key": s3_markdown_key,
        "local_output_cleanup": local_output_cleanup,
    }
