from pathlib import Path
import traceback
import uuid

from ..config import INPUT_ROOT
from ..uploads.s3 import download_file_from_s3
from .output import (
    prepare_document_output,
    safe_path_part,
    write_markdown,
    write_markdown_images,
)
from .processing import run_ocr


def source_filename(source: dict, fallback: str | None) -> str:
    name = source.get("filename") or fallback or Path(source["key"]).name
    suffix = Path(name).suffix or ".pdf"
    return f"input{suffix}"


def resolve_input_file(
    source: str | dict,
    user_id: str,
    document_id: str,
    filename: str | None,
) -> tuple[Path, bool]:
    if isinstance(source, dict):
        if source.get("type") != "s3":
            raise ValueError(f"Unsupported OCR source type: {source.get('type')}")

        bucket = source["bucket"]
        key = source["key"]
        local_path = (
            INPUT_ROOT
            / "sources"
            / user_id
            / document_id
            / source_filename(source, filename)
        )
        return download_file_from_s3(bucket, key, local_path), True

    return Path(source), False


def process_ocr(
    file_path: str | dict,
    user_id: str = "00000000-0000-0000-0000-000000000123",
    document_id: str | None = None,
    filename: str | None = None,
):
    user_id = safe_path_part(user_id)
    document_id = safe_path_part(document_id or uuid.uuid4().hex)
    file_path, cleanup_input = resolve_input_file(file_path, user_id, document_id, filename)
    filename = filename or file_path.name
    output_paths = prepare_document_output(user_id, document_id)

    if not file_path.exists():
        raise FileNotFoundError(f"File not found: {file_path}")

    if file_path.stat().st_size < 1000:
        raise ValueError(f"File too small / corrupted: {file_path}")

    try:
        final_output, markdown_images = run_ocr(file_path)
    except Exception:
        print(f"OCR failed for {file_path}")
        traceback.print_exc()
        raise

    write_markdown(output_paths.markdown_file, final_output)
    write_markdown_images(output_paths.document_dir, markdown_images)
    if cleanup_input:
        file_path.unlink(missing_ok=True)

    return {
        "status": "ocr_completed",
        "user_id": user_id,
        "document_id": document_id,
        "filename": filename,
        "markdown_file": str(output_paths.markdown_file),
        "output_dir": str(output_paths.document_dir),
    }
