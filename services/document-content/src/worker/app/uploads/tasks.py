from pathlib import Path

from ..ocr.output import document_output_paths, safe_path_part
from .s3 import upload_output_to_s3


def upload_target_from_ocr_result(ocr_result: dict) -> tuple[str, str, Path]:
    user_id = safe_path_part(ocr_result["user_id"])
    document_id = safe_path_part(ocr_result["document_id"])
    document_output_dir = Path(ocr_result["output_dir"])
    return user_id, document_id, document_output_dir


def upload_target_from_ids(user_id: str, document_id: str) -> tuple[str, str, Path]:
    user_id = safe_path_part(user_id)
    document_id = safe_path_part(document_id)
    output_paths = document_output_paths(user_id, document_id)
    return user_id, document_id, output_paths.document_dir


def upload_ocr_output(ocr_result_or_user_id: dict | str, document_id: str | None = None):
    filename = None
    markdown_file = None

    if isinstance(ocr_result_or_user_id, dict):
        ocr_result = ocr_result_or_user_id
        if ocr_result.get("status") != "ocr_completed":
            return {
                "status": "skipped",
                "reason": "OCR did not complete successfully",
                "ocr_result": ocr_result,
            }
        filename = ocr_result.get("filename")
        markdown_file = ocr_result.get("markdown_file")
        user_id, document_id, document_output_dir = upload_target_from_ocr_result(ocr_result)
    else:
        if document_id is None:
            raise ValueError("document_id is required when uploading by user_id")
        user_id, document_id, document_output_dir = upload_target_from_ids(
            ocr_result_or_user_id,
            document_id,
        )

    if not document_output_dir.exists():
        raise FileNotFoundError(f"Output directory not found: {document_output_dir}")

    s3_upload = upload_output_to_s3(document_output_dir, user_id, document_id)

    return {
        "status": s3_upload["status"],
        "user_id": user_id,
        "document_id": document_id,
        "filename": filename,
        "markdown_file": markdown_file,
        "output_dir": str(document_output_dir),
        "s3_upload": s3_upload,
    }
