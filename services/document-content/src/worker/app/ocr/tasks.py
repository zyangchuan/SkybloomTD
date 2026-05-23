from pathlib import Path
import traceback
import uuid

from ..celery_app import celery_app
from .output import (
    prepare_document_output,
    safe_path_part,
    write_markdown,
    write_markdown_images,
)
from .processing import run_ocr


@celery_app.task(name="worker.process_ocr")
def process_ocr(
    file_path: str,
    user_id: str = "00000000-0000-0000-0000-000000000123",
    document_id: str | None = None,
    filename: str | None = None,
):
    file_path = Path(file_path)
    user_id = safe_path_part(user_id)
    document_id = safe_path_part(document_id or uuid.uuid4().hex)
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

    return {
        "status": "ocr_completed",
        "user_id": user_id,
        "document_id": document_id,
        "filename": filename,
        "markdown_file": str(output_paths.markdown_file),
        "output_dir": str(output_paths.document_dir),
    }
